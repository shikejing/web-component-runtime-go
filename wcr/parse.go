package wcr

import (
	"fmt"
	"strings"
)

// parse.go is a small HTML parser covering exactly what Serialize emits:
// elements, attributes, text, comments, void elements and declarative
// shadow DOM (<template shadowrootmode>). It lets the full
// render -> serialize -> parse -> hydrate chain run and be verified locally.

// Parse parses HTML into a tree rooted at a "#document" node. Declarative
// shadow DOM templates are attached to their host as a ShadowRoot.
func Parse(html string) (*Node, error) {
	p := &parser{in: html}
	root := NewElement("#document")
	if err := p.parseChildren(root, ""); err != nil {
		return nil, err
	}
	return root, nil
}

type parser struct {
	in  string
	pos int
}

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("wcr: parse error at offset %d: %s", p.pos, fmt.Sprintf(format, args...))
}

func (p *parser) parseChildren(parent *Node, closing string) error {
	for p.pos < len(p.in) {
		if closing != "" && strings.HasPrefix(p.in[p.pos:], "</"+closing) {
			p.pos += len(closing) + 2
			if err := p.expect('>'); err != nil {
				return err
			}
			return nil
		}
		if p.in[p.pos] != '<' {
			start := p.pos
			for p.pos < len(p.in) && p.in[p.pos] != '<' {
				p.pos++
			}
			parent.AppendChild(NewText(unescape(p.in[start:p.pos])))
			continue
		}
		switch {
		case strings.HasPrefix(p.in[p.pos:], "<!--"):
			end := strings.Index(p.in[p.pos+4:], "-->")
			if end < 0 {
				return p.errf("unterminated comment")
			}
			parent.AppendChild(&Node{Kind: CommentNode, Text: p.in[p.pos+4 : p.pos+4+end]})
			p.pos += 4 + end + 3
		case strings.HasPrefix(p.in[p.pos:], "</"):
			return p.errf("unexpected closing tag")
		default:
			if err := p.parseElement(parent); err != nil {
				return err
			}
		}
	}
	if closing != "" {
		return p.errf("missing closing tag </%s>", closing)
	}
	return nil
}

func (p *parser) parseElement(parent *Node) error {
	p.pos++ // consume '<'
	start := p.pos
	for p.pos < len(p.in) && !strings.ContainsRune(" \t\r\n/>", rune(p.in[p.pos])) {
		p.pos++
	}
	tag := strings.ToLower(p.in[start:p.pos])
	if tag == "" {
		return p.errf("empty tag name")
	}
	el := NewElement(tag)
	for {
		p.skipSpace()
		if p.pos >= len(p.in) {
			return p.errf("unterminated tag <%s>", tag)
		}
		if p.in[p.pos] == '>' {
			p.pos++
			break
		}
		if strings.HasPrefix(p.in[p.pos:], "/>") {
			p.pos += 2
			parent.AppendChild(el)
			return nil
		}
		name, value, err := p.parseAttr()
		if err != nil {
			return err
		}
		el.SetAttr(name, value)
	}
	if voidElements[tag] {
		parent.AppendChild(el)
		return nil
	}
	if tag == "style" || tag == "script" {
		// raw text until the matching close tag
		close := "</" + tag + ">"
		idx := strings.Index(strings.ToLower(p.in[p.pos:]), close)
		if idx < 0 {
			return p.errf("unterminated <%s>", tag)
		}
		el.AppendChild(NewText(p.in[p.pos : p.pos+idx]))
		p.pos += idx + len(close)
		parent.AppendChild(el)
		return nil
	}
	if err := p.parseChildren(el, tag); err != nil {
		return err
	}
	// declarative shadow DOM: hoist <template shadowrootmode> into ShadowRoot
	if tag == "template" {
		if mode, ok := el.GetAttr("shadowrootmode"); ok {
			sr := &ShadowRoot{Mode: mode}
			// Shadow children (including any <style>) are kept as regular
			// children so a parse->serialize round trip is byte-stable.
			sr.Children = append(sr.Children, el.Children...)
			// the template's parent element is the shadow host
			host := parent
			if host.Kind != ElementNode || host.Tag == "#document" {
				return p.errf("shadowroot template without a host element")
			}
			host.ShadowRoot = sr
			sr.Host = host
			for _, c := range sr.Children {
				c.Parent = shadowRootNode(sr)
			}
			return nil // template itself is not added to the tree
		}
	}
	parent.AppendChild(el)
	return nil
}

func (p *parser) parseAttr() (string, string, error) {
	start := p.pos
	for p.pos < len(p.in) && !strings.ContainsRune("= \t\r\n/>", rune(p.in[p.pos])) {
		p.pos++
	}
	name := p.in[start:p.pos]
	if name == "" {
		return "", "", p.errf("bad attribute")
	}
	p.skipSpace()
	if p.pos >= len(p.in) || p.in[p.pos] != '=' {
		return name, "", nil
	}
	p.pos++
	p.skipSpace()
	if p.pos >= len(p.in) {
		return "", "", p.errf("unterminated attribute %q", name)
	}
	q := p.in[p.pos]
	if q == '"' || q == '\'' {
		p.pos++
		end := strings.IndexByte(p.in[p.pos:], q)
		if end < 0 {
			return "", "", p.errf("unterminated attribute %q", name)
		}
		v := unescape(p.in[p.pos : p.pos+end])
		p.pos += end + 1
		return name, v, nil
	}
	start = p.pos
	for p.pos < len(p.in) && !strings.ContainsRune(" \t\r\n>", rune(p.in[p.pos])) {
		p.pos++
	}
	return name, unescape(p.in[start:p.pos]), nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.in) && strings.ContainsRune(" \t\r\n", rune(p.in[p.pos])) {
		p.pos++
	}
}

func (p *parser) expect(c byte) error {
	p.skipSpace()
	if p.pos >= len(p.in) || p.in[p.pos] != c {
		return p.errf("expected %q", string(c))
	}
	p.pos++
	return nil
}

func unescape(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
	return r.Replace(s)
}
