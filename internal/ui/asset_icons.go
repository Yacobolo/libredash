package ui

import (
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type assetIconFactory func(children ...g.Node) g.Node

var assetIconByType = map[string]assetIconFactory{
	"cache_table":     lucide.TableProperties,
	"catalog":         lucide.BookOpen,
	"connection":      lucide.Plug,
	"dashboard":       lucide.LayoutDashboard,
	"dataset":         lucide.Database,
	"dimension":       lucide.Ruler,
	"filter":          lucide.ListFilter,
	"measure":         lucide.Sigma,
	"metric_view":     lucide.ChartNoAxesCombined,
	"page":            lucide.PanelTop,
	"semantic_model":  lucide.Box,
	"source":          lucide.Cable,
	"table":           lucide.Table2,
	"visual":          lucide.ChartColumn,
	"visual_element":  lucide.SquareDashedMousePointer,
	"workspace":       lucide.Boxes,
	"workspace_group": lucide.GalleryVerticalEnd,
}

func assetTypeIcon(typ string) g.Node {
	icon := assetIconByType[typ]
	if icon == nil {
		icon = lucide.Component
	}
	return h.Span(
		h.Class("inline-flex size-8 shrink-0 items-center justify-center rounded-small text-icon-muted"),
		g.Attr("aria-hidden", "true"),
		icon(assetIconAttrs()...),
	)
}

func assetIconAttrs() []g.Node {
	return []g.Node{h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}
