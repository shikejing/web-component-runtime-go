package wcr

import (
	"errors"
	"fmt"
	"sync"
)

// Lifecycle groups the custom-element reaction callbacks. All are optional.
type Lifecycle struct {
	Connected    func(el *Element)
	Disconnected func(el *Element)
	// AttributeChanged is called for observed attributes only.
	AttributeChanged func(el *Element, name, oldVal, newVal string)
	// Adopted is called when the element moves between documents/registries.
	Adopted func(el *Element, oldDoc, newDoc *Document)
}

// Definition describes a custom element class.
type Definition struct {
	Name string // must contain a dash, e.g. "my-el"
	// ObservedAttributes lists attributes that trigger AttributeChanged.
	ObservedAttributes []string
	// Render produces the shadow tree children for the element.
	Render func(el *Element) []*Node
	// Styles are scoped to the shadow tree of this element.
	Styles string
	Lifecycle
}

// Registry holds custom element definitions. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	defs  map[string]*Definition
	order []string // registration order, for diagnostics only
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{defs: make(map[string]*Definition)}
}

// ErrDuplicate is returned when a name is registered twice.
var ErrDuplicate = errors.New("wcr: duplicate custom element definition")

// Define registers def. Registering the same name twice fails with a
// wrapped ErrDuplicate explaining the conflict.
func (r *Registry) Define(def *Definition) error {
	if def == nil || def.Name == "" {
		return errors.New("wcr: definition requires a name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[def.Name]; exists {
		return fmt.Errorf("%w: %q is already defined", ErrDuplicate, def.Name)
	}
	cp := *def
	r.defs[def.Name] = &cp
	r.order = append(r.order, def.Name)
	return nil
}

// Lookup returns the definition for name, or nil.
func (r *Registry) Lookup(name string) *Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defs[name]
}

// snapshot returns a copy of the definition map so a render in progress is
// unaffected by concurrent Define calls.
func (r *Registry) snapshot() map[string]*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*Definition, len(r.defs))
	for k, v := range r.defs {
		out[k] = v
	}
	return out
}
