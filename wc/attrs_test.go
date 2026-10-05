package wc

import (
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

func setupReflect(t *testing.T, renders *int) (*Runtime, *dom.Node) {
	t.Helper()
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name:               "x-ref",
		ObservedAttributes: []string{"value"},
		Reflect:            map[string]string{"value": "value"},
		Template: func(el *Element) (*dom.Node, error) {
			*renders++
			f := dom.NewElement("span")
			if v, ok := el.Props["value"]; ok {
				f.AppendChild(dom.NewText(v.(string)))
			}
			return f, nil
		},
	})
	doc := dom.NewDocument()
	host := doc.AppendChild(dom.NewElement("x-ref"))
	rt := NewRuntime(reg)
	if err := rt.Upgrade(doc); err != nil {
		t.Fatal(err)
	}
	rt.Flush()
	return rt, host
}

func TestPropertyReflectsToAttribute(t *testing.T) {
	var renders int
	rt, host := setupReflect(t, &renders)
	rt.SetProperty(host, "value", "hello")
	rt.Flush()
	if v, _ := host.GetAttribute("value"); v != "hello" {
		t.Fatalf("attribute not reflected, got %q", v)
	}
}

func TestAttributeSyncsProperty(t *testing.T) {
	var renders int
	rt, host := setupReflect(t, &renders)
	// 直接改写特性，属性也要保持一致。
	rt.SetAttribute(host, "value", "world")
	rt.Flush()
	if v, _ := rt.Property(host, "value"); v != "world" {
		t.Fatalf("property not synced, got %v", v)
	}
}

func TestObservedAttributeCallback(t *testing.T) {
	reg := NewRegistry()
	type change struct{ name, old, new string }
	var changes []change
	_ = reg.Define(&Definition{
		Name:               "x-obs",
		ObservedAttributes: []string{"v"},
		Lifecycle: Lifecycle{
			AttributeChanged: func(el *Element, name, old, new string) {
				changes = append(changes, change{name, old, new})
			},
		},
	})
	doc := dom.NewDocument()
	host := doc.AppendChild(dom.NewElement("x-obs"))
	rt := NewRuntime(reg)
	_ = rt.Upgrade(doc)
	rt.SetAttribute(host, "v", "1")
	rt.SetAttribute(host, "v", "2")
	rt.RemoveAttribute(host, "v")
	want := []change{{"v", "", "1"}, {"v", "1", "2"}, {"v", "2", ""}}
	if len(changes) != len(want) {
		t.Fatalf("got %v, want %v", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("got %v, want %v", changes, want)
		}
	}
}

func TestSameRoundChangesCoalesce(t *testing.T) {
	var renders int
	rt, host := setupReflect(t, &renders)
	base := renders
	// 同一轮内对同一元素的多处改动必须合并成一次重渲染。
	rt.SetAttribute(host, "value", "a")
	rt.SetAttribute(host, "value", "b")
	rt.SetProperty(host, "value", "c")
	rt.SetAttribute(host, "other", "x")
	rt.Flush()
	if renders != base+1 {
		t.Fatalf("expected 1 re-render, got %d", renders-base)
	}
	// 最终状态一致，不暴露中间态。
	if v, _ := host.GetAttribute("value"); v != "c" {
		t.Fatalf("final attribute = %q", v)
	}
	flat := Flatten(host)
	if got := dom.Serialize(flat); got != `<x-ref data-wc-host="x-ref" other="x" value="c"><span data-wc-scope="x-ref">c</span></x-ref>` {
		t.Fatalf("final render = %s", got)
	}
}
