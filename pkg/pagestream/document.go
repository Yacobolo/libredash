package pagestream

import (
	"fmt"
	"net/url"
	"strings"

	g "maragu.dev/gomponents"
	dsattr "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const datastarScriptSrc = "/static/vendor/datastar-1.0.2.js?v=dev"

type PageSpec struct {
	Title      string
	Language   string
	HTMLAttrs  []g.Node
	Head       []g.Node
	MainAttrs  []g.Node
	UpdatesURL string
	Body       []g.Node
}

func RenderPage(spec PageSpec) g.Node {
	updatesURL := validateUpdatesURL(spec.UpdatesURL)
	return renderDocument(documentSpec{
		Title:     spec.Title,
		Language:  spec.Language,
		HTMLAttrs: spec.HTMLAttrs,
		Head:      spec.Head,
		MainAttrs: spec.MainAttrs,
		Init:      []string{openUpdatesAction(updatesURL)},
		Body:      spec.Body,
	})
}

func openUpdatesAction(updatesURL string) string {
	return "@get('" + jsSingleQuoted(updatesURL) + "', {openWhenHidden: true})"
}

func jsSingleQuoted(value string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`).Replace(value)
}

type documentSpec struct {
	Title     string
	Language  string
	HTMLAttrs []g.Node
	Head      []g.Node
	MainAttrs []g.Node
	Init      []string
	Body      []g.Node
}

func renderDocument(spec documentSpec) g.Node {
	language := spec.Language
	if language == "" {
		language = "en"
	}
	head := []g.Node{datastarScript()}
	head = append(head, spec.Head...)
	mainChildren := []g.Node{}
	if init := initExpression(spec.Init...); init != "" {
		mainChildren = append(mainChildren, dsattr.Init(init))
	}
	mainChildren = append(mainChildren, spec.Body...)
	mainAttrs := append([]g.Node{}, spec.MainAttrs...)
	mainAttrs = append(mainAttrs, mainChildren...)
	return c.HTML5(c.HTML5Props{
		Title:     spec.Title,
		Language:  language,
		HTMLAttrs: spec.HTMLAttrs,
		Head:      head,
		Body:      []g.Node{h.Main(mainAttrs...)},
	})
}

func datastarScript() g.Node {
	return h.Script(h.Type("module"), h.Src(datastarScriptSrc))
}

func initExpression(expressions ...string) string {
	out := ""
	for _, expression := range expressions {
		if expression == "" {
			continue
		}
		if out != "" {
			out += "; "
		}
		out += expression
	}
	return out
}

func validateUpdatesURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		panic("pagestream: UpdatesURL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.IsAbs() || parsed.Path != "/updates" {
		panic(fmt.Sprintf("pagestream: UpdatesURL must be a relative /updates URL, got %q", raw))
	}
	return trimmed
}
