# web-component-runtime-go

自定义元素与影子 DOM 的服务端渲染（SSR）与客户端激活（Hydration）运行时。
整条链路可在本地完整跑通：定义注册 → 服务端渲染 → 序列化 → 解析 → 激活 → 动态更新。

## 包结构

| 包 | 职责 |
| --- | --- |
| `dom` | 最小 DOM 树：节点、特性、确定性序列化、HTML 子集解析 |
| `wc` | 自定义元素注册表、生命周期、影子 DOM、插槽、渲染器、激活 |

## 快速开始

```go
reg := wc.NewRegistry()
_ = reg.Define(&wc.Definition{
    Name:               "x-card",
    ObservedAttributes: []string{"title"},
    Reflect:            map[string]string{"title": "title"}, // 属性 -> 特性
    Styles:             "div { color: red }",
    Template: func(el *wc.Element) (*dom.Node, error) {
        f := dom.NewElement("div")
        f.AppendChild(dom.NewElement("slot"))
        return f, nil
    },
    OnError: wc.FallbackLightDOM,
})

// 服务端：渲染为含声明式影子 DOM 的确定性 HTML。
doc := dom.NewDocument()
card := doc.AppendChild(dom.NewElement("x-card"))
card.AppendChild(dom.NewText("hello"))
html, _ := wc.NewRenderer(wc.NewRuntime(reg)).Render(doc)

// 客户端：解析并激活，不重新渲染。
parsed, _ := dom.Parse(html)
rt := wc.NewRuntime(reg)
_ = wc.Hydrate(rt, parsed)

// 激活后动态更新：同一轮多处改动合并为一次重渲染。
rt.SetAttribute(card2, "title", "hi")
rt.Flush()
```

## 渲染语义

### 定义注册（`Registry`）

- `Define` 校验元素名（必须含连字符、全小写），**重复注册同名元素会被拒绝**，
  错误为 `ErrDuplicateDefinition` 并包含元素名。
- 注册只追加不覆盖，**注册顺序不影响最终渲染结果**。
- `Registry` 并发安全；渲染开始时对注册表做**快照**，渲染途中新增定义不影响本次渲染。

### 生命周期

升级（`Upgrade`）按文档顺序进行，对每个元素按标准顺序触发：

1. `AttributeChanged`：对元素上已存在的每个已观测特性，按特性顺序触发（old 为 `""`）；
2. 反射属性初始值同步；
3. `Connected`；
4. 调度首次渲染（激活接管的元素除外）。

`Disconnect` 与 `Adopt` 同样按文档顺序派发。

### 影子 DOM 与样式作用域

- 影子树序列化为声明式影子 DOM：`<template shadowrootmode="open">`。
- 影子内样式经 `ScopeCSS` 改写：每个选择器追加 `[data-wc-scope="<元素名>"]`，
  `:host` / `:host(...)` 映射到 `[data-wc-host="<元素名>"]`，**内部样式不泄漏**。
- 作用域令牌取元素名，与遍历顺序、并发度无关。
- 外部样式只有可继承属性（`IsInheritable` / `FilterInherited`：color、font-*、
  line-height、text-align 等）允许穿透影子边界。
- 多层嵌套影子树逐层打标记，天然逐层隔离。

### 插槽分发

- `Distribute(host)` 计算分发：具名子节点（`slot="x"`）进入同名 `<slot name="x">`，
  其余进入默认插槽；**未命中的节点归入 `Slot == nil` 的桶**，归属确定可预期。
- `Flatten(node)` 产出组合树：插槽被分配节点替换；无分配节点时使用插槽的回退内容；
  未分配节点不出现在组合树中。
- 渲染后动态修改 `slot` 特性或增删子节点，重新调用 `Distribute`/`Flatten` 即得到更新后的分发。

### 特性与属性联动

- `SetAttribute`/`RemoveAttribute`：更新特性、同步反射属性、触发已观测特性回调、调度重渲染。
- `SetProperty`：设置属性并按 `Reflect` 声明反射到特性（走同一条特性路径，回调照常触发）。
- **同一轮内对同一元素的多处改动合并为一次重渲染**（`Flush` 按文档顺序每元素执行一次），
  不暴露中间态。

### 激活（Hydration）

- SSR 产物中带影子树的宿主带 `data-wc-ssr` 激活标记。
- `Hydrate` 把 `<template shadowrootmode="open">` 就地接管为影子树（先完整构建再一次性挂载，
  **原子生效**），随后升级实例并派发生命周期，**不重新渲染**已接管的影子树。
- 激活前后序列化结果逐字节一致；**重复激活幂等**；激活失败（如定义缺失）时元素保持原样，
  修正后可重试。

### 并发与异常隔离

- 单个元素模板返回 error 或 panic 都被隔离在该元素内，按 `OnError` 策略降级：
  - `FallbackLightDOM`（默认）：不输出影子树，仅保留轻 DOM；
  - `FallbackEmpty`：输出空影子树；
  - `FallbackPropagate`：把错误作为 `Render` 的返回值抛出。
- `RenderConcurrent(root, workers)` 并发构建影子树、按文档顺序统一应用，
  输出与 `Render` **逐字节一致**。
- 渲染持有运行时锁，多个渲染互不干扰；模板递归生成自身由轮次上限（1024）兜底。

### 确定性输出

同一份定义与状态，在任何遍历顺序与并发度下序列化结果逐字节一致：

- 特性按名称排序输出；
- 作用域令牌取自元素名而非计数器；
- 脏元素按文档顺序处理；
- 注册表快照隔离渲染期变更；
- 不依赖 map 遍历顺序。

## 配置项一览

| 字段 / 函数 | 说明 |
| --- | --- |
| `Definition.Name` | 元素名，必须含连字符且全小写 |
| `Definition.ObservedAttributes` | 已观测特性列表，变化时触发 `AttributeChanged` |
| `Definition.Reflect` | 属性名 → 特性名 的反射声明 |
| `Definition.Lifecycle` | `Connected` / `Disconnected` / `AttributeChanged` / `Adopted` |
| `Definition.Template` | 影子树模板；返回 `dom.DocumentNode` 片段时展开其子节点，否则整体挂载 |
| `Definition.Styles` | 影子内样式，自动作用域化 |
| `Definition.OnError` | 渲染失败降级策略（见上） |
| `Renderer.Render` / `RenderConcurrent` | 串行 / 并发服务端渲染 |
| `Hydrate` | 激活 SSR 产物 |
| `Runtime.Flush` | 合并执行同轮改动的重渲染 |

## 测试

```sh
go test ./...        # 全部测试
go test -race ./...  # 竞态检测
```

覆盖：生命周期顺序、重复注册拒绝、影子作用域、插槽分发与动态更新、
属性/特性联动与同轮合并、激活接管/幂等/失败恢复、错误隔离与降级、
确定性输出与并发渲染一致性。
