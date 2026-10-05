package wcr

import (
	"strings"
	"testing"
)

func hydrateDef() *Definition {
	return &Definition{
		Name:               "hy-el",
		ObservedAttributes: []string{"mode"},
		Styles:             ".box { color: red }",
		Render: func(el *Element) []*Node {
			d := NewElement("div")
			d.SetAttr("class", "box")
			d.AppendChild(NewText("SHADOW"))
			slot := NewElement("slot")
			return []*Node{d, slot}
		},
	}
}

func TestFullChainRenderSerializeParseHydrate(t *testing.T) {
	// server side
	reg := NewRegistry()
	if err := reg.Define(hydrateDef()); err != nil {
		t.Fatal(err)
	}
	sdoc := NewDocument(reg)
	host := NewElement("hy-el")
	host.SetAttr("mode", "on")
	host.AppendChild(NewText("LIGHT"))
	sdoc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(sdoc, sdoc.Root)
	html := Serialize(sdoc.Root, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true})

	if !strings.Contains(html, `<template shadowrootmode="open">`) {
		t.Fatalf("expected declarative shadow DOM, got %s", html)
	}
	if !strings.Contains(html, `data-wcr-id="0"`) {
		t.Fatalf("expected hydration marker, got %s", html)
	}

	// client side: parse what the server sent
	parsed, err := Parse(html)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	before := Serialize(parsed, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true})

	creg := NewRegistry()
	if err := creg.Define(hydrateDef()); err != nil {
		t.Fatal(err)
	}
	cdoc := NewDocument(creg)
	cdoc.Root = parsed
	var connected []string
	// observe lifecycle on the client
	def := creg.Lookup("hy-el")
	def.Connected = func(el *Element) { connected = append(connected, el.Node.Tag) }

	if err := NewHydrator(creg).Hydrate(cdoc, parsed); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	after := Serialize(parsed, SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true})

	// observable state identical before/after activation
	if before != after {
		t.Fatalf("hydration changed observable state:\n%s\nvs\n%s", before, after)
	}
	// element upgraded in place, not re-rendered
	var upgraded *Node
	walkTree(parsed, func(n *Node) {
		if n.Tag == "hy-el" {
			upgraded = n
		}
	})
	if upgraded == nil || upgraded.elem == nil {
		t.Fatal("element must be upgraded after hydration")
	}
	if len(connected) != 1 {
		t.Fatalf("connected must fire once, got %v", connected)
	}
	// light DOM still in place and slots assignable
	assignSlots(upgraded)
	slots := collectSlots(upgraded.ShadowRoot.Children)
	if len(slots) != 1 || len(AssignedNodes(slots[0])) != 1 {
		t.Fatal("slot distribution must work on the hydrated tree")
	}
}

func TestHydrateIdempotent(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Define(hydrateDef()); err != nil {
		t.Fatal(err)
	}
	doc := NewDocument(reg)
	host := NewElement("hy-el")
	host.SetAttr("data-wcr-id", "0")
	doc.Root.AppendChild(host)
	NewRenderer(reg, RenderOptions{}).Render(doc, doc.Root)

	// strip runtime state to simulate a fresh client tree
	host.elem = nil
	h := NewHydrator(reg)
	if err := h.Hydrate(doc, doc.Root); err != nil {
		t.Fatalf("first hydrate: %v", err)
	}
	first := host.elem
	if err := h.Hydrate(doc, doc.Root); err != nil {
		t.Fatalf("second hydrate must be a no-op, got %v", err)
	}
	if host.elem != first {
		t.Fatal("repeated hydration must not replace the element state")
	}
}

func TestHydrateFailureRollsBack(t *testing.T) {
	reg := NewRegistry()
	bad := *hydrateDef()
	bad.Name = "bad-el"
	bad.AttributeChanged = func(el *Element, name, old, new string) {
		panic("boom")
	}
	if err := reg.Define(&bad); err != nil {
		t.Fatal(err)
	}
	if err := reg.Define(hydrateDef()); err != nil {
		t.Fatal(err)
	}
	doc := NewDocument(reg)
	badNode := NewElement("bad-el")
	badNode.SetAttr("mode", "x")
	badNode.SetAttr("data-wcr-id", "0")
	good := NewElement("hy-el")
	good.SetAttr("data-wcr-id", "1")
	doc.Root.AppendChild(badNode)
	doc.Root.AppendChild(good)

	err := NewHydrator(reg).Hydrate(doc, doc.Root)
	if err == nil {
		t.Fatal("expected aggregated hydration error")
	}
	// failed element fully rolled back, not half-upgraded
	if badNode.elem != nil {
		t.Fatal("failed element must be rolled back to pre-hydration state")
	}
	// sibling hydrated fine — failure is isolated
	if good.elem == nil {
		t.Fatal("sibling element must hydrate despite the failure")
	}
	// retry after fixing the definition succeeds
	reg2 := NewRegistry()
	fixed := bad
	fixed.AttributeChanged = nil
	reg2.Define(&fixed)
	reg2.Define(hydrateDef())
	if err := NewHydrator(reg2).Hydrate(doc, doc.Root); err != nil {
		t.Fatalf("retry after rollback must succeed, got %v", err)
	}
	if badNode.elem == nil {
		t.Fatal("retried element must now be hydrated")
	}
}
