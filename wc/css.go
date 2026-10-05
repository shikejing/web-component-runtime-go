package wc

import "strings"

// inheritableProps 是允许穿透影子边界的可继承 CSS 属性集合。
var inheritableProps = map[string]bool{
	"color": true, "cursor": true, "direction": true,
	"font": true, "font-family": true, "font-size": true,
	"font-style": true, "font-weight": true, "font-variant": true,
	"letter-spacing": true, "line-height": true, "quotes": true,
	"tab-size": true, "text-align": true, "text-indent": true,
	"text-transform": true, "visibility": true, "white-space": true,
	"word-break": true, "word-spacing": true,
}

// IsInheritable 判断 CSS 属性是否为可继承属性。
func IsInheritable(prop string) bool {
	return inheritableProps[strings.TrimSpace(strings.ToLower(prop))]
}

// FilterInherited 从声明集中筛出允许穿透影子边界的可继承属性。
func FilterInherited(decls map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range decls {
		if IsInheritable(k) {
			out[k] = v
		}
	}
	return out
}

// scopeAttr 返回影子内容元素携带的作用域标记特性名。
const scopeAttr = "data-wc-scope"

// hostAttr 返回宿主元素携带的作用域标记特性名。
const hostAttr = "data-wc-host"

// ScopeCSS 将影子树内的 CSS 选择器改写为带作用域标记的形式，
// 保证内部样式不泄漏到外部、外部样式（除可继承属性外）不穿透边界。
// scope 为确定性的作用域令牌（通常取元素名），与遍历顺序无关。
func ScopeCSS(css, scope string) string {
	var b strings.Builder
	rest := css
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			break
		}
		selectors := strings.TrimSpace(rest[:i])
		rest = rest[i+1:]
		j := strings.Index(rest, "}")
		if j < 0 {
			break
		}
		decls := strings.TrimSpace(rest[:j])
		rest = rest[j+1:]
		b.WriteString(scopeSelectors(selectors, scope))
		b.WriteString(" { ")
		b.WriteString(decls)
		b.WriteString(" }\n")
	}
	return b.String()
}

// scopeSelectors 为逗号分隔的选择器逐一加上作用域标记。
func scopeSelectors(selectors, scope string) string {
	parts := strings.Split(selectors, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, scopeCompound(strings.TrimSpace(p), scope))
	}
	return strings.Join(out, ", ")
}

// scopeCompound 为单个选择器的每个复合段加作用域标记。
func scopeCompound(sel, scope string) string {
	if sel == ":host" {
		return "[" + hostAttr + `="` + scope + `"]`
	}
	if strings.HasPrefix(sel, ":host(") && strings.HasSuffix(sel, ")") {
		inner := sel[len(":host(") : len(sel)-1]
		return "[" + hostAttr + `="` + scope + `"]` + inner
	}
	segs := strings.Fields(sel)
	for i, s := range segs {
		if s == ">" || s == "+" || s == "~" {
			continue // 组合器不加标记
		}
		segs[i] = s + "[" + scopeAttr + `="` + scope + `"]`
	}
	return strings.Join(segs, " ")
}
