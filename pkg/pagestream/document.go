package pagestream

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	g "maragu.dev/gomponents"
	dsattr "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

type PageSpec struct {
	Title            string
	Language         string
	HTMLAttrs        []g.Node
	Head             []g.Node
	MainAttrs        []g.Node
	Signals          map[string]any
	BeforeStreamInit []string
	UpdatesURL       string
	Body             []g.Node
}

func RenderPage(spec PageSpec) g.Node {
	updatesURL := validateUpdatesURL(spec.UpdatesURL)
	signals := cloneSignals(spec.Signals)
	ensureRuntimeUpdatesURL(signals, updatesURL)
	init := append([]string{}, spec.BeforeStreamInit...)
	init = append(init, RefreshSignalsAction())
	return renderDocument(documentSpec{
		Title:     spec.Title,
		Language:  spec.Language,
		HTMLAttrs: spec.HTMLAttrs,
		Head:      spec.Head,
		MainAttrs: spec.MainAttrs,
		Signals:   signals,
		Init:      init,
		Body:      spec.Body,
	})
}

func RefreshSignalsAction() string {
	return "@get($runtime.updatesUrl, {openWhenHidden: true})"
}

type documentSpec struct {
	Title     string
	Language  string
	HTMLAttrs []g.Node
	Head      []g.Node
	MainAttrs []g.Node
	Signals   map[string]any
	Init      []string
	Body      []g.Node
}

func renderDocument(spec documentSpec) g.Node {
	language := spec.Language
	if language == "" {
		language = "en"
	}
	mainChildren := []g.Node{}
	if spec.Signals != nil {
		mainChildren = append(mainChildren, dsattr.Signals(spec.Signals))
	}
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
		Head:      spec.Head,
		Body:      []g.Node{h.Main(mainAttrs...)},
	})
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

func cloneSignals(signals map[string]any) map[string]any {
	if signals == nil {
		panic("pagestream: Signals are required")
	}
	clone := make(map[string]any, len(signals)+1)
	for key, value := range signals {
		clone[key] = value
	}
	return clone
}

func ensureRuntimeUpdatesURL(signals map[string]any, updatesURL string) {
	runtime, ok := signals["runtime"]
	if !ok || runtime == nil {
		signals["runtime"] = map[string]any{"updatesUrl": updatesURL}
		return
	}
	updated, ok := runtimeWithUpdatesURL(runtime, updatesURL)
	if !ok {
		panic("pagestream: runtime signal must expose updatesUrl")
	}
	signals["runtime"] = updated
}

func runtimeWithUpdatesURL(runtime any, updatesURL string) (any, bool) {
	if runtimeMap, ok := runtime.(map[string]any); ok {
		clone := make(map[string]any, len(runtimeMap)+1)
		for key, value := range runtimeMap {
			clone[key] = value
		}
		if existing, _ := clone["updatesUrl"].(string); strings.TrimSpace(existing) != "" && existing != updatesURL {
			panic(fmt.Sprintf("pagestream: runtime.updatesUrl %q does not match UpdatesURL %q", existing, updatesURL))
		}
		clone["updatesUrl"] = updatesURL
		return clone, true
	}

	value := reflect.ValueOf(runtime)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, false
	}
	copyValue := reflect.New(value.Type()).Elem()
	copyValue.Set(value)
	field := copyValue.FieldByName("UpdatesURL")
	if !field.IsValid() || field.Kind() != reflect.String || !field.CanSet() {
		return nil, false
	}
	existing := strings.TrimSpace(field.String())
	if existing != "" && existing != updatesURL {
		panic(fmt.Sprintf("pagestream: runtime.updatesUrl %q does not match UpdatesURL %q", existing, updatesURL))
	}
	field.SetString(updatesURL)
	return copyValue.Interface(), true
}
