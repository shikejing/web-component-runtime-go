package wcr

import (
	"errors"
	"fmt"
)

// hydrate.go implements client-side activation: server-rendered trees are
// taken over in place, without re-rendering.
//
// Guarantees:
//   - The DOM structure is never rebuilt; hydration only attaches Element
//     state and fires lifecycle callbacks, so observable state before and
//     after activation is identical.
//   - Hydration is idempotent: already-hydrated elements are skipped.
//   - Failures are transactional per element: a panicking callback rolls
//     the element back to its pre-hydration state (never half-upgraded),
//     other elements still hydrate, and the returned error aggregates all
//     failures. A later Hydrate call can retry the rolled-back elements.

// ErrAlreadyHydrated is returned by HydrateElement when the element is
// already active.
var ErrAlreadyHydrated = errors.New("wcr: element already hydrated")

// Hydrator attaches behavior to server-rendered trees without re-rendering.
type Hydrator struct {
	Reg *Registry
}

// NewHydrator builds a hydrator for reg.
func NewHydrator(reg *Registry) *Hydrator { return &Hydrator{Reg: reg} }

// Hydrate walks root in document order, activating every custom element
// that carries server-rendered state (a shadow root or a hydration marker)
// and has a registered definition. Individual failures are isolated and
// aggregated into the returned error.
func (h *Hydrator) Hydrate(doc *Document, root *Node) error {
	defs := h.Reg.snapshot()
	var targets []*Node
	walkTree(root, func(n *Node) {
		if n.Kind != ElementNode || n.elem != nil {
			return
		}
		if _, marked := n.GetAttr("data-wcr-id"); !marked && n.ShadowRoot == nil {
			return
		}
		if _, ok := defs[n.Tag]; ok {
			targets = append(targets, n)
		}
	})
	var errs []error
	// Phase 1: attach state + attributeChanged (transactional per element).
	var ready []*Element
	for _, n := range targets {
		el, err := h.activate(doc, defs[n.Tag], n)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ready = append(ready, el)
	}
	// Phase 2: connected callbacks, document order.
	for _, el := range ready {
		if err := connectSafely(el); err != nil {
			errs = append(errs, err)
		}
	}
	doc.Flush()
	return errors.Join(errs...)
}

// activate attaches Element state to n without touching its DOM structure.
// On any callback panic the element is rolled back to its pre-hydration
// state and the error is returned.
func (h *Hydrator) activate(doc *Document, def *Definition, n *Node) (el *Element, err error) {
	if n.elem != nil && n.elem.hydrated {
		return nil, fmt.Errorf("%w: <%s>", ErrAlreadyHydrated, n.Tag)
	}
	el = &Element{Node: n, Def: def, Doc: doc, Lifecycle: def.Lifecycle, hydrated: true}
	n.elem = el
	defer func() {
		if rec := recover(); rec != nil {
			// roll back: no half-hydrated elements
			n.elem = nil
			el.hydrated = false
			el.connected = false
			err = fmt.Errorf("wcr: hydration of <%s> failed, rolled back: %v", n.Tag, rec)
			el = nil
		}
	}()
	for _, name := range def.ObservedAttributes {
		if v, ok := n.GetAttr(name); ok && el.AttributeChanged != nil {
			el.AttributeChanged(el, name, "", v)
		}
	}
	return el, nil
}

func connectSafely(el *Element) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			el.connected = false
			err = fmt.Errorf("wcr: connected of <%s> panicked: %v", el.Node.Tag, rec)
		}
	}()
	el.connected = true
	if el.Connected != nil {
		el.Connected(el)
	}
	return nil
}
