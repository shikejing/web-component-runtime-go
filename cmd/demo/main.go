// Command demo runs the full server-render -> serialize -> parse -> hydrate
// chain locally and verifies that hydration preserves observable state.
package main

import (
	"fmt"
	"os"

	"github.com/shikejing/web-component-runtime-go/wcr"
)

func cardDef() *wcr.Definition {
	return &wcr.Definition{
		Name:               "user-card",
		ObservedAttributes: []string{"theme"},
		Styles:             ".card { border: 1px solid #ccc } .name { color: navy }",
		Render: func(el *wcr.Element) []*wcr.Node {
			card := wcr.NewElement("div")
			card.SetAttr("class", "card")
			name := wcr.NewElement("span")
			name.SetAttr("class", "name")
			name.AppendChild(wcr.NewText("User"))
			slot := wcr.NewElement("slot") // default slot for extra content
			card.AppendChild(name)
			card.AppendChild(slot)
			return []*wcr.Node{card}
		},
		Lifecycle: wcr.Lifecycle{
			Connected: func(el *wcr.Element) { fmt.Println("[lifecycle] connected:", el.Node.Tag) },
		},
	}
}

func main() {
	// 1. server: register + render
	reg := wcr.NewRegistry()
	if err := reg.Define(cardDef()); err != nil {
		fmt.Fprintln(os.Stderr, "define:", err)
		os.Exit(1)
	}
	doc := wcr.NewDocument(reg)
	card := wcr.NewElement("user-card")
	card.SetAttr("theme", "dark")
	card.AppendChild(wcr.NewText("extra content"))
	doc.Root.AppendChild(card)
	wcr.NewRenderer(reg, wcr.RenderOptions{Concurrency: 4}).Render(doc, doc.Root)

	// 2. serialize with declarative shadow DOM + hydration markers
	opts := wcr.SerializeOptions{DeclarativeShadowDOM: true, HydrationMarker: true}
	html := wcr.Serialize(doc.Root, opts)
	fmt.Println("[server html]", html)

	// 3. client: parse the payload
	parsed, err := wcr.Parse(html)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	before := wcr.Serialize(parsed, opts)

	// 4. hydrate in place (no re-render)
	creg := wcr.NewRegistry()
	if err := creg.Define(cardDef()); err != nil {
		fmt.Fprintln(os.Stderr, "define:", err)
		os.Exit(1)
	}
	cdoc := wcr.NewDocument(creg)
	cdoc.Root = parsed
	if err := wcr.NewHydrator(creg).Hydrate(cdoc, parsed); err != nil {
		fmt.Fprintln(os.Stderr, "hydrate:", err)
		os.Exit(1)
	}
	after := wcr.Serialize(parsed, opts)

	// 5. verify observable state is unchanged by activation
	if before != after {
		fmt.Fprintln(os.Stderr, "MISMATCH: hydration changed observable state")
		os.Exit(1)
	}
	fmt.Println("[ok] hydration preserved observable state")
}
