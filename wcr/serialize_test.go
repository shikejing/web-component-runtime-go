package wcr

import "testing"

func TestSerializeParseRoundTripByteIdentical(t *testing.T) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name:   "rt-el",
		Styles: ".a { color: red; margin: 0 }",
		Render: func(el *Element) []*Node {
			d := NewElement("div")
			d.SetAttr("class", "a")
			d.AppendChild(NewText("x<y&z"))
			s := NewElement("slot")
			s.SetAttr("name", "n")
			s.AppendChild(NewText("fb"))
			return []*Node{d, s}
		},
	})
	doc := NewDocument(reg)
	host := NewElement("rt-el")
	host.SetAttr("title", `a"b&c`)
	host.AppendChild(NewText("light"))
	doc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(doc, doc.Root)

	opts := SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true}
	first := Serialize(doc.Root, opts)
	parsed, err := Parse(first)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	second := Serialize(parsed, opts)
	if first != second {
		t.Fatalf("round trip not byte-identical:\n%s\nvs\n%s", first, second)
	}
	// and again, for stability
	third, err := Parse(second)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if got := Serialize(third, opts); got != first {
		t.Fatalf("second round trip drifted:\n%s\nvs\n%s", first, got)
	}
}

func TestSerializeDeterministicAcrossRuns(t *testing.T) {
	build := func() string {
		reg := NewRegistry()
		reg.Define(&Definition{
			Name: "det-el",
			Render: func(el *Element) []*Node {
				return []*Node{NewText("v")}
			},
		})
		doc := NewDocument(reg)
		root := NewElement("div")
		for i := 0; i < 8; i++ {
			root.AppendChild(NewElement("det-el"))
		}
		doc.Root.AppendChild(root)
		NewRenderer(reg, RenderOptions{Concurrency: 8}).Render(doc, root)
		return Serialize(doc.Root, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true})
	}
	base := build()
	for i := 0; i < 20; i++ {
		if got := build(); got != base {
			t.Fatalf("run %d differs", i)
		}
	}
}
