package wc

import (
	"fmt"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// attachShadow 为宿主元素挂载空影子根。
func (rt *Runtime) attachShadow(el *Element) *dom.Node {
	sr := dom.NewShadowRoot()
	el.Node.Shadow = sr
	return sr
}

// buildShadow 构建影子树（纯计算，不修改宿主，可并发执行）。
// 返回 nil, nil 表示该元素没有影子树内容。
func (rt *Runtime) buildShadow(el *Element) (*dom.Node, error) {
	def := el.Def
	if def.Template == nil && def.Styles == "" {
		return nil, nil
	}
	sr := dom.NewShadowRoot()
	if def.Styles != "" {
		style := dom.NewElement("style")
		style.AppendChild(dom.NewText(ScopeCSS(def.Styles, def.Name)))
		sr.AppendChild(style)
	}
	if def.Template != nil {
		frag, err := safeTemplate(el)
		if err != nil {
			return nil, err
		}
		if frag != nil {
			if frag.Type == dom.DocumentNode {
				// 片段：子节点整体迁入影子树。
				for _, c := range append([]*dom.Node(nil), frag.Children...) {
					sr.AppendChild(c)
				}
			} else {
				sr.AppendChild(frag)
			}
		}
	}
	// 为影子内容打作用域标记，实现逐层样式隔离。
	dom.Walk(sr, func(n *dom.Node) {
		if n.Type == dom.ElementNode {
			n.SetAttribute(scopeAttr, def.Name)
		}
	})
	return sr, nil
}

// safeTemplate 调用模板并把 panic 转换为 error，实现异常隔离。
func safeTemplate(el *Element) (frag *dom.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("wc: template of %q panicked: %v", el.Def.Name, r)
		}
	}()
	return el.Def.Template(el)
}

// applyShadow 按降级策略把构建结果应用到宿主。
func (rt *Runtime) applyShadow(el *Element, sr *dom.Node, err error) error {
	if err != nil {
		el.Err = err
		switch el.Def.OnError {
		case FallbackPropagate:
			return err
		case FallbackEmpty:
			el.Node.Shadow = dom.NewShadowRoot()
		default: // FallbackLightDOM
			el.Node.Shadow = nil
			el.Node.RemoveAttribute(hostAttr)
		}
		return nil
	}
	el.Err = nil
	el.Node.Shadow = sr
	if sr != nil {
		el.Node.SetAttribute(hostAttr, el.Def.Name)
	} else {
		el.Node.RemoveAttribute(hostAttr)
	}
	return nil
}

// renderHost 渲染单个宿主的影子树，并升级影子内新出现的自定义元素。
// 单个元素失败只影响自身，不影响整棵树。
func (rt *Runtime) renderHost(el *Element) error {
	sr, err := rt.buildShadow(el)
	if aerr := rt.applyShadow(el, sr, err); aerr != nil {
		return aerr
	}
	if el.Node.Shadow != nil {
		_ = rt.Upgrade(el.Node.Shadow)
	}
	return nil
}
