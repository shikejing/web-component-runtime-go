package wc

import (
	"errors"
	"strings"
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

func TestErrorIsolationLightDOMFallback(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:    "x-bad",
		OnError: FallbackLightDOM,
		Template: func(el *Element) (*dom.Node, error) {
			return nil, errors.New("boom")
		},
	})
	_ = reg.Define(&Definition{
		Name: "x-good",
		Template: func(el *Element) (*dom.Node, error) {
			f := dom.NewElement("i")
			f.AppendChild(dom.NewText("ok"))
			return f, nil
		},
	})
	doc := dom.NewDocument()
	bad := doc.AppendChild(dom.NewElement("x-bad"))
	bad.AppendChild(dom.NewText("kept"))
	doc.AppendChild(dom.NewElement("x-good"))

	out, err := NewRenderer(NewRuntime(reg)).Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	// 失败元素降级为仅轻 DOM，不影响兄弟元素。
	if !strings.Contains(out, "<x-bad>kept</x-bad>") {
		t.Fatalf("light dom fallback wrong: %s", out)
	}
	if !strings.Contains(out, "<i") || !strings.Contains(out, "ok") {
		t.Fatalf("sibling affected: %s", out)
	}
}

func TestErrorIsolationPanicAndEmptyFallback(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:    "x-panic",
		OnError: FallbackEmpty,
		Template: func(el *Element) (*dom.Node, error) {
			panic("kaboom")
		},
	})
	doc := dom.NewDocument()
	doc.AppendChild(dom.NewElement("x-panic"))
	out, err := NewRenderer(NewRuntime(reg)).Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<template shadowrootmode="open"></template>`) {
		t.Fatalf("empty fallback wrong: %s", out)
	}
}

func TestErrorPropagate(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:    "x-fatal",
		OnError: FallbackPropagate,
		Template: func(el *Element) (*dom.Node, error) {
			return nil, errors.New("fatal")
		},
	})
	doc := dom.NewDocument()
	doc.AppendChild(dom.NewElement("x-fatal"))
	if _, err := NewRenderer(NewRuntime(reg)).Render(doc); err == nil {
		t.Fatal("expected propagated error")
	}
}

func TestDeterministicOutput(t *testing.T) {
	build := func() *dom.Node {
		doc := dom.NewDocument()
		for _, tag := range []string{"x-one", "x-two", "x-three"} {
			el := doc.AppendChild(dom.NewElement(tag))
			el.SetAttribute("z", "1")
			el.SetAttribute("a", "2")
			el.SetAttribute("m", "3")
		}
		return doc
	}
	newReg := func() *Registry {
		reg := NewRegistry()
		for _, tag := range []string{"x-one", "x-two", "x-three"} {
			name := tag
			_ = reg.Define(&Definition{
				Name:    name,
				Reflect: map[string]string{"p1": "a", "p2": "m", "p3": "z"},
				Template: func(el *Element) (*dom.Node, error) {
					f := dom.NewElement("u")
					f.AppendChild(dom.NewText(name))
					return f, nil
				},
			})
		}
		return reg
	}
	base, err := NewRenderer(NewRuntime(newReg())).Render(build())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, err := NewRenderer(NewRuntime(newReg())).Render(build())
		if err != nil {
			t.Fatal(err)
		}
		if got != base {
			t.Fatalf("run %d differs:\n%s\n%s", i, base, got)
		}
	}
	// 并发渲染逐字节一致。
	for _, workers := range []int{2, 4, 16} {
		got, err := NewRenderer(NewRuntime(newReg())).RenderConcurrent(build(), workers)
		if err != nil {
			t.Fatal(err)
		}
		if got != base {
			t.Fatalf("workers=%d differs:\n%s\n%s", workers, base, got)
		}
	}
}

func TestDefineDuringRenderDoesNotAffectOngoing(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name: "x-first",
		Template: func(el *Element) (*dom.Node, error) {
			// 渲染途中注册新定义。
			_ = reg.Define(&Definition{
				Name: "x-late",
				Template: func(el *Element) (*dom.Node, error) {
					f := dom.NewElement("b")
					f.AppendChild(dom.NewText("late"))
					return f, nil
				},
			})
			f := dom.NewElement("s")
			f.AppendChild(dom.NewText("first"))
			return f, nil
		},
	})
	doc := dom.NewDocument()
	doc.AppendChild(dom.NewElement("x-first"))
	doc.AppendChild(dom.NewElement("x-late"))
	out, err := NewRenderer(NewRuntime(reg)).Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	// 快照语义：x-late 在本次渲染中保持未升级，不丢节点、不错位。
	if !strings.Contains(out, "<x-late></x-late>") {
		t.Fatalf("late element should remain unupgraded: %s", out)
	}
	// 渲染结束后新定义可用。
	doc2 := dom.NewDocument()
	doc2.AppendChild(dom.NewElement("x-late"))
	out2, err := NewRenderer(NewRuntime(reg)).Render(doc2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2, "late</b>") {
		t.Fatalf("late definition should work after registration: %s", out2)
	}
}
