package wc

import (
	"strings"
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

func TestScopeCSS(t *testing.T) {
	out := ScopeCSS(`div, .a > p { color: red } :host { display: block } :host(.on) { color: blue }`, "x-el")
	for _, want := range []string{
		`div[data-wc-scope="x-el"]`,
		`.a[data-wc-scope="x-el"] > p[data-wc-scope="x-el"]`,
		`[data-wc-host="x-el"] { display: block }`,
		`[data-wc-host="x-el"].on { color: blue }`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("scoped css missing %q:\n%s", want, out)
		}
	}
	// 确定性：同一输入多次调用结果一致。
	if out != ScopeCSS(`div, .a > p { color: red } :host { display: block } :host(.on) { color: blue }`, "x-el") {
		t.Fatal("ScopeCSS not deterministic")
	}
}

func TestInheritableFilter(t *testing.T) {
	if !IsInheritable("color") || !IsInheritable("font-size") {
		t.Fatal("color/font-size should be inheritable")
	}
	if IsInheritable("margin") || IsInheritable("border") {
		t.Fatal("margin/border should not be inheritable")
	}
	got := FilterInherited(map[string]string{
		"color": "red", "margin": "1px", "font-family": "serif",
	})
	if len(got) != 2 || got["color"] != "red" || got["font-family"] != "serif" {
		t.Fatalf("unexpected filter result: %v", got)
	}
}

func TestShadowScopeMarkersInOutput(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:   "x-outer",
		Styles: "span { color: red }",
		Template: func(el *Element) (*dom.Node, error) {
			f := dom.NewElement("div")
			f.AppendChild(dom.NewElement("span"))
			f.AppendChild(dom.NewElement("x-inner"))
			return f, nil
		},
	})
	_ = reg.Define(&Definition{
		Name:   "x-inner",
		Styles: "em { color: blue }",
		Template: func(el *Element) (*dom.Node, error) {
			f := dom.NewElement("p")
			f.AppendChild(dom.NewElement("em"))
			return f, nil
		},
	})
	doc := dom.NewDocument()
	doc.AppendChild(dom.NewElement("x-outer"))
	out, err := NewRenderer(NewRuntime(reg)).Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	// 外层影子内容带外层作用域标记，内层带内层标记：逐层隔离。
	if !strings.Contains(out, `span[data-wc-scope="x-outer"]`) {
		t.Fatalf("outer scope marker missing:\n%s", out)
	}
	if !strings.Contains(out, `em[data-wc-scope="x-inner"]`) {
		t.Fatalf("inner scope marker missing:\n%s", out)
	}
	// 外层样式选择器只命中外层作用域，内层同理。
	if !strings.Contains(out, `span[data-wc-scope="x-outer"] { color: red }`) {
		t.Fatalf("outer scoped style wrong:\n%s", out)
	}
	if !strings.Contains(out, `em[data-wc-scope="x-inner"] { color: blue }`) {
		t.Fatalf("inner scoped style wrong:\n%s", out)
	}
	// 宿主标记。
	if !strings.Contains(out, `data-wc-host="x-outer"`) || !strings.Contains(out, `data-wc-host="x-inner"`) {
		t.Fatalf("host markers missing:\n%s", out)
	}
}
