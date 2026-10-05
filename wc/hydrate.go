package wc

import "github.com/shikejing/web-component-runtime-go/dom"

// Hydrate 激活服务端渲染产物：把声明式影子 DOM 接管为影子树，
// 只升级实例与派发生命周期，不重新渲染已接管的影子树。
// 重复激活是幂等的；单个元素的接管是原子的，不会留下半成品状态。
func Hydrate(rt *Runtime, root *dom.Node) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.defs = rt.reg.snapshot()
	defer func() { rt.defs = nil }()

	adoptDeclarativeShadows(root)
	if err := rt.Upgrade(root); err != nil {
		return err
	}
	// 仅渲染没有可接管影子树的元素（例如服务端降级过的元素）。
	rt.Flush()
	return nil
}

// adoptDeclarativeShadows 把 <template shadowrootmode="open"> 就地转换为影子根。
// 每个宿主的转换先完整构建再一次性挂载，中途失败不会留下半成品。
func adoptDeclarativeShadows(n *dom.Node) {
	if n.Type == dom.ElementNode && len(n.Children) > 0 {
		first := n.Children[0]
		if first.Type == dom.ElementNode && first.Tag == "template" {
			if mode, ok := first.GetAttribute("shadowrootmode"); ok && mode == "open" {
				sr := dom.NewShadowRoot()
				for _, c := range append([]*dom.Node(nil), first.Children...) {
					sr.AppendChild(c)
				}
				n.RemoveChild(first)
				n.Shadow = sr
				adoptDeclarativeShadows(sr)
			}
		}
	}
	for _, c := range append([]*dom.Node(nil), n.Children...) {
		adoptDeclarativeShadows(c)
	}
}
