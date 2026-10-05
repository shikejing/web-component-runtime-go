package wc

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shikejing/web-component-runtime-go/dom"
)

// event 记录一次生命周期回调。
type event struct{ kind, detail string }

func defineRecorder(t *testing.T, reg *Registry, name string, log *[]event) {
	t.Helper()
	err := reg.Define(&Definition{
		Name:               name,
		ObservedAttributes: []string{"a", "b"},
		Lifecycle: Lifecycle{
			Connected:    func(el *Element) { *log = append(*log, event{"connected", name}) },
			Disconnected: func(el *Element) { *log = append(*log, event{"disconnected", name}) },
			AttributeChanged: func(el *Element, attr, old, new string) {
				*log = append(*log, event{"attr:" + attr, old + ">" + new})
			},
			Adopted: func(el *Element, oldDoc, newDoc *dom.Node) {
				*log = append(*log, event{"adopted", name})
			},
		},
	})
	if err != nil {
		t.Fatalf("Define(%s): %v", name, err)
	}
}

func TestLifecycleOrderOnUpgrade(t *testing.T) {
	reg := NewRegistry()
	var log []event
	defineRecorder(t, reg, "x-rec", &log)

	doc := dom.NewDocument()
	host := doc.AppendChild(dom.NewElement("x-rec"))
	host.SetAttribute("a", "1")
	host.SetAttribute("b", "2")

	rt := NewRuntime(reg)
	if err := rt.Upgrade(doc); err != nil {
		t.Fatal(err)
	}
	want := []event{
		{"attr:a", ">1"},
		{"attr:b", ">2"},
		{"connected", "x-rec"},
	}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

func TestDisconnectAndAdopt(t *testing.T) {
	reg := NewRegistry()
	var log []event
	defineRecorder(t, reg, "x-mv", &log)

	doc := dom.NewDocument()
	host := doc.AppendChild(dom.NewElement("x-mv"))
	rt := NewRuntime(reg)
	if err := rt.Upgrade(doc); err != nil {
		t.Fatal(err)
	}
	log = nil
	rt.Disconnect(host)
	doc2 := dom.NewDocument()
	rt.Adopt(host, doc, doc2)
	want := []event{{"disconnected", "x-mv"}, {"adopted", "x-mv"}}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

func TestDuplicateDefineRejected(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Define(&Definition{Name: "x-dup"}); err != nil {
		t.Fatal(err)
	}
	err := reg.Define(&Definition{Name: "x-dup"})
	if !errors.Is(err, ErrDuplicateDefinition) {
		t.Fatalf("expected ErrDuplicateDefinition, got %v", err)
	}
	if !strings.Contains(err.Error(), "x-dup") {
		t.Fatalf("error should name the element: %v", err)
	}
	if err := reg.Define(&Definition{Name: "nohyphen"}); err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestRegistrationOrderDoesNotChangeOutput(t *testing.T) {
	build := func(order []string) string {
		reg := NewRegistry()
		for _, name := range order {
			n := name
			_ = reg.Define(&Definition{
				Name: n,
				Template: func(el *Element) (*dom.Node, error) {
					f := dom.NewElement("span")
					f.AppendChild(dom.NewText(n))
					return f, nil
				},
			})
		}
		doc := dom.NewDocument()
		doc.AppendChild(dom.NewElement("x-a"))
		doc.AppendChild(dom.NewElement("x-b"))
		doc.AppendChild(dom.NewElement("x-c"))
		out, err := NewRenderer(NewRuntime(reg)).Render(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := build([]string{"x-a", "x-b", "x-c"})
	second := build([]string{"x-c", "x-a", "x-b"})
	if first != second {
		t.Fatalf("registration order changed output:\n%s\n%s", first, second)
	}
}
