package dom

import (
	"strings"
	"testing"
)

func TestSerializeDeterministicAttrOrder(t *testing.T) {
	el := NewElement("x-a")
	el.SetAttribute("z", "1")
	el.SetAttribute("a", "2")
	el.SetAttribute("m", "3")
	got := Serialize(el)
	if got != `<x-a a="2" m="3" z="1"></x-a>` {
		t.Fatalf("got %s", got)
	}
}

func TestSerializeEscapes(t *testing.T) {
	el := NewElement("p")
	el.SetAttribute("title", `a"b<c>&`)
	el.AppendChild(NewText("x<y>&z"))
	got := Serialize(el)
	want := `<p title="a&quot;b&lt;c&gt;&amp;">x&lt;y&gt;&amp;z</p>`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestShadowSerializesAsDeclarativeTemplate(t *testing.T) {
	host := NewElement("x-h")
	sr := NewShadowRoot()
	sr.AppendChild(NewElement("span"))
	host.Shadow = sr
	got := Serialize(host)
	want := `<x-h><template shadowrootmode="open"><span></span></template></x-h>`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestParseSerializeRoundTrip(t *testing.T) {
	src := `<x-h a="1" b="2"><template shadowrootmode="open"><span data-wc-scope="x-h">hi &lt;there&gt;</span></template>light<!--c--><br></x-h>`
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	got := Serialize(doc)
	if got != src {
		t.Fatalf("round trip mismatch:\n%s\n%s", src, got)
	}
	// 再解析一次仍一致（幂等）。
	doc2, err := Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if Serialize(doc2) != src {
		t.Fatal("re-parse mismatch")
	}
}

func TestParseAdoptsShadow(t *testing.T) {
	doc, err := Parse(`<x-h><template shadowrootmode="open"><i></i></template>t</x-h>`)
	if err != nil {
		t.Fatal(err)
	}
	host := doc.Children[0]
	if host.Shadow == nil {
		t.Fatal("shadow not parsed")
	}
	if len(host.Children) != 1 || host.Children[0].Type != TextNode {
		t.Fatalf("light children wrong: %+v", host.Children)
	}
	if !strings.Contains(Serialize(host.Shadow), "<i></i>") {
		t.Fatalf("shadow content wrong: %s", Serialize(host.Shadow))
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{
		`<div>`, `</div>`, `<div></span>`, `<div a=nq></div>`,
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
