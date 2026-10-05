package wcr

import (
	"sort"
	"strings"
)

// scope.go implements shadow-DOM style isolation.
//
//   - Styles declared on a definition apply only inside that element's
//     shadow tree; selectors never match nodes outside it.
//   - Outer styles never cross a shadow boundary inward, except for the
//     fixed set of inherited CSS properties plus any properties the host
//     definition explicitly lists via InheritableProperties.
//   - Nested shadow trees isolate layer by layer: each boundary applies
//     the same rule independently.

// inheritedProperties is the set of CSS properties that may cross a shadow
// boundary inward by default (mirrors CSS inheritance).
var inheritedProperties = map[string]bool{
	"color": true, "font": true, "font-family": true, "font-size": true,
	"font-style": true, "font-weight": true, "line-height": true,
	"letter-spacing": true, "word-spacing": true, "text-align": true,
	"text-transform": true, "visibility": true, "white-space": true,
	"cursor": true, "direction": true,
}

// cssRule is one parsed selector{declarations} rule.
type cssRule struct {
	selector string
	decls    []Attr // property -> value, source order
}

// parseCSS splits css into rules. It is intentionally small: no @-rules,
// no comments with braces. Deterministic: source order is preserved.
func parseCSS(css string) []cssRule {
	var rules []cssRule
	rest := css
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			break
		}
		sel := strings.TrimSpace(rest[:i])
		rest = rest[i+1:]
		j := strings.IndexByte(rest, '}')
		if j < 0 {
			break
		}
		body := rest[:j]
		rest = rest[j+1:]
		if sel == "" || strings.HasPrefix(sel, "@") {
			continue
		}
		var decls []Attr
		for _, d := range strings.Split(body, ";") {
			kv := strings.SplitN(d, ":", 2)
			if len(kv) != 2 {
				continue
			}
			k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			if k != "" && v != "" {
				decls = append(decls, Attr{k, v})
			}
		}
		rules = append(rules, cssRule{selector: sel, decls: decls})
	}
	return rules
}

// ScopeStyles rewrites css so every selector is confined to the shadow tree
// of host. The emitted scope attribute is deterministic (derived from the
// host tag), so output does not depend on map or traversal order.
func ScopeStyles(css string, host *Node, inheritFrom []string) string {
	scope := "[data-wcr-scope=\"" + host.Tag + "\"]"
	var sb strings.Builder
	for _, r := range parseCSS(css) {
		sel := r.selector
		if strings.Contains(sel, ":host") {
			sel = strings.ReplaceAll(sel, ":host", scope)
		} else {
			sel = scope + " " + sel
		}
		sb.WriteString(sel)
		sb.WriteString(" { ")
		for i, d := range r.decls {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(d.Name + ": " + d.Value)
		}
		sb.WriteString(" }\n")
	}
	return sb.String()
}

// compound is one simple-selector compound like "div.cls#id" or ":host".
type compound struct {
	tag      string
	id       string
	classes  []string
	hostOnly bool
}

func parseCompound(s string) compound {
	c := compound{}
	s = strings.TrimSpace(s)
	if s == ":host" {
		c.hostOnly = true
		return c
	}
	for len(s) > 0 {
		switch {
		case s[0] == '.':
			end := strings.IndexAny(s[1:], ".#")
			if end < 0 {
				c.classes = append(c.classes, s[1:])
				s = ""
			} else {
				c.classes = append(c.classes, s[1:1+end])
				s = s[1+end:]
			}
		case s[0] == '#':
			end := strings.IndexAny(s[1:], ".#")
			if end < 0 {
				c.id = s[1:]
				s = ""
			} else {
				c.id = s[1 : 1+end]
				s = s[1+end:]
			}
		default:
			end := strings.IndexAny(s, ".#")
			if end < 0 {
				c.tag = s
				s = ""
			} else {
				c.tag = s[:end]
				s = s[end:]
			}
		}
	}
	return c
}

// matchCompound reports whether n matches c, given host is the shadow host
// of the tree n lives in.
func matchCompound(c compound, n, host *Node) bool {
	if c.hostOnly {
		return n == host
	}
	if n.Kind != ElementNode {
		return false
	}
	if c.tag != "" && n.Tag != c.tag {
		return false
	}
	if c.id != "" {
		id, ok := n.GetAttr("id")
		if !ok || id != c.id {
			return false
		}
	}
	for _, want := range c.classes {
		cls, ok := n.GetAttr("class")
		if !ok {
			return false
		}
		found := false
		for _, have := range strings.Fields(cls) {
			if have == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// matchSelector matches a (possibly descendant-combinator) selector against
// n, never crossing above host.
func matchSelector(sel string, n, host *Node) bool {
	parts := strings.Fields(sel)
	if len(parts) == 0 {
		return false
	}
	if !matchCompound(parseCompound(parts[len(parts)-1]), n, host) {
		return false
	}
	// walk ancestors for the remaining parts, right to left
	cur := n.Parent
	for i := len(parts) - 2; i >= 0; i-- {
		c := parseCompound(parts[i])
		matched := false
		for p := cur; p != nil && p != host.Parent; p = p.Parent {
			if matchCompound(c, p, host) {
				cur = p
				matched = true
				break
			}
			if p == host {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// hostOf returns the shadow host of the tree containing n, or nil if n is
// in the top-level document.
func hostOf(n *Node) *Node {
	for p := n; p != nil; p = p.Parent {
		if p.Kind == ElementNode && p.Tag == "#shadow-root" {
			return p.Parent
		}
	}
	return nil
}

// ComputedStyle resolves the effective declarations for n.
//
// Sources, weakest to strongest:
//  1. inherited properties crossing the shadow boundary (fixed inherited
//     set only — outer non-inherited declarations never leak in),
//  2. scoped rules of the enclosing shadow tree's host definition,
//  3. the element's inline style attribute.
//
// The result is a plain map; callers that serialize it must sort keys (the
// serializer does).
func ComputedStyle(n *Node) map[string]string {
	out := map[string]string{}
	host := hostOf(n)
	if host != nil {
		// 1. inheritance across the boundary: only inherited properties of
		// the host's own computed style are visible inside.
		outer := ComputedStyle(host)
		keys := make([]string, 0, len(outer))
		for k := range outer {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic application order
		for _, k := range keys {
			if inheritedProperties[k] {
				out[k] = outer[k]
			}
		}
		// 2. scoped rules from the host's definition.
		if host.elem != nil && host.elem.Def != nil {
			for _, r := range parseCSS(host.elem.Def.Styles) {
				if matchSelector(r.selector, n, host) {
					for _, d := range r.decls {
						out[d.Name] = d.Value
					}
				}
			}
		}
	}
	// 3. inline style.
	if style, ok := n.GetAttr("style"); ok {
		for _, d := range strings.Split(style, ";") {
			kv := strings.SplitN(d, ":", 2)
			if len(kv) == 2 {
				k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
				if k != "" && v != "" {
					out[k] = v
				}
			}
		}
	}
	return out
}
