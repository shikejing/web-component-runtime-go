package wcr

import "testing"

func reflectSetup(t *testing.T, renders *int) (*Document, *Node) {
	t.Helper()
	reg := NewRegistry()
	err := reg.Define(&Definition{
		Name:               "ref-el",
		ObservedAttributes: []string{"count", "label"},
		Render: func(el *Element) []*Node {
			*renders++
			v, _ := el.Node.GetAttr("count")
			return []*Node{NewText("count=" + v)}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocument(reg)
	host := NewElement("ref-el")
	doc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(doc, doc.Root)
	return doc, host
}

func TestPropertyReflectsToAttribute(t *testing.T) {
	var renders int
	doc, host := reflectSetup(t, &renders)
	el := host.elem
	el.Set("count", 3)
	if v, ok := host.GetAttr("count"); !ok || v != "3" {
		t.Fatalf("property must reflect to attribute, got %q", v)
	}
	el.Set("enabled", true)
	if _, ok := host.GetAttr("enabled"); !ok {
		t.Fatal("bool true must reflect as attribute presence")
	}
	el.Set("enabled", false)
	if _, ok := host.GetAttr("enabled"); ok {
		t.Fatal("bool false must remove the attribute")
	}
	doc.Flush()
}

func TestAttributeRewriteUpdatesStateAndFiresCallback(t *testing.T) {
	var renders int
	doc, host := reflectSetup(t, &renders)
	el := host.elem
	var seen []string
	el.AttributeChanged = func(e *Element, name, old, new string) {
		seen = append(seen, name+"="+new)
	}
	el.SetAttribute("count", "9")
	if got := el.Get("count"); got != "9" {
		t.Fatalf("direct attribute rewrite must update property mirror, got %v", got)
	}
	if len(seen) != 1 || seen[0] != "count=9" {
		t.Fatalf("observed attribute must fire callback, got %v", seen)
	}
	// unobserved attribute does not fire
	el.SetAttribute("other", "x")
	if len(seen) != 1 {
		t.Fatalf("unobserved attribute must not fire, got %v", seen)
	}
	doc.Flush()
}

func TestCoalescedReRender(t *testing.T) {
	var renders int
	doc, host := reflectSetup(t, &renders)
	if renders != 1 {
		t.Fatalf("initial render count = %d, want 1", renders)
	}
	el := host.elem
	// multiple changes within one turn
	el.Set("count", 1)
	el.SetAttribute("count", "2")
	el.Set("label", "x")
	el.SetAttribute("count", "5")
	doc.Flush()
	if renders != 2 {
		t.Fatalf("changes in one turn must coalesce into one re-render, got %d", renders)
	}
	// final state only, no intermediate state observable
	txt := host.ShadowRoot.Children[0].Text
	if txt != "count=5" {
		t.Fatalf("shadow tree must show final state, got %q", txt)
	}
	// a second turn renders again
	el.Set("count", 7)
	doc.Flush()
	if renders != 3 {
		t.Fatalf("second turn must render once more, got %d", renders)
	}
}
