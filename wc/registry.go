// Package wc 实现自定义元素与影子 DOM 的服务端渲染与客户端激活运行时。
package wc

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// FallbackPolicy 定义单个元素渲染失败时的降级策略。
type FallbackPolicy int

const (
	// FallbackLightDOM 渲染失败时仅保留轻 DOM，不输出影子树。
	FallbackLightDOM FallbackPolicy = iota
	// FallbackEmpty 渲染失败时输出空影子树。
	FallbackEmpty
	// FallbackPropagate 渲染失败时把错误向上抛出。
	FallbackPropagate
)

// Lifecycle 是自定义元素的生命周期回调集合。
type Lifecycle struct {
	Connected        func(el *Element)
	Disconnected     func(el *Element)
	AttributeChanged func(el *Element, name, oldValue, newValue string)
	Adopted          func(el *Element, oldDoc, newDoc *dom.Node)
}

// Definition 描述一个自定义元素。
type Definition struct {
	Name               string
	ObservedAttributes []string
	// Reflect 声明 属性名 -> 特性名 的反射关系。
	Reflect   map[string]string
	Lifecycle Lifecycle
	// Template 产出影子树内容片段，返回 error 时按 OnError 策略降级。
	Template func(el *Element) (*dom.Node, error)
	Styles   string
	OnError  FallbackPolicy
}

// observed 返回已观测特性集合。
func (d *Definition) observed() map[string]bool {
	m := make(map[string]bool, len(d.ObservedAttributes))
	for _, a := range d.ObservedAttributes {
		m[a] = true
	}
	return m
}

// Registry 是自定义元素注册表，并发安全。
type Registry struct {
	mu   sync.RWMutex
	defs map[string]*Definition
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry { return &Registry{defs: map[string]*Definition{}} }

// ErrDuplicateDefinition 表示重复注册同名元素。
var ErrDuplicateDefinition = errors.New("wc: duplicate custom element definition")

// validateName 校验自定义元素名：必须含连字符且为小写标签名。
func validateName(name string) error {
	if name == "" {
		return errors.New("wc: element name must not be empty")
	}
	if !strings.Contains(name, "-") {
		return fmt.Errorf("wc: %q is not a valid custom element name: missing hyphen", name)
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("wc: %q is not a valid custom element name: must be lowercase", name)
	}
	return nil
}

// Define 注册定义；同名重复注册将被拒绝并说明原因。
// 注册只追加不覆盖，因此注册顺序不影响最终渲染结果。
func (r *Registry) Define(d *Definition) error {
	if d == nil {
		return errors.New("wc: definition must not be nil")
	}
	if err := validateName(d.Name); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[d.Name]; exists {
		return fmt.Errorf("%w: %q is already defined", ErrDuplicateDefinition, d.Name)
	}
	cp := *d
	r.defs[d.Name] = &cp
	return nil
}

// Get 查询定义。
func (r *Registry) Get(name string) (*Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[name]
	return d, ok
}

// snapshot 返回注册表副本，渲染期间不受后续注册影响。
func (r *Registry) snapshot() map[string]*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*Definition, len(r.defs))
	for k, v := range r.defs {
		out[k] = v
	}
	return out
}
