package wcr

import (
	"strings"
	"testing"
)

func TestScopeStylesRewriting(t *testing.T) {
	host := NewElement("my-card")
	out := ScopeStyles(".title { color: red } :host { display: block }", host, nil)
	if !strings.Contains(out, `[data-wcr-scope="my-card"] .title { color: red }`) {
		t.Fatalf("class selector not scoped: %q", out)
	}
	if !strings.Contains(out, `[data-wcr-scope="my-card"] { display: block }`) {
		t.Fatalf(":host not rewritten: %q", out)
	}
	// deterministic: same input, same bytes
	if out != ScopeStyles(".title { color: red } :host { display: block }", host, nil) {
		t.Fatal("scoping must be deterministic")
	}
}

func TestInnerStylesDoNotLeakOut(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name:   "inner-el",
		Styles: ".secret { color: red }",
		Render: func(el *Element) []*Node {
			d := NewElement("div")
			d.SetAttr("class", "secret")
			return []*Node{d}
		},
	})
	doc := NewDocument(reg)
	outside := NewElement("div")
	outside.SetAttr("class", "secret")
	host := NewElement("inner-el")
	root := NewElement("div")
	root.AppendChild(outside)
	root.AppendChild(host)
	doc.Root.AppendChild(root)
	NewRenderer(reg, RenderOptions{}).Render(doc, root)

	inner := host.ShadowRoot.Children[0]
	if got := ComputedStyle(inner)["color"]; got != "red" {
		t.Fatalf("inner node should get scoped color, got %q", got)
	}
	if got := ComputedStyle(outside)["color"]; got != "" {
		t.Fatalf("inner styles must not leak outside, got %q", got)
	}
}

func TestOuterStylesDoNotCrossBoundaryExceptInherited(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name: "bound-el",
		Render: func(el *Element) []*Node {
			s := NewElement("span")
			return []*Node{s}
		},
	})
	doc := NewDocument(reg)
	host := NewElement("bound-el")
	host.SetAttr("style", "color: green; position: absolute")
	doc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(doc, host)

	inner := host.ShadowRoot.Children[0]
	cs := ComputedStyle(inner)
	if cs["color"] != "green" {
		t.Fatalf("inherited color must cross the boundary, got %q", cs["color"])
	}
	if cs["position"] != "" {
		t.Fatalf("non-inherited position must not cross the boundary, got %q", cs["position"])
	}
}

func TestNestedShadowTreesIsolateLayerByLayer(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name:   "lvl-inner",
		Styles: "span { color: blue }",
		Render: func(el *Element) []*Node { return []*Node{NewElement("span")} },
	})
	reg.Define(&Definition{
		Name:   "lvl-outer",
		Styles: "span { color: red }",
		Render: func(el *Element) []*Node { return []*Node{NewElement("lvl-inner")} },
	})
	doc := NewDocument(reg)
	outer := NewElement("lvl-outer")
	doc.Root.AppendChild(outer)
	NewRenderer(reg, RenderOptions{}).Render(doc, doc.Root)

	innerHost := outer.ShadowRoot.Children[0]
	span := innerHost.ShadowRoot.Children[0]
	// the span sees lvl-inner's styles, not lvl-outer's
	if got := ComputedStyle(span)["color"]; got != "blue" {
		t.Fatalf("inner layer should win inside its own tree, got %q", got)
	}
	// lvl-outer's "span" selector must not match across the boundary:
	// verify by checking the inner host itself against outer rules
	if got := ComputedStyle(innerHost)["color"]; got == "red" {
		t.Fatal("outer scoped selector must not match inside nested shadow tree")
	}
}
