package ui

import (
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type assetIconFactory func(children ...g.Node) g.Node

type assetPresentation struct {
	Icon        assetIconFactory
	Background  string
	Accent      string
	BorderColor string
}

var assetPresentationByType = map[string]assetPresentation{
	"cache_table":     assetPresentationFor(lucide.TableProperties, "cache-table"),
	"catalog":         assetPresentationFor(lucide.BookOpen, "catalog"),
	"connection":      assetPresentationFor(lucide.Plug, "connection"),
	"dashboard":       assetPresentationFor(lucide.LayoutDashboard, "dashboard"),
	"dataset":         assetPresentationFor(lucide.Database, "dataset"),
	"dimension":       assetPresentationFor(lucide.Ruler, "dimension"),
	"filter":          assetPresentationFor(lucide.ListFilter, "filter"),
	"measure":         assetPresentationFor(lucide.Sigma, "measure"),
	"metric_view":     assetPresentationFor(lucide.ChartNoAxesCombined, "metric-view"),
	"page":            assetPresentationFor(lucide.PanelTop, "page"),
	"semantic_model":  assetPresentationFor(lucide.Box, "semantic-model"),
	"source":          assetPresentationFor(lucide.Cable, "source"),
	"table":           assetPresentationFor(lucide.Table2, "table"),
	"visual":          assetPresentationFor(lucide.ChartColumn, "visual"),
	"visual_element":  assetPresentationFor(lucide.SquareDashedMousePointer, "visual"),
	"workspace":       assetPresentationFor(lucide.Boxes, "catalog"),
	"workspace_group": assetPresentationFor(lucide.GalleryVerticalEnd, "catalog"),
}

func assetTypeIcon(typ string) g.Node {
	presentation := assetPresentationForType(typ)
	return h.Span(
		h.Class("inline-flex size-8 shrink-0 items-center justify-center rounded-small border"),
		h.Style(assetPresentationStyle(presentation)),
		g.Attr("aria-hidden", "true"),
		presentation.Icon(assetIconAttrs()...),
	)
}

func assetTypeInlineIcon(typ string) g.Node {
	presentation := assetPresentationForType(typ)
	return h.Span(
		h.Class("inline-flex size-5 shrink-0 items-center justify-center rounded-small border"),
		h.Style(assetPresentationStyle(presentation)),
		g.Attr("aria-hidden", "true"),
		presentation.Icon(assetInlineIconAttrs()...),
	)
}

func assetPresentationFor(icon assetIconFactory, tokenName string) assetPresentation {
	return assetPresentation{
		Icon:        icon,
		Background:  "--ld-asset-" + tokenName + "-bg",
		Accent:      "--ld-asset-" + tokenName + "-accent",
		BorderColor: "--ld-asset-" + tokenName + "-border",
	}
}

func assetPresentationForType(typ string) assetPresentation {
	presentation, ok := assetPresentationByType[typ]
	if ok {
		return presentation
	}
	return assetPresentation{
		Icon:        lucide.Component,
		Background:  "--ld-bg-panel-muted",
		Accent:      "--ld-fg-muted",
		BorderColor: "--ld-line-muted",
	}
}

func assetPresentationStyle(presentation assetPresentation) string {
	return "background-color: var(" + presentation.Background + "); border-color: var(" + presentation.BorderColor + "); color: var(" + presentation.Accent + ")"
}

func assetIconAttrs() []g.Node {
	return []g.Node{h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func assetInlineIconAttrs() []g.Node {
	return []g.Node{h.Class("size-3.5 shrink-0"), h.Style("stroke-width: 1.75")}
}
