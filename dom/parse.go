package dom

import (
	"fmt"
	"strings"
)

// Parse 解析 Serialize 产出的 HTML 子集：元素、特性、文本、注释、
// 以及声明式影子 DOM（<template shadowrootmode="open">）。
// 解析结果是确定性的，与 Serialize 互为往返。
func Parse(html string) (*Node, error) {
	p := &parser{s: html}
	doc := NewDocument()
	if err := p.parseChildren(doc, ""); err != nil {
		return nil, err
	}
	if p.i != len(p.s) {
		return nil, fmt.Errorf("dom: unexpected trailing input at %d", p.i)
	}
	return doc, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) parseChildren(parent *Node, closing string) error {
	for p.i < len(p.s) {
		if strings.HasPrefix(p.s[p.i:], "</") {
			end := strings.IndexByte(p.s[p.i:], '>')
			if end < 0 {
				return fmt.Errorf("dom: unterminated closing tag at %d", p.i)
			}
			name := strings.TrimSpace(p.s[p.i+2 : p.i+end])
			p.i += end + 1
			if name != closing {
				return fmt.Errorf("dom: mismatched closing tag %q, expected %q", name, closing)
			}
			return nil
		}
		if strings.HasPrefix(p.s[p.i:], "<!--") {
			end := strings.Index(p.s[p.i:], "-->")
			if end < 0 {
				return fmt.Errorf("dom: unterminated comment at %d", p.i)
			}
			parent.AppendChild(NewComment(p.s[p.i+4 : p.i+end]))
			p.i += end + 3
			continue
		}
		if p.s[p.i] == '<' {
			if err := p.parseElement(parent); err != nil {
				return err
			}
			continue
		}
		start := p.i
		for p.i < len(p.s) && p.s[p.i] != '<' {
			p.i++
		}
		parent.AppendChild(NewText(unescape(p.s[start:p.i])))
	}
	if closing != "" {
		return fmt.Errorf("dom: missing closing tag %q", closing)
	}
	return nil
}

func (p *parser) parseElement(parent *Node) error {
	p.i++ // consume '<'
	start := p.i
	for p.i < len(p.s) && !isSpace(p.s[p.i]) && p.s[p.i] != '>' {
		p.i++
	}
	tag := p.s[start:p.i]
	el := NewElement(tag)
	for {
		for p.i < len(p.s) && isSpace(p.s[p.i]) {
			p.i++
		}
		if p.i >= len(p.s) {
			return fmt.Errorf("dom: unterminated tag %q", tag)
		}
		if p.s[p.i] == '>' {
			p.i++
			break
		}
		astart := p.i
		for p.i < len(p.s) && p.s[p.i] != '=' && !isSpace(p.s[p.i]) && p.s[p.i] != '>' {
			p.i++
		}
		name := p.s[astart:p.i]
		value := ""
		if p.i < len(p.s) && p.s[p.i] == '=' {
			p.i++
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return fmt.Errorf("dom: attribute %q must be double-quoted", name)
			}
			p.i++
			vstart := p.i
			for p.i < len(p.s) && p.s[p.i] != '"' {
				p.i++
			}
			if p.i >= len(p.s) {
				return fmt.Errorf("dom: unterminated attribute %q", name)
			}
			value = unescape(p.s[vstart:p.i])
			p.i++
		}
		el.SetAttribute(name, value)
	}
	// 声明式影子 DOM：作为宿主首个子节点的 template 转换为影子根。
	if tag == "template" {
		if mode, ok := el.GetAttribute("shadowrootmode"); ok && mode == "open" {
			sr := NewShadowRoot()
			if err := p.parseChildren(sr, "template"); err != nil {
				return err
			}
			parent.Shadow = sr
			return nil
		}
	}
	parent.AppendChild(el)
	if voidElements[tag] {
		return nil
	}
	return p.parseChildren(el, tag)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

var unescaper = strings.NewReplacer(
	"&quot;", `"`,
	"&gt;", ">",
	"&lt;", "<",
	"&amp;", "&",
)

func unescape(s string) string { return unescaper.Replace(s) }
