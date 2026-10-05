package wc

import "github.com/shikejing/web-component-runtime-go/dom"

// Assignment 描述一个插槽及其分配到的节点。
// Slot 为 nil 的桶存放未命中任何插槽的节点，保证归属确定可预期。
type Assignment struct {
	Slot  *dom.Node
	Nodes []*dom.Node
}

// slotName 返回插槽名；无名插槽返回空串（默认插槽）。
func slotName(slot *dom.Node) string {
	v, _ := slot.GetAttribute("name")
	return v
}

// findSlots 按文档顺序收集影子树中的插槽。
func findSlots(sr *dom.Node) []*dom.Node {
	var slots []*dom.Node
	if sr == nil {
		return nil
	}
	dom.Walk(sr, func(n *dom.Node) {
		if n.Type == dom.ElementNode && n.Tag == "slot" {
			slots = append(slots, n)
		}
	})
	return slots
}

// Distribute 计算宿主轻 DOM 子节点到影子插槽的分发结果。
// 具名子节点（slot="x"）进入同名插槽，其余进入默认插槽；
// 未命中任何插槽的节点归入 Slot 为 nil 的桶。
func Distribute(host *dom.Node) []Assignment {
	slots := findSlots(host.Shadow)
	byName := map[string]*dom.Node{}
	var defaultSlot *dom.Node
	for _, s := range slots {
		if name := slotName(s); name == "" {
			if defaultSlot == nil {
				defaultSlot = s
			}
		} else if _, ok := byName[name]; !ok {
			byName[name] = s
		}
	}
	assigned := map[*dom.Node][]*dom.Node{}
	var unmatched []*dom.Node
	for _, ch := range host.Children {
		target := defaultSlot
		if ch.Type == dom.ElementNode {
			if name, ok := ch.GetAttribute("slot"); ok && name != "" {
				if s, hit := byName[name]; hit {
					target = s
				} else {
					target = nil // 具名但未命中：不属于任何插槽
				}
			}
		}
		if target == nil {
			unmatched = append(unmatched, ch)
			continue
		}
		assigned[target] = append(assigned[target], ch)
	}
	out := make([]Assignment, 0, len(slots)+1)
	for _, s := range slots {
		out = append(out, Assignment{Slot: s, Nodes: assigned[s]})
	}
	if len(unmatched) > 0 {
		out = append(out, Assignment{Slot: nil, Nodes: unmatched})
	}
	return out
}

// Flatten 返回组合树副本：插槽被其分配节点替换，未命中时使用插槽回退内容，
// 未分配到任何插槽的轻 DOM 节点不出现在组合树中。
func Flatten(n *dom.Node) *dom.Node {
	return flatten(n, nil)
}

// flatten 克隆 n 并按 slotHost 的上下文解析插槽。
func flatten(n *dom.Node, slotHost *dom.Node) *dom.Node {
	c := n.Clone(false)
	switch {
	case n.Type == dom.ElementNode && n.Shadow != nil:
		// 宿主：组合内容来自影子树，插槽相对该宿主解析。
		flattenChildren(n.Shadow, c, n)
	default:
		flattenChildren(n, c, slotHost)
	}
	return c
}

func flattenChildren(src, dst *dom.Node, slotHost *dom.Node) {
	var assigns map[*dom.Node][]*dom.Node
	if slotHost != nil {
		assigns = map[*dom.Node][]*dom.Node{}
		for _, a := range Distribute(slotHost) {
			if a.Slot != nil {
				assigns[a.Slot] = a.Nodes
			}
		}
	}
	for _, ch := range src.Children {
		if slotHost != nil && ch.Type == dom.ElementNode && ch.Tag == "slot" {
			nodes := assigns[ch]
			if len(nodes) == 0 {
				nodes = ch.Children // 回退内容
			}
			for _, an := range nodes {
				dst.AppendChild(Flatten(an))
			}
			continue
		}
		dst.AppendChild(flatten(ch, slotHost))
	}
}
