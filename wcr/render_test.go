package wcr

import (
	"fmt"
	"strings"
	"testing"
)

// buildWideTree creates a document with n independent custom elements.
func buildWideTree(reg *Registry, n int) (*Document, *Node) {
	doc := NewDocument(reg)
	root := NewElement("div")
	for i := 0; i < n; i++ {
		el := NewElement("par-el")
		el.SetAttr("idx", fmt.Sprint(i))
		root.AppendChild(el)
	}
	doc.Root.AppendChild(root)
	return doc, root
}

func parDef() *Definition {
	return &Definition{
		Name: "par-el",
		Render: func(el *Element) []*Node {
			idx, _ := el.Node.GetAttr("idx")
			inner := NewElement("span")
			inner.AppendChild(NewText("item-" + idx))
			return []*Node{inner}
		},
	}
}

func TestDeterministicAcrossConcurrency(t *testing.T) {
	var outputs []string
	for _, conc := range []int{0, 1, 2, 4, 16} {
		reg := NewRegistry()
		if err := reg.Define(parDef()); err != nil {
			t.Fatal(err)
		}
		doc, root := buildWideTree(reg, 64)
		NewRenderer(reg, RenderOptions{Concurrency: conc}).Render(doc, root)
		outputs = append(outputs, Serialize(root, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true}))
	}
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Fatalf("output differs between concurrency levels:\n%s\nvs\n%s", outputs[0], outputs[i])
		}
	}
}

func TestRegistrySnapshotIsolation(t *testing.T) {
	reg := NewRegistry()
	// a definition whose Render registers a NEW element mid-render; the
	// in-flight tree must not be affected
	reg.Define(&Definition{
		Name: "snap-el",
		Render: func(el *Element) []*Node {
			reg.Define(&Definition{
				Name:   "late-el",
				Render: func(e *Element) []*Node { return []*Node{NewText("LATE")} },
			})
			return []*Node{NewElement("late-el")}
		},
	})
	doc := NewDocument(reg)
	root := NewElement("div")
	root.AppendChild(NewElement("snap-el"))
	doc.Root.AppendChild(root)
	NewRenderer(reg, RenderOptions{}).Render(doc, root)
	out := Serialize(root, SerializeOptions{DeclarativeShadowDOM: true})
	if strings.Contains(out, "LATE") {
		t.Fatal("definition added mid-render must not affect the in-flight tree")
	}
	// but a fresh render sees it
	doc2 := NewDocument(reg)
	root2 := NewElement("div")
	root2.AppendChild(NewElement("late-el"))
	doc2.Root.AppendChild(root2)
	NewRenderer(reg, RenderOptions{}).Render(doc2, root2)
	if !strings.Contains(Serialize(root2, SerializeOptions{DeclarativeShadowDOM: true}), "LATE") {
		t.Fatal("later renders must see the new definition")
	}
}

func TestElementErrorIsolation(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name: "boom-el",
		Render: func(el *Element) []*Node {
			panic("kaboom")
		},
	})
	reg.Define(&Definition{
		Name:   "ok-el",
		Render: func(el *Element) []*Node { return []*Node{NewText("OK")} },
	})
	doc := NewDocument(reg)
	root := NewElement("div")
	root.AppendChild(NewElement("ok-el"))
	root.AppendChild(NewElement("boom-el"))
	root.AppendChild(NewElement("ok-el"))
	doc.Root.AppendChild(root)

	// default strategy: comment placeholder, siblings unaffected
	NewRenderer(reg, RenderOptions{}).Render(doc, root)
	out := Serialize(root, SerializeOptions{DeclarativeShadowDOM: true})
	if got := strings.Count(out, "OK"); got != 2 {
		t.Fatalf("siblings must render normally, got %d OK in %s", got, out)
	}
	if !strings.Contains(out, "<!--wcr: render failed: boom-el-->") {
		t.Fatalf("failed element must degrade to placeholder, got %s", out)
	}

	// custom strategy
	doc2 := NewDocument(reg)
	root2 := NewElement("div")
	root2.AppendChild(NewElement("boom-el"))
	doc2.Root.AppendChild(root2)
	NewRenderer(reg, RenderOptions{
		OnElementError: func(el *Element, err error) []*Node {
			return []*Node{NewText("FALLBACK")}
		},
	}).Render(doc2, root2)
	out2 := Serialize(root2, SerializeOptions{DeclarativeShadowDOM: true})
	if !strings.Contains(out2, "FALLBACK") {
		t.Fatalf("custom degradation strategy must be honored, got %s", out2)
	}
}

func TestFailedElementMarkedAndSkipped(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name:   "fail-el",
		Render: func(el *Element) []*Node { panic("x") },
	})
	doc := NewDocument(reg)
	host := NewElement("fail-el")
	doc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(doc, doc.Root)
	if host.elem == nil || !host.elem.failed || host.elem.renderErr == nil {
		t.Fatal("failed element must carry its error state")
	}
	// flush must not attempt to re-render a failed element
	host.elem.Set("a", 1)
	doc.Flush()
}
