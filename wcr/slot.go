package wcr

// slot.go implements slot distribution (shadow DOM "slot assignment").
//
// Rules:
//   - A light-DOM child with slot="x" is distributed to the shadow tree's
//     <slot name="x">; children without a slot attribute go to the default
//     (unnamed) slot.
//   - Distribution is recomputed from scratch on every call, so dynamic
//     changes to the light DOM or the shadow tree are picked up by simply
//     calling assignSlots again. Results depend only on document order,
//     never on map iteration order.
//   - A child naming a non-existent slot keeps assignedSlot == nil: it
//     deterministically stays unrendered in the light DOM.
//   - A slot with no assigned nodes exposes its fallback content; see
//     SlotFallbackActive.

// assignSlots distributes the light-DOM children of host into the slots of
// its shadow tree.
func assignSlots(host *Node) {
	if host.ShadowRoot == nil {
		return
	}
	slots := collectSlots(host.ShadowRoot.Children)
	// clear previous assignment
	for _, s := range slots {
		s.assigned = nil
	}
	for _, c := range host.Children {
		c.assignedSlot = nil
	}
	var def *Node // default slot
	byName := map[string]*Node{}
	for _, s := range slots {
		if name, ok := s.GetAttr("name"); ok && name != "" {
			if _, taken := byName[name]; !taken {
				byName[name] = s // first slot with a name wins
			}
		} else if def == nil {
			def = s
		}
	}
	for _, c := range host.Children {
		target := def
		if name, ok := c.GetAttr("slot"); ok && name != "" {
			s, found := byName[name]
			if !found {
				continue // unmatched: deterministically unassigned
			}
			target = s
		}
		if target == nil {
			continue
		}
		c.assignedSlot = target
		target.assigned = append(target.assigned, c)
	}
}

// collectSlots returns all <slot> elements in tree order.
func collectSlots(nodes []*Node) []*Node {
	var out []*Node
	var walk func(ns []*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			if n.Kind == ElementNode && n.Tag == "slot" {
				out = append(out, n)
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

// SlotFallbackActive reports whether slot currently shows its fallback
// content (i.e. no light-DOM node is distributed to it).
func SlotFallbackActive(slot *Node) bool { return len(slot.assigned) == 0 }

// AssignedNodes returns the nodes distributed to slot, in document order.
func AssignedNodes(slot *Node) []*Node { return slot.assigned }
