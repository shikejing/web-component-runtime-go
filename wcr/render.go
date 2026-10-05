package wcr

import (
	"fmt"
	"sync"
)

// render.go expands custom elements into shadow trees.
//
// Isolation & determinism guarantees:
//   - The registry is snapshotted once per Render call: definitions added
//     or replaced mid-render never affect the in-flight tree.
//   - A panicking Render callback fails only its own element; the rest of
//     the tree renders normally (degradation via RenderOptions).
//   - With Concurrency > 1, independent shadow trees render in parallel,
//     but every result is attached to a fixed position and connected
//     callbacks fire in document order, so output is byte-identical for
//     any concurrency level.

// RenderOptions tunes the renderer.
type RenderOptions struct {
	// Concurrency bounds parallel element rendering. <=1 means sequential.
	Concurrency int
	// OnElementError returns a replacement subtree for a failed element's
	// shadow content. If nil, a comment placeholder is used.
	OnElementError func(el *Element, err error) (fallback []*Node)
}

// Renderer renders a document tree against a registry snapshot.
type Renderer struct {
	Reg  *Registry
	Opts RenderOptions
}

// NewRenderer builds a renderer for reg.
func NewRenderer(reg *Registry, opts RenderOptions) *Renderer {
	return &Renderer{Reg: reg, Opts: opts}
}

// Render expands all custom elements under root, attaches shadow trees,
// distributes slots and fires lifecycle callbacks in standard order.
// It returns root for chaining.
func (r *Renderer) Render(doc *Document, root *Node) *Node {
	defs := r.Reg.snapshot()
	// Wave expansion: each wave finds not-yet-upgraded custom elements whose
	// shadow content can be produced independently, renders them (possibly
	// concurrently), then attaches results at fixed positions. Nested custom
	// elements inside fresh shadow trees surface in the next wave.
	for {
		targets := collectUpgradable(root, defs)
		if len(targets) == 0 {
			break
		}
		results := r.renderWave(doc, defs, targets)
		for i, n := range targets {
			attachShadow(doc, defs, n, results[i])
		}
	}
	// Slot assignment after the whole tree is stable.
	walkTree(root, func(n *Node) {
		if n.ShadowRoot != nil {
			assignSlots(n)
		}
	})
	// Connected callbacks fire last, in document order.
	walkTree(root, func(n *Node) {
		if n.elem != nil && !n.elem.connected && !n.elem.failed {
			n.elem.connected = true
			if n.elem.Connected != nil {
				n.elem.Connected(n.elem)
			}
		}
	})
	doc.Flush() // coalesce any changes made by connected callbacks
	return root
}

// collectUpgradable returns, in document order, custom element nodes that
// have a definition but no shadow root yet.
func collectUpgradable(root *Node, defs map[string]*Definition) []*Node {
	var out []*Node
	walkTree(root, func(n *Node) {
		if n.Kind == ElementNode && n.ShadowRoot == nil && n.elem == nil {
			if _, ok := defs[n.Tag]; ok {
				out = append(out, n)
			}
		}
	})
	return out
}

type waveResult struct {
	children []*Node
	styles   string
	err      error
}

// renderWave produces shadow content for each target. With Concurrency > 1
// the work is fanned out to a bounded worker pool; results are returned in
// the same order as targets regardless of completion order.
func (r *Renderer) renderWave(doc *Document, defs map[string]*Definition, targets []*Node) []waveResult {
	results := make([]waveResult, len(targets))
	conc := r.Opts.Concurrency
	if conc <= 1 || len(targets) <= 1 {
		for i, n := range targets {
			results[i] = r.renderOne(doc, defs, n)
		}
		return results
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, n := range targets {
		wg.Add(1)
		go func(i int, n *Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = r.renderOne(doc, defs, n)
		}(i, n)
	}
	wg.Wait()
	return results
}

// renderOne upgrades one element and produces its shadow content, isolating
// panics to this element.
func (r *Renderer) renderOne(doc *Document, defs map[string]*Definition, n *Node) (res waveResult) {
	def := defs[n.Tag]
	el := &Element{Node: n, Def: def, Doc: doc, Lifecycle: def.Lifecycle}
	n.elem = el
	// attributeChanged for pre-existing observed attributes fires before
	// connected, per the custom-elements spec ordering.
	for _, name := range def.ObservedAttributes {
		if v, ok := n.GetAttr(name); ok && el.AttributeChanged != nil {
			el.AttributeChanged(el, name, "", v)
		}
	}
	defer func() {
		if rec := recover(); rec != nil {
			el.failed = true
			el.renderErr = fmt.Errorf("wcr: render of <%s> panicked: %v", n.Tag, rec)
			res = waveResult{err: el.renderErr}
			if r.Opts.OnElementError != nil {
				res.children = r.Opts.OnElementError(el, el.renderErr)
			} else {
				res.children = []*Node{{Kind: CommentNode, Text: "wcr: render failed: " + n.Tag}}
			}
		}
	}()
	if def.Render == nil {
		return waveResult{}
	}
	children := def.Render(el)
	return waveResult{children: children, styles: def.Styles}
}

// attachShadow installs rendered shadow content on n and upgrades nested
// custom elements found inside it on the next wave.
func attachShadow(doc *Document, defs map[string]*Definition, n *Node, res waveResult) {
	sr := &ShadowRoot{Host: n, Mode: "open", Styles: res.styles}
	n.ShadowRoot = sr
	for _, c := range res.children {
		c.Parent = nil
		sr.Children = append(sr.Children, c)
		c.Parent = shadowRootNode(sr)
	}
}

// shadowRootNode materializes a stable parent node for shadow children so
// ancestor walks (e.g. style scoping) can identify the boundary.
func shadowRootNode(sr *ShadowRoot) *Node {
	if sr.rootNode == nil {
		sr.rootNode = &Node{Kind: ElementNode, Tag: "#shadow-root"}
		sr.rootNode.Parent = sr.Host
		sr.rootNode.shadowOf = sr
	}
	return sr.rootNode
}

// walkTree visits n and all descendants (light DOM and shadow trees) in
// document order.
func walkTree(n *Node, fn func(*Node)) {
	fn(n)
	if n.ShadowRoot != nil {
		shadowRootNode(n.ShadowRoot) // ensure boundary node exists
		for _, c := range n.ShadowRoot.Children {
			walkTree(c, fn)
		}
	}
	for _, c := range n.Children {
		walkTree(c, fn)
	}
}

// reRender replaces the element's shadow tree once, preserving identity of
// the host node and its light DOM. Used by Document.Flush.
func (e *Element) reRender() {
	if e.Node == nil || e.Def == nil || e.Def.Render == nil {
		return
	}
	children := e.Def.Render(e)
	sr := &ShadowRoot{Host: e.Node, Mode: "open", Styles: e.Def.Styles}
	e.Node.ShadowRoot = sr
	for _, c := range children {
		c.Parent = shadowRootNode(sr)
		sr.Children = append(sr.Children, c)
	}
	// upgrade nested custom elements produced by the re-render
	if e.Doc != nil && e.Doc.Registry != nil {
		defs := e.Doc.Registry.snapshot()
		for {
			targets := collectUpgradable(shadowRootNode(sr), defs)
			if len(targets) == 0 {
				break
			}
			rr := &Renderer{Reg: e.Doc.Registry}
			results := rr.renderWave(e.Doc, defs, targets)
			for i, t := range targets {
				attachShadow(e.Doc, defs, t, results[i])
			}
		}
	}
	assignSlots(e.Node)
}
