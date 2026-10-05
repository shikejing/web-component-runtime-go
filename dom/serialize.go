package dom

import (
	"sort"
	"strings"
)

// voidElements 是无子节点的 HTML 空元素。
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// Serialize 序列化节点及其子树。特性按名称排序以保证逐字节确定性。
// 影子树序列化为声明式影子 DOM（<template shadowrootmode="open">）。
func Serialize(n *Node) string {
	var b strings.Builder
	writeNode(&b, n)
	return b.String()
}

// SerializeChildren 仅序列化子节点。
func SerializeChildren(n *Node) string {
	var b strings.Builder
	for _, c := range n.Children {
		writeNode(&b, c)
	}
	return b.String()
}

func writeNode(b *strings.Builder, n *Node) {
	switch n.Type {
	case DocumentNode:
		for _, c := range n.Children {
			writeNode(b, c)
		}
	case ShadowRootNode:
		b.WriteString(`<template shadowrootmode="open">`)
		for _, c := range n.Children {
			writeNode(b, c)
		}
		b.WriteString(`</template>`)
	case TextNode:
		b.WriteString(EscapeText(n.Data))
	case CommentNode:
		b.WriteString("<!--")
		b.WriteString(n.Data)
		b.WriteString("-->")
	case ElementNode:
		b.WriteByte('<')
		b.WriteString(n.Tag)
		attrs := n.Attributes()
		sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].Name < attrs[j].Name })
		for _, a := range attrs {
			b.WriteByte(' ')
			b.WriteString(a.Name)
			b.WriteString(`="`)
			b.WriteString(EscapeAttr(a.Value))
			b.WriteByte('"')
		}
		b.WriteByte('>')
		if n.Shadow != nil {
			writeNode(b, n.Shadow)
		}
		if !voidElements[n.Tag] {
			for _, c := range n.Children {
				writeNode(b, c)
			}
			b.WriteString("</")
			b.WriteString(n.Tag)
			b.WriteByte('>')
		}
	}
}

var textEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

var attrEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
)

// EscapeText 转义文本内容。
func EscapeText(s string) string { return textEscaper.Replace(s) }

// EscapeAttr 转义特性值。
func EscapeAttr(s string) string { return attrEscaper.Replace(s) }
