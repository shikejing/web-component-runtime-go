// Package dom 提供运行时所需的最小 DOM 树实现。
package dom

// NodeType 表示节点类型。
type NodeType int

const (
	DocumentNode NodeType = iota
	ElementNode
	TextNode
	CommentNode
	ShadowRootNode
)

// Attr 是一个有序特性键值对。
type Attr struct{ Name, Value string }

// Node 是 DOM 树节点。Shadow 仅对挂载了影子树的宿主元素有效。
type Node struct {
	Type     NodeType
	Tag      string
	Data     string
	Parent   *Node
	Children []*Node
	Shadow   *Node
	attrs    []Attr
}

func NewDocument() *Node          { return &Node{Type: DocumentNode} }
func NewElement(tag string) *Node { return &Node{Type: ElementNode, Tag: tag} }
func NewText(s string) *Node      { return &Node{Type: TextNode, Data: s} }
func NewComment(s string) *Node   { return &Node{Type: CommentNode, Data: s} }
func NewShadowRoot() *Node        { return &Node{Type: ShadowRootNode} }

// AppendChild 将 c 追加为子节点并返回 c。
func (n *Node) AppendChild(c *Node) *Node {
	if c.Parent != nil {
		c.Parent.RemoveChild(c)
	}
	c.Parent = n
	n.Children = append(n.Children, c)
	return c
}

// InsertBefore 将 c 插入到 ref 之前；ref 为 nil 时等价于 AppendChild。
func (n *Node) InsertBefore(c, ref *Node) *Node {
	if ref == nil {
		return n.AppendChild(c)
	}
	if c.Parent != nil {
		c.Parent.RemoveChild(c)
	}
	for i, ch := range n.Children {
		if ch == ref {
			n.Children = append(n.Children, nil)
			copy(n.Children[i+1:], n.Children[i:])
			n.Children[i] = c
			c.Parent = n
			return c
		}
	}
	return n.AppendChild(c)
}

// RemoveChild 移除子节点 c。
func (n *Node) RemoveChild(c *Node) {
	for i, ch := range n.Children {
		if ch == c {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			c.Parent = nil
			return
		}
	}
}

// SetAttribute 设置特性，已存在时原地更新以保持稳定顺序。
func (n *Node) SetAttribute(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].Name == name {
			n.attrs[i].Value = value
			return
		}
	}
	n.attrs = append(n.attrs, Attr{Name: name, Value: value})
}

// GetAttribute 读取特性。
func (n *Node) GetAttribute(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// HasAttribute 判断特性是否存在。
func (n *Node) HasAttribute(name string) bool {
	_, ok := n.GetAttribute(name)
	return ok
}

// RemoveAttribute 移除特性。
func (n *Node) RemoveAttribute(name string) {
	for i, a := range n.attrs {
		if a.Name == name {
			n.attrs = append(n.attrs[:i], n.attrs[i+1:]...)
			return
		}
	}
}

// Attributes 返回特性的有序副本。
func (n *Node) Attributes() []Attr {
	out := make([]Attr, len(n.attrs))
	copy(out, n.attrs)
	return out
}

// Clone 克隆节点；deep 为 true 时递归克隆子树与影子树。
func (n *Node) Clone(deep bool) *Node {
	c := &Node{Type: n.Type, Tag: n.Tag, Data: n.Data}
	c.attrs = n.Attributes()
	if deep {
		for _, ch := range n.Children {
			c.AppendChild(ch.Clone(true))
		}
		if n.Shadow != nil {
			c.Shadow = n.Shadow.Clone(true)
		}
	}
	return c
}

// Root 返回节点所属树的根。
func (n *Node) Root() *Node {
	cur := n
	for cur.Parent != nil {
		cur = cur.Parent
	}
	return cur
}

// Walk 以文档前序遍历子树（不含影子树）。
func Walk(n *Node, fn func(*Node)) {
	fn(n)
	for _, ch := range n.Children {
		Walk(ch, fn)
	}
}
