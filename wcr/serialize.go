package wcr

import (
	"strconv"
	"strings"
)

// serialize.go produces deterministic HTML, optionally as declarative
// shadow DOM with hydration markers.
//
// Determinism: attributes are emitted in insertion order, hydration ids are
// assigned by a document-order walk, and no map iteration is involved
// anywhere in the output path — identical trees always serialize to
// identical bytes.

// SerializeOptions tunes HTML output.
type SerializeOptions struct {
	// DeclarativeShadowDOM emits <template shadowrootmode> wrappers so the
	// shadow tree is part of the HTML payload.
	DeclarativeShadowDOM bool
	// HydrationMarker adds data-wcr-id activation markers to upgraded
	// custom elements, in document order starting at 0.
	HydrationMarker bool
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"source": true, "track": true, "wbr": true,
}

// Serialize renders root to deterministic HTML.
func Serialize(root *Node, opts SerializeOptions) string {
	s := &serializer{opts: opts}
	s.node(root)
	return s.sb.String()
}

type serializer struct {
	sb   strings.Builder
	opts SerializeOptions
	next int // hydration id counter
}

func (s *serializer) node(n *Node) {
	switch n.Kind {
	case TextNode:
		s.sb.WriteString(escapeText(n.Text))
	case CommentNode:
		s.sb.WriteString("<!--" + n.Text + "-->")
	case ElementNode:
		if n.Tag == "#document" || n.Tag == "#shadow-root" {
			s.children(n.Children)
			return
		}
		s.element(n)
	}
}

func (s *serializer) element(n *Node) {
	s.sb.WriteByte('<')
	s.sb.WriteString(n.Tag)
	if n.ShadowRoot != nil {
		// scope marker confines the element's styles to its shadow tree
		s.sb.WriteString(` data-wcr-scope="`)
		s.sb.WriteString(n.Tag)
		s.sb.WriteByte('"')
	}
	if s.opts.HydrationMarker {
		// upgraded elements get a fresh document-order id; elements parsed
		// from server HTML keep the id the server assigned them
		if n.elem != nil {
			s.sb.WriteString(` data-wcr-id="`)
			s.sb.WriteString(strconv.Itoa(s.next))
			s.sb.WriteByte('"')
			s.next++
		} else if id, ok := n.GetAttr("data-wcr-id"); ok {
			s.sb.WriteString(` data-wcr-id="`)
			s.sb.WriteString(id)
			s.sb.WriteByte('"')
		}
	}
	for _, a := range n.Attrs {
		// runtime-owned markers are re-synthesized above; never duplicated
		if a.Name == "data-wcr-scope" || a.Name == "data-wcr-id" {
			continue
		}
		s.sb.WriteByte(' ')
		s.sb.WriteString(a.Name)
		s.sb.WriteString(`="`)
		s.sb.WriteString(escapeAttr(a.Value))
		s.sb.WriteByte('"')
	}
	s.sb.WriteByte('>')
	if voidElements[n.Tag] {
		return
	}
	if n.ShadowRoot != nil && s.opts.DeclarativeShadowDOM {
		s.sb.WriteString(`<template shadowrootmode="`)
		s.sb.WriteString(n.ShadowRoot.Mode)
		s.sb.WriteString(`">`)
		if n.ShadowRoot.Styles != "" {
			s.sb.WriteString("<style>")
			s.sb.WriteString(ScopeStyles(n.ShadowRoot.Styles, n, nil))
			s.sb.WriteString("</style>")
		}
		s.children(n.ShadowRoot.Children)
		s.sb.WriteString("</template>")
	}
	s.children(n.Children)
	s.sb.WriteString("</")
	s.sb.WriteString(n.Tag)
	s.sb.WriteByte('>')
}

func (s *serializer) children(ns []*Node) {
	for _, c := range ns {
		s.node(c)
	}
}

func escapeText(t string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(t)
}

func escapeAttr(t string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(t)
}
