package wcr

import (
	"errors"
	"strings"
	"testing"
)

func TestDefineDuplicateRejected(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Define(&Definition{Name: "my-el"}); err != nil {
		t.Fatalf("first define: %v", err)
	}
	err := reg.Define(&Definition{Name: "my-el"})
	if err == nil {
		t.Fatal("second define must fail")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("error must wrap ErrDuplicate, got %v", err)
	}
	if !strings.Contains(err.Error(), "my-el") {
		t.Fatalf("error must name the conflicting element, got %q", err.Error())
	}
}

func TestDefineRequiresName(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Define(&Definition{}); err == nil {
		t.Fatal("nameless definition must be rejected")
	}
}

func TestRegistrationOrderDoesNotChangeOutput(t *testing.T) {
	build := func(order []string) string {
		reg := NewRegistry()
		defs := map[string]*Definition{
			"x-a": {Name: "x-a", Render: func(el *Element) []*Node { return []*Node{NewText("A")} }},
			"x-b": {Name: "x-b", Render: func(el *Element) []*Node { return []*Node{NewText("B")} }},
			"x-c": {Name: "x-c", Render: func(el *Element) []*Node { return []*Node{NewText("C")} }},
		}
		for _, name := range order {
			if err := reg.Define(defs[name]); err != nil {
				t.Fatal(err)
			}
		}
		doc := NewDocument(reg)
		root := NewElement("div")
		for _, tag := range []string{"x-c", "x-a", "x-b"} {
			root.AppendChild(NewElement(tag))
		}
		NewRenderer(reg, RenderOptions{}).Render(doc, root)
		return Serialize(root, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true})
	}
	a := build([]string{"x-a", "x-b", "x-c"})
	b := build([]string{"x-c", "x-a", "x-b"})
	if a != b {
		t.Fatalf("registration order changed output:\n%s\nvs\n%s", a, b)
	}
}

func TestLifecycleOrder(t *testing.T) {
	var events []string
	reg := NewRegistry()
	err := reg.Define(&Definition{
		Name:               "lc-el",
		ObservedAttributes: []string{"foo"},
		Render:             func(el *Element) []*Node { return nil },
		Lifecycle: Lifecycle{
			Connected: func(el *Element) { events = append(events, "connected") },
			AttributeChanged: func(el *Element, name, old, new string) {
				events = append(events, "attr:"+name+":"+old+"->"+new)
			},
			Disconnected: func(el *Element) { events = append(events, "disconnected") },
			Adopted: func(el *Element, old, new *Document) {
				events = append(events, "adopted")
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocument(reg)
	root := NewElement("div")
	el := NewElement("lc-el")
	el.SetAttr("foo", "1")
	root.AppendChild(el)
	doc.Root.AppendChild(root)
	NewRenderer(reg, RenderOptions{}).Render(doc, root)

	// attributeChanged (pre-existing observed attr) precedes connected
	want := []string{"attr:foo:->1", "connected"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", events, want)
	}

	// attribute change after connect
	el.elem.SetAttribute("foo", "2")
	doc.Flush()
	if last := events[len(events)-1]; last != "attr:foo:1->2" {
		t.Fatalf("got %v", events)
	}

	// adoption
	doc2 := NewDocument(reg)
	Adopt(doc2, el)
	if last := events[len(events)-1]; last != "adopted" {
		t.Fatalf("got %v", events)
	}

	// disconnection
	Disconnect(el)
	if last := events[len(events)-1]; last != "disconnected" {
		t.Fatalf("got %v", events)
	}
	if el.elem.connected {
		t.Fatal("element must be marked disconnected")
	}
}
