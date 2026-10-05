# web-component-runtime-go

自定义元素与影子 DOM 的服务端渲染（SSR）与客户端激活（hydration）运行时。
核心目标：**可复现**——同一份定义与状态，在任何遍历顺序与并发度下产出
逐字节一致的序列化结果；激活阶段不重建 DOM，接管前后可观察状态一致。

## 快速开始

```bash
go test ./...      # 运行全部测试
go run ./cmd/demo  # 本地跑通 渲染→序列化→解析→激活 完整链路
```

```go
reg := wcr.NewRegistry()
reg.Define(&wcr.Definition{
    Name:               "user-card",
    ObservedAttributes: []string{"theme"},
    Styles:             ".name { color: navy }",
    Render: func(el *wcr.Element) []*wcr.Node { /* 构造影子树 */ },
    Lifecycle: wcr.Lifecycle{ Connected: func(el *wcr.Element) {} },
})

doc := wcr.NewDocument(reg)
// ... 构建 DOM ...
wcr.NewRenderer(reg, wcr.RenderOptions{Concurrency: 4}).Render(doc, doc.Root)
html := wcr.Serialize(doc.Root, wcr.SerializeOptions{
    DeclarativeShadowDOM: true,
    HydrationMarker:      true,
})

// 客户端
parsed, _ := wcr.Parse(html)
wcr.NewHydrator(reg).Hydrate(doc, parsed) // 原地激活，不重渲染
```

## 渲染语义

### 定义注册（Registry）

- `Define` 注册定义；同名重复注册返回包装了 `ErrDuplicate` 的错误并指明
  冲突名称；无名定义被拒绝。
- 注册顺序不影响渲染结果：输出只取决于文档顺序与定义内容。
- 每次 `Render` 对注册表做一次**快照**：渲染途中新增/替换的定义不影响
  在途的树，下一次渲染才可见。

### 生命周期顺序

升级（upgrade）时按标准顺序触发：

1. `AttributeChanged`——对升级前已存在的每个被观测特性各触发一次；
2. `Connected`——整棵树展开完毕后，按**文档顺序**触发（与并发度无关）；
3. `Disconnected`——`Disconnect(node)` 时对子树内每个已升级元素按文档顺序触发；
4. `Adopted`——`Adopt(newDoc, node)` 迁移文档时触发，元素的脏队列随文档迁移。

### 影子 DOM 样式隔离

- 定义上的 `Styles` 只作用于该元素的影子树；序列化时经 `ScopeStyles`
  改写为 `[data-wcr-scope="<tag>"] ...` 形式，选择器不会泄漏到外部。
- 外部样式不得穿透影子边界，唯一例外是 CSS 规范中的可继承属性
  （`color`、`font-*`、`line-height` 等，见 `scope.go` 的
  `inheritedProperties`）。
- 多层嵌套影子树逐层应用同一规则：每一层边界独立隔离。
- `ComputedStyle(n)` 按 继承 → 作用域规则（源码顺序）→ 内联 `style`
  的优先级解析，支持 `tag`、`.class`、`#id`、`:host` 与后代组合器。

### 插槽分发

- 带 `slot="x"` 的轻 DOM 子节点进入影子树的 `<slot name="x">`，其余进入
  默认（无名）插槽；同名插槽第一个生效。
- 分发结果只取决于文档顺序；`assignSlots` 每次全量重算，轻 DOM 或影子树
  动态变化后重新调用即可得到更新后的分发。
- 指向不存在插槽的节点保持未分发（确定性地留在轻 DOM 中不渲染）；
  无命中节点的插槽展示回退内容（`SlotFallbackActive` 为真）。

### 特性与属性联动

- `Element.Set(name, value)` 设置属性并反射到特性：`bool` 映射为特性
  存在性，字符串/数字映射为特性值；反射有重入保护。
- `Element.SetAttribute` / `RemoveAttribute` 直接改写特性：被观测特性
  触发 `AttributeChanged` 并同步属性镜像，未被观测特性不触发。
- **合并重渲染**：同一轮内对同一元素的所有改动只标记一次脏位，
  `Document.Flush()` 时合并为一次重渲染，不暴露中间态；回调引发的新
  改动会循环排空直到队列清空。

### 序列化与激活标记

- `SerializeOptions.DeclarativeShadowDOM`：影子树序列化为
  `<template shadowrootmode="open">`，样式以 `<style>` 内联其中。
- `SerializeOptions.HydrationMarker`：为每个已升级元素按文档顺序发出
  `data-wcr-id="0..n"`；从服务端 HTML 解析回来的元素保留原 id。
- `data-wcr-scope` / `data-wcr-id` 为运行时保留属性，序列化时去重。
- 输出确定性：特性按插入顺序、激活 id 按文档顺序、样式按键序应用，
  任何 map 迭代顺序都不会影响字节输出（`Parse`∘`Serialize` 往返
  逐字节稳定）。

### 激活（Hydration）

- `Hydrator.Hydrate` 按文档顺序激活所有带激活标记或影子根的元素：
  只挂载 `Element` 状态并触发回调，**不重建 DOM**，激活前后
  序列化结果逐字节一致。
- 幂等：已激活元素跳过，重复激活是安全的。
- 事务性失败：回调 panic 时该元素回滚到激活前状态（不会停在半成品），
  其余元素照常激活，错误聚合返回；修复定义后重试即可成功。

### 并发与错误隔离

- `RenderOptions.Concurrency` 限制并行渲染的 worker 数；同一波内独立的
  影子树并行展开，结果挂回固定位置，`Connected` 仍在文档顺序下触发，
  因此任意并发度输出一致（`-race` 通过）。
- 单个元素 `Render` panic 只隔离该元素：默认降级为注释占位节点
  `<!--wcr: render failed: <tag>-->`，可用 `RenderOptions.OnElementError`
  自定义降级内容；兄弟与子树其余部分不受影响。

## 配置项一览

| 配置 | 位置 | 说明 |
|---|---|---|
| `RenderOptions.Concurrency` | 渲染器 | ≤1 为顺序执行；>1 为并行 worker 上限 |
| `RenderOptions.OnElementError` | 渲染器 | 元素渲染失败的降级策略，nil 用注释占位 |
| `SerializeOptions.DeclarativeShadowDOM` | 序列化 | 输出 `<template shadowrootmode>` |
| `SerializeOptions.HydrationMarker` | 序列化 | 输出 `data-wcr-id` 激活标记 |
| `Definition.ObservedAttributes` | 定义 | 触发 `AttributeChanged` 的特性白名单 |
| `Definition.Styles` | 定义 | 作用域样式，只作用于本元素影子树 |

## 目录结构

```
wcr/            运行时库
  dom.go        节点/影子根数据结构
  registry.go   定义注册（查重、快照）
  render.go     树展开、并发、错误隔离、重渲染
  lifecycle.go  断开/迁移回调
  reflect.go    特性↔属性联动与合并重渲染队列
  slot.go       插槽分发
  scope.go      样式作用域与计算样式
  serialize.go  确定性序列化（DSD + 激活标记）
  parse.go      HTML 解析（含声明式影子 DOM）
  hydrate.go    客户端激活
cmd/demo/       端到端可运行示例
```
