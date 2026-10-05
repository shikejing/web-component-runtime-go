package wc

import (
	"fmt"
	"sort"
	"sync"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// Element 是一个已升级的自定义元素实例。
type Element struct {
	Node      *dom.Node
	Def       *Definition
	Props     map[string]any
	Connected bool
	Hydrated  bool
	// Err 记录最近一次渲染失败的错误（降级时非 nil）。
	Err error
}

// Runtime 管理元素实例、生命周期派发与重渲染调度。
type Runtime struct {
	reg  *Registry
	mu   sync.Mutex // 序列化渲染过程，保证并发渲染互不干扰
	inst map[*dom.Node]*Element

	defs      map[string]*Definition // 渲染期间固定的注册表快照，nil 表示实时读取
	dirty     []*dom.Node
	dirtySet  map[*dom.Node]bool
	order     map[*dom.Node]int
	nextOrder int
}

// NewRuntime 创建运行时。
func NewRuntime(reg *Registry) *Runtime {
	return &Runtime{
		reg:      reg,
		inst:     map[*dom.Node]*Element{},
		dirtySet: map[*dom.Node]bool{},
		order:    map[*dom.Node]int{},
	}
}

// lookupDef 查询定义：渲染快照优先。
func (rt *Runtime) lookupDef(tag string) (*Definition, bool) {
	if rt.defs != nil {
		d, ok := rt.defs[tag]
		return d, ok
	}
	return rt.reg.Get(tag)
}

// ElementOf 返回节点对应的元素实例。
func (rt *Runtime) ElementOf(n *dom.Node) (*Element, bool) {
	el, ok := rt.inst[n]
	return el, ok
}

// Upgrade 按文档顺序升级子树中的自定义元素（含影子树内的元素）。
// 对每个元素：先按特性顺序触发已观测特性的 attributeChanged，
// 再触发 connected，与标准升级顺序一致。
// 已带有影子树的元素（激活接管）标记为 Hydrated 且不再调度重渲染。
func (rt *Runtime) Upgrade(root *dom.Node) error {
	var nodes []*dom.Node
	walkAll(root, func(n *dom.Node) {
		if n.Type == dom.ElementNode {
			nodes = append(nodes, n)
		}
	})
	for _, n := range nodes {
		if _, ok := rt.inst[n]; ok {
			continue
		}
		if _, ok := rt.order[n]; !ok {
			rt.order[n] = rt.nextOrder
			rt.nextOrder++
		}
		def, ok := rt.lookupDef(n.Tag)
		if !ok {
			continue
		}
		el := &Element{Node: n, Def: def, Props: map[string]any{}}
		rt.inst[n] = el
		// 1. 已存在的已观测特性，按元素特性顺序触发。
		observed := def.observed()
		for _, a := range n.Attributes() {
			if observed[a.Name] && def.Lifecycle.AttributeChanged != nil {
				def.Lifecycle.AttributeChanged(el, a.Name, "", a.Value)
			}
		}
		// 2. 同步反射属性的初始值。
		for prop, attr := range def.Reflect {
			if v, ok := n.GetAttribute(attr); ok {
				el.Props[prop] = v
			}
		}
		// 3. connected 回调。
		el.Connected = true
		if def.Lifecycle.Connected != nil {
			def.Lifecycle.Connected(el)
		}
		// 4. 调度首次渲染；已接管影子树的元素（激活）跳过重渲染。
		if n.Shadow != nil {
			el.Hydrated = true
		} else {
			rt.markDirty(n)
		}
	}
	return nil
}

// Connect 触发子树的 connected 回调（文档顺序）。
func (rt *Runtime) Connect(root *dom.Node) {
	dom.Walk(root, func(n *dom.Node) {
		if el, ok := rt.inst[n]; ok && !el.Connected {
			el.Connected = true
			if el.Def.Lifecycle.Connected != nil {
				el.Def.Lifecycle.Connected(el)
			}
			rt.markDirty(n)
		}
	})
}

// Disconnect 触发子树的 disconnected 回调（文档顺序）。
func (rt *Runtime) Disconnect(root *dom.Node) {
	dom.Walk(root, func(n *dom.Node) {
		if el, ok := rt.inst[n]; ok && el.Connected {
			el.Connected = false
			if el.Def.Lifecycle.Disconnected != nil {
				el.Def.Lifecycle.Disconnected(el)
			}
		}
	})
}

// Adopt 触发子树的 adopted 回调（文档顺序）。
func (rt *Runtime) Adopt(root, oldDoc, newDoc *dom.Node) {
	dom.Walk(root, func(n *dom.Node) {
		if el, ok := rt.inst[n]; ok && el.Def.Lifecycle.Adopted != nil {
			el.Def.Lifecycle.Adopted(el, oldDoc, newDoc)
		}
	})
}

// SetAttribute 经运行时设置特性：同步反射属性、触发已观测特性回调并调度重渲染。
func (rt *Runtime) SetAttribute(n *dom.Node, name, value string) {
	old, _ := n.GetAttribute(name)
	n.SetAttribute(name, value)
	rt.afterAttribute(n, name, old, value)
}

// RemoveAttribute 经运行时移除特性。
func (rt *Runtime) RemoveAttribute(n *dom.Node, name string) {
	old, ok := n.GetAttribute(name)
	if !ok {
		return
	}
	n.RemoveAttribute(name)
	rt.afterAttribute(n, name, old, "")
}

func (rt *Runtime) afterAttribute(n *dom.Node, name, old, new string) {
	el, ok := rt.inst[n]
	if !ok {
		return
	}
	// 特性被直接改写时，反射属性保持同步。
	for prop, attr := range el.Def.Reflect {
		if attr == name {
			el.Props[prop] = new
		}
	}
	if el.Def.observed()[name] && el.Def.Lifecycle.AttributeChanged != nil {
		el.Def.Lifecycle.AttributeChanged(el, name, old, new)
	}
	rt.markDirty(n)
}

// SetProperty 设置属性并按 Reflect 声明反射到特性。
func (rt *Runtime) SetProperty(n *dom.Node, name string, value any) {
	el, ok := rt.inst[n]
	if !ok {
		return
	}
	el.Props[name] = value
	if attr, ok := el.Def.Reflect[name]; ok {
		rt.SetAttribute(n, attr, stringify(value))
	}
}

// Property 读取属性。
func (rt *Runtime) Property(n *dom.Node, name string) (any, bool) {
	el, ok := rt.inst[n]
	if !ok {
		return nil, false
	}
	v, ok := el.Props[name]
	return v, ok
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return ""
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// markDirty 将元素标记为待渲染；同一轮内重复标记只保留一次。
func (rt *Runtime) markDirty(n *dom.Node) {
	if rt.dirtySet[n] {
		return
	}
	rt.dirtySet[n] = true
	rt.dirty = append(rt.dirty, n)
}

// maxFlushRounds 限制单次 Flush 的轮数，防止模板递归生成自身导致死循环。
const maxFlushRounds = 1024

// Flush 合并同一轮内的改动，对每个脏元素只执行一次重渲染，
// 处理顺序为文档顺序，不暴露中间态。
func (rt *Runtime) Flush() {
	for round := 0; len(rt.dirty) > 0 && round < maxFlushRounds; round++ {
		batch := rt.dirty
		rt.dirty = nil
		rt.dirtySet = map[*dom.Node]bool{}
		sort.SliceStable(batch, func(i, j int) bool {
			return rt.order[batch[i]] < rt.order[batch[j]]
		})
		for _, n := range batch {
			if el, ok := rt.inst[n]; ok {
				_ = rt.renderHost(el)
			}
		}
	}
}
