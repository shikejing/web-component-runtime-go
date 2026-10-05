package wcr

import "testing"

// build a host with default + named slot shadow tree:
// <slot name="header">H-FALLBACK</slot><slot></slot><slot name="footer">F-FALLBACK</slot>
func slotHost() (*Registry, *Document, *Node) {
	reg := NewRegistry()
	reg.Define(&Definition{
		Name: "slot-el",
		Render: func(el *Element) []*Node {
			mk := func(name, fallback string) *Node {
				s := NewElement("slot")
				if name != "" {
					s.SetAttr("name", name)
				}
				s.AppendChild(NewText(fallback))
				return s
			}
			return []*Node{mk("header", "H-FALLBACK"), mk("", "D-FALLBACK"), mk("footer", "F-FALLBACK")}
		},
	})
	doc := NewDocument(reg)
	host := NewElement("slot-el")
	doc.Root.AppendChild(host)
	return reg, doc, host
}

func slotsOf(host *Node) map[string]*Node {
	out := map[string]*Node{}
	for _, s := range collectSlots(host.ShadowRoot.Children) {
		if n, ok := s.GetAttr("name"); ok {
			out[n] = s
		} else {
			out[""] = s
		}
	}
	return out
}

func TestDefaultAndNamedSlots(t *testing.T) {
	reg, doc, host := slotHost()
	h := NewElement("span")
	h.SetAttr("slot", "header")
	h.AppendChild(NewText("TITLE"))
	body := NewText("BODY")
	ghost := NewElement("b")
	ghost.SetAttr("slot", "nonexistent")
	host.AppendChild(h)
	host.AppendChild(body)
	host.AppendChild(ghost)
	NewRenderer(reg, RenderOptions{}).Render(doc, host)

	slots := slotsOf(host)
	if got := AssignedNodes(slots["header"]); len(got) != 1 || got[0] != h {
		t.Fatalf("header slot assignment wrong: %v", got)
	}
	if got := AssignedNodes(slots[""]); len(got) != 1 || got[0] != body {
		t.Fatalf("default slot assignment wrong: %v", got)
	}
	// unmatched node: deterministically unassigned
	if ghost.assignedSlot != nil {
		t.Fatal("node naming a non-existent slot must stay unassigned")
	}
	// footer got nothing: fallback active
	if !SlotFallbackActive(slots["footer"]) {
		t.Fatal("footer slot must show fallback")
	}
	if SlotFallbackActive(slots["header"]) || SlotFallbackActive(slots[""]) {
		t.Fatal("filled slots must not show fallback")
	}
}

func TestSlotRedistributionOnDynamicChange(t *testing.T) {
	reg, doc, host := slotHost()
	a := NewElement("i")
	a.AppendChild(NewText("A"))
	host.AppendChild(a)
	NewRenderer(reg, RenderOptions{}).Render(doc, host)
	slots := slotsOf(host)
	if got := AssignedNodes(slots[""]); len(got) != 1 {
		t.Fatalf("before: default slot should have 1 node, got %d", len(got))
	}
	// move the node into the named slot and re-assign
	a.SetAttr("slot", "header")
	assignSlots(host)
	if got := AssignedNodes(slots["header"]); len(got) != 1 || got[0] != a {
		t.Fatalf("after: header slot should own the node")
	}
	if got := AssignedNodes(slots[""]); len(got) != 0 {
		t.Fatalf("after: default slot should be empty")
	}
	if !SlotFallbackActive(slots[""]) {
		t.Fatal("default slot fallback must reactivate")
	}
	// add a new node and re-assign: document order preserved
	b := NewElement("i")
	host.AppendChild(b)
	c := NewElement("i")
	host.AppendChild(c)
	assignSlots(host)
	got := AssignedNodes(slots[""])
	if len(got) != 2 || got[0] != b || got[1] != c {
		t.Fatalf("document order must be preserved, got %v", got)
	}
}
