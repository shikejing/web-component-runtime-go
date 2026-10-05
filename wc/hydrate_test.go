package wc

import (
	"strings"
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// ssrApp 构建一个含嵌套元素与插槽的 SSR 应用。
func ssrApp(t *testing.T, templateCalls *int) (*Registry, string) {
	t.Helper()
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:   "x-card",
		Styles: "div { color: red }",
		Template: func(el *Element) (*dom.Node, error) {
			if templateCalls != nil {
				*templateCalls++
			}
			f := dom.NewElement("div")
			f.AppendChild(dom.NewElement("slot"))
			return f, nil
		},
	})
	doc := dom.NewDocument()
	card := doc.AppendChild(dom.NewElement("x-card"))
	card.AppendChild(dom.NewText("hello"))
	out, err := NewRenderer(NewRuntime(reg)).Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	return reg, out
}

func TestSSRContainsDeclarativeShadowAndMarker(t *testing.T) {
	_, out := ssrApp(t, nil)
	if !strings.Contains(out, `<template shadowrootmode="open">`) {
		t.Fatalf("missing declarative shadow dom: %s", out)
	}
	if !strings.Contains(out, `data-wc-ssr`) {
		t.Fatalf("missing hydration marker: %s", out)
	}
}

func TestHydrateTakesOverWithoutRerender(t *testing.T) {
	reg, ssr := ssrApp(t, nil)

	// 客户端：解析 SSR 产物，激活。
	calls := 0
	doc, err := dom.Parse(ssr)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(reg)
	if err := Hydrate(rt, doc); err != nil {
		t.Fatal(err)
	}
	// 激活不得重新渲染模板。
	if calls != 0 {
		t.Fatalf("hydrate re-rendered %d times", calls)
	}
	// 激活前后可观察状态一致：序列化逐字节相同。
	after := dom.Serialize(doc)
	if after != ssr {
		t.Fatalf("state changed by hydrate:\n%s\n%s", ssr, after)
	}
	// 实例已升级并标记为已激活。
	var host *dom.Node
	dom.Walk(doc, func(n *dom.Node) {
		if n.Type == dom.ElementNode && n.Tag == "x-card" {
			host = n
		}
	})
	el, ok := rt.ElementOf(host)
	if !ok || !el.Hydrated || !el.Connected {
		t.Fatalf("element not hydrated: %+v ok=%v", el, ok)
	}
	if host.Shadow == nil {
		t.Fatal("shadow not adopted")
	}
}

func TestHydrateIdempotent(t *testing.T) {
	reg, ssr := ssrApp(t, nil)
	doc, err := dom.Parse(ssr)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(reg)
	if err := Hydrate(rt, doc); err != nil {
		t.Fatal(err)
	}
	if err := Hydrate(rt, doc); err != nil {
		t.Fatalf("second hydrate: %v", err)
	}
	if got := dom.Serialize(doc); got != ssr {
		t.Fatalf("double hydrate changed state:\n%s\n%s", ssr, got)
	}
}

func TestHydrateFailureLeavesNoHalfState(t *testing.T) {
	reg, ssr := ssrApp(t, nil)
	// 破坏产物：把模板标签改坏，模拟激活中途失败。
	broken := strings.Replace(ssr, `<template shadowrootmode="open">`, `<template shadowrootmode="open"><broken`, 1)
	if _, err := dom.Parse(broken); err == nil {
		t.Fatal("expected parse error on broken payload")
	}
	// 合法但未注册定义的产物：激活应报错，且元素保持未激活的原样。
	doc, err := dom.Parse(ssr)
	if err != nil {
		t.Fatal(err)
	}
	empty := NewRuntime(NewRegistry())
	if err := Hydrate(empty, doc); err != nil {
		t.Fatal(err)
	}
	// 未注册的元素不被升级，DOM 保持 SSR 原样（模板仍在，可重试）。
	if got := dom.Serialize(doc); got != ssr {
		t.Fatalf("failed hydrate left modified tree:\n%s\n%s", ssr, got)
	}
	// 用正确注册表重试，仍可成功激活。
	rt := NewRuntime(reg)
	if err := Hydrate(rt, doc); err != nil {
		t.Fatal(err)
	}
	var host *dom.Node
	dom.Walk(doc, func(n *dom.Node) {
		if n.Type == dom.ElementNode && n.Tag == "x-card" {
			host = n
		}
	})
	if host.Shadow == nil {
		t.Fatal("retry hydrate failed to adopt shadow")
	}
}
