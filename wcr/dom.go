// Package wcr implements a server-side rendering and client-side hydration
// runtime for custom elements and shadow DOM.
package wcr

// NodeKind discriminates the concrete kind of a Node.
type NodeKind int

const (
	ElementNode NodeKind = iota
	TextNode
	CommentNode
)

// Node is the single DOM node type used by the runtime.
type Node struct {
	Kind     NodeKind
	Tag      string // element only, always lowercased
	Text     string // text/comment only
	Attrs    []Attr // element only, insertion ordered
	Children []*Node
	Parent   *Node

	// ShadowRoot is non-nil when the element hosts a shadow tree.
	ShadowRoot *ShadowRoot

	// internal bookkeeping
	assignedSlot *Node       // the <slot> this node is currently distributed to
	assigned     []*Node     // slot only: light-DOM nodes distributed here
	elem         *Element    // set when this node is upgraded to a custom element
	shadowOf     *ShadowRoot // set on "#shadow-root" boundary nodes
}

// Attr is a single attribute. Value may be empty for boolean attributes.
type Attr struct {
	Name  string
	Value string
}

// ShadowRoot is the root of a shadow tree attached to a host element.
type ShadowRoot struct {
	Host     *Node
	Children []*Node
	Mode     string // "open" or "closed"
	Styles   string // scoped styles declared by the definition

	rootNode *Node // lazily materialized "#shadow-root" boundary node
}

// NewElement creates an element node with the given (lowercased) tag.
func NewElement(tag string) *Node { return &Node{Kind: ElementNode, Tag: tag} }

// NewText creates a text node.
func NewText(s string) *Node { return &Node{Kind: TextNode, Text: s} }

// AppendChild appends child to n, maintaining parent pointers.
func (n *Node) AppendChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// GetAttr returns the attribute value and whether it is present.
func (n *Node) GetAttr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// SetAttr sets or replaces an attribute.
func (n *Node) SetAttr(name, value string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{Name: name, Value: value})
}

// RemoveAttr removes an attribute if present.
func (n *Node) RemoveAttr(name string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs = append(n.Attrs[:i], n.Attrs[i+1:]...)
			return
		}
	}
}
