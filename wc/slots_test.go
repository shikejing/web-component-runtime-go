package wc

import (
	"strings"
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// slotHost 构建带默认插槽、具名插槽与回退内容的宿主。
func slotHost(t *testing.T) (*Runtime, *dom.Node) {
	t.Helper()
	reg := NewRegistry()
	_ = reg.Define(&Definition{
		Name: "x-slot",
		Template: func(el *Element) (*dom.Node, error) {
			f := dom.NewElement("div")
			def := dom.NewElement("slot")
			named := dom.NewElement("slot")
			named.SetAttribute("name", "title")
			fb := dom.NewElement("slot")
			fb.SetAttribute("name", "empty")
			fb.AppendChild(dom.NewText("fallback"))
			f.AppendChild(named)
			f.AppendChild(def)
			f.AppendChild(fb)
			return f, nil
		},
	})
	doc := dom.NewDocument()
	host := doc.AppendChild(dom.NewElement("x-slot"))
	rt := NewRuntime(reg)
	if _, err := NewRenderer(rt).Render(doc); err != nil {
		t.Fatal(err)
	}
	return rt, host
}

func TestSlotDistribution(t *testing.T) {
	_, host := slotHost(t)
	title := dom.NewElement("h1")
	title.SetAttribute("slot", "title")
	host.AppendChild(title)
	host.AppendChild(dom.NewText("light"))
	orphan := dom.NewElement("b")
	orphan.SetAttribute("slot", "nowhere")
	host.AppendChild(orphan)

	var named, def, fallback, unmatched int
	for _, a := range Distribute(host) {
		switch {
		case a.Slot == nil:
			unmatched = len(a.Nodes)
		case a.Slot.Attributes() != nil && hasAttrVal(a.Slot, "name", "title"):
			named = len(a.Nodes)
		case hasAttrVal(a.Slot, "name", "empty"):
			fallback = len(a.Nodes)
		default:
			def = len(a.Nodes)
		}
	}
	if named != 1 || def != 1 || fallback != 0 || unmatched != 1 {
		t.Fatalf("named=%d default=%d fallback=%d unmatched=%d", named, def, fallback, unmatched)
	}
}

func hasAttrVal(n *dom.Node, name, val string) bool {
	v, ok := n.GetAttribute(name)
	return ok && v == val
}

func TestSlotFlattenAndFallback(t *testing.T) {
	_, host := slotHost(t)
	title := dom.NewElement("h1")
	title.SetAttribute("slot", "title")
	title.AppendChild(dom.NewText("T"))
	host.AppendChild(title)
	host.AppendChild(dom.NewText("body"))

	flat := Flatten(host)
	out := dom.Serialize(flat)
	// 具名插槽收到 h1，默认插槽收到文本，空插槽用回退内容。
	if !strings.Contains(out, "<h1") || !strings.Contains(out, "body") || !strings.Contains(out, "fallback") {
		t.Fatalf("flatten wrong: %s", out)
	}
	// 组合树中不应再出现 slot 元素。
	if strings.Contains(out, "<slot") {
		t.Fatalf("slot should be replaced: %s", out)
	}
}

func TestSlotRedistributeOnDynamicChange(t *testing.T) {
	rt, host := slotHost(t)
	child := dom.NewElement("span")
	child.AppendChild(dom.NewText("dyn"))
	host.AppendChild(child)

	count := func() int {
		for _, a := range Distribute(host) {
			if a.Slot != nil && hasAttrVal(a.Slot, "name", "title") {
				return len(a.Nodes)
			}
		}
		return -1
	}
	if got := count(); got != 0 {
		t.Fatalf("before change named=%d", got)
	}
	// 渲染后动态设置 slot 特性，分发结果随之更新。
	rt.SetAttribute(child, "slot", "title")
	rt.Flush()
	if got := count(); got != 1 {
		t.Fatalf("after change named=%d", got)
	}
}
