package wcr

// Element pairs a DOM node with runtime state for a custom element instance.
type Element struct {
	Node *Node
	Def  *Definition
	Doc  *Document

	Lifecycle // promoted callbacks copied from the definition at upgrade

	props      map[string]any
	hydrated   bool
	failed     bool
	renderErr  error
	connected  bool
	dirty      bool
	reflecting bool
}

// Document is an owning document for elements (used by Adopted callbacks).
type Document struct {
	Registry *Registry
	Root     *Node

	pending []*Element // dirty queue, see reflect.go
}

// NewDocument creates a document bound to reg.
func NewDocument(reg *Registry) *Document {
	return &Document{Registry: reg, Root: NewElement("#document")}
}

// Get returns a reflected property value.
func (e *Element) Get(name string) any { return e.props[name] }
