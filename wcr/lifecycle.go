package wcr

// lifecycle.go implements the remaining custom-element reactions:
// disconnection and document adoption, in standard order.

// Disconnect detaches node (and its subtree) from its parent, firing
// Disconnected callbacks in document order for every upgraded custom
// element in the subtree.
func Disconnect(node *Node) {
	if node.Parent != nil {
		p := node.Parent
		for i, c := range p.Children {
			if c == node {
				p.Children = append(p.Children[:i], p.Children[i+1:]...)
				break
			}
		}
		node.Parent = nil
	}
	walkTree(node, func(n *Node) {
		if n.elem != nil && n.elem.connected {
			n.elem.connected = false
			if n.elem.Disconnected != nil {
				n.elem.Disconnected(n.elem)
			}
		}
	})
}

// Adopt moves node (and its subtree) to newDoc, firing Adopted callbacks in
// document order for every upgraded custom element in the subtree. The
// element's document pointer and dirty queue move with it.
func Adopt(newDoc *Document, node *Node) {
	var moved []*Element
	walkTree(node, func(n *Node) {
		if n.elem != nil {
			moved = append(moved, n.elem)
		}
	})
	for _, el := range moved {
		old := el.Doc
		if old == newDoc {
			continue
		}
		el.Doc = newDoc
		if el.Adopted != nil {
			el.Adopted(el, old, newDoc)
		}
	}
}
