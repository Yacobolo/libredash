# Visual types

LeapView visuals are defined in dashboard YAML. Every visual has a required `type`, a query, and type-specific presentation settings. Choose the visual that best fits the analytical task; rendering is inferred from its type.

Every preview is compiled and queried from the YAML printed beside it against a fixed documentation dataset. Invalid or stale examples fail documentation generation.

## ECharts interaction capabilities

All built-in charts below use the typed ECharts adapter. “Selectable” means a rendered source row can originate the existing semantic `point_selection` interaction when the YAML declares stable mappings and explicit targets. It does not mean selection is enabled by default.

| Visuals | Source-row selection | Notes |
| --- | --- | --- |
| Line, area, bar, column, scatter, combo | Yes | Each rendered Cartesian datum resolves to its original frame row. |
| Waterfall | Yes | The visible value bar is selectable; the synthetic offset series is silent. |
| Histogram, heatmap, candlestick, boxplot | Yes | Selection requires a stable compiled identity in the shaped source row. |
| Pie, donut, funnel | Yes | Each sector or stage resolves to a source row. |
| Gauge | Yes | The single gauge datum may resolve to its source row. |
| Treemap, sunburst, tree | Yes | A real hierarchy row is selectable when every compiled mapping has a value at that depth; incomplete ancestors and synthetic roots are silent. |
| Graph, Sankey | Yes | Source links are selectable through their private row locators; aggregate renderer nodes remain silent. |
| Radar | No | A radar polygon represents several source rows, so `point_selection` fails compilation. |

Unsupported interaction declarations fail deployment compilation instead of rendering an approximation. Map point and region selection use the separate MapLibre interaction path documented on the [map page](/docs/visuals/map).

## Change over time

- [Line chart](/docs/visuals/line)
- [Area chart](/docs/visuals/area)
- [Column chart](/docs/visuals/column)
- [Combo chart](/docs/visuals/combo)
- [Candlestick chart](/docs/visuals/candlestick)

## Compare and rank

- [Bar chart](/docs/visuals/bar)
- [Scatter chart](/docs/visuals/scatter)
- [Funnel chart](/docs/visuals/funnel)
- [Waterfall chart](/docs/visuals/waterfall)
- [Histogram](/docs/visuals/histogram)
- [Boxplot](/docs/visuals/boxplot)
- [Radar chart](/docs/visuals/radar)

## Part-to-whole and hierarchy

- [Pie chart](/docs/visuals/pie)
- [Donut chart](/docs/visuals/donut)
- [Treemap](/docs/visuals/treemap)
- [Tree](/docs/visuals/tree)
- [Sunburst](/docs/visuals/sunburst)

## Relationships and location

- [Heatmap](/docs/visuals/heatmap)
- [Sankey](/docs/visuals/sankey)
- [Graph](/docs/visuals/graph)
- [Map](/docs/visuals/map)

## Custom

- [Custom Vega-Lite](/docs/visuals/custom)

## Summary and exact values

- [Gauge](/docs/visuals/gauge)
- [KPI](/docs/visuals/kpi)
- [Table](/docs/visuals/table)
- [Matrix](/docs/visuals/matrix)
- [Pivot](/docs/visuals/pivot)
