package wc

import (
	"sort"
	"sync"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// ssrAttr 是服务端渲染产物上的激活标记特性。
const ssrAttr = "data-wc-ssr"

// Renderer 执行服务端渲染，产出含声明式影子 DOM 的确定性 HTML。
type Renderer struct {
	rt *Runtime
}

// NewRenderer 创建渲染器。
func NewRenderer(rt *Runtime) *Renderer { return &Renderer{rt: rt} }

// Render 渲染整棵树。渲染开始时对注册表做快照，
// 渲染途中新增或替换定义不影响本次渲染。
func (r *Renderer) Render(root *dom.Node) (string, error) {
	return r.render(root, 1)
}

// RenderConcurrent 以给定并发度渲染，输出与 Render 逐字节一致。
func (r *Renderer) RenderConcurrent(root *dom.Node, workers int) (string, error) {
	if workers < 1 {
		workers = 1
	}
	return r.render(root, workers)
}

// buildResult 是一次并发影子构建的结果。
type buildResult struct {
	sr  *dom.Node
	err error
}

func (r *Renderer) render(root *dom.Node, workers int) (string, error) {
	rt := r.rt
	rt.mu.Lock()
	defer rt.mu.Unlock()
	// 固定注册表快照：渲染途中的注册变更不影响本次渲染。
	rt.defs = rt.reg.snapshot()
	defer func() { rt.defs = nil }()

	if err := rt.Upgrade(root); err != nil {
		return "", err
	}
	// 按轮处理脏元素：每轮内并发构建影子树（纯计算），
	// 再按文档顺序统一应用，保证输出与并发度无关。
	for round := 0; len(rt.dirty) > 0 && round < maxFlushRounds; round++ {
		batch := rt.dirty
		rt.dirty = nil
		rt.dirtySet = map[*dom.Node]bool{}
		sort.SliceStable(batch, func(i, j int) bool {
			return rt.order[batch[i]] < rt.order[batch[j]]
		})
		results := make([]buildResult, len(batch))
		var wg sync.WaitGroup
		sem := make(chan struct{}, workers)
		for i, n := range batch {
			el, ok := rt.inst[n]
			if !ok {
				continue
			}
			wg.Add(1)
			go func(i int, el *Element) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				sr, err := rt.buildShadow(el)
				results[i] = buildResult{sr: sr, err: err}
			}(i, el)
		}
		wg.Wait()
		var firstErr error
		for i, n := range batch {
			el, ok := rt.inst[n]
			if !ok {
				continue
			}
			if err := rt.applyShadow(el, results[i].sr, results[i].err); err != nil && firstErr == nil {
				firstErr = err
			}
			if el.Node.Shadow != nil {
				_ = rt.Upgrade(el.Node.Shadow)
			}
		}
		if firstErr != nil {
			return "", firstErr
		}
	}
	// 为带影子树的宿主写入激活标记。
	walkAll(root, func(n *dom.Node) {
		if n.Type == dom.ElementNode && n.Shadow != nil {
			if _, ok := rt.inst[n]; ok {
				n.SetAttribute(ssrAttr, "")
			}
		}
	})
	return dom.Serialize(root), nil
}

// walkAll 以前序遍历轻 DOM 与影子树。
func walkAll(n *dom.Node, fn func(*dom.Node)) {
	fn(n)
	if n.Shadow != nil {
		walkAll(n.Shadow, fn)
	}
	for _, c := range n.Children {
		walkAll(c, fn)
	}
}
