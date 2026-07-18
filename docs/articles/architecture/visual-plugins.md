# Visual plugin architecture

LibreDash separates visual meaning, query shape, payload normalization, and rendering-library adaptation. A dashboard targets a LibreDash visual type and shape; a renderer plugin turns the normalized payload into a concrete canvas or DOM implementation.

## Product contract

Dashboard configuration defines visual type, renderer, shape, semantic query, presentation options, and interaction mappings. Go validates the query against the active semantic model and emits a `ChartPayload` with stable metadata, dimensions, measures, data rows, formatting, selection, and bounded options.

Shapes communicate expected data structure independently of a library. Examples include `category_value`, `category_series_value`, `single_value`, `matrix`, `graph`, `geo`, `ohlc`, `distribution`, `binned_measure`, and `hierarchy`.

The product contract must be understandable without knowing an ECharts option name.

## Browser host

The `ld-echart` host reads the chart payload from its signal, resolves the requested renderer (defaulting to `echarts`), mounts it into the canvas container, and forwards update, resize, clear, and dispose lifecycle.

It also owns product-level actions such as focus, show/copy data, CSV export, and clear selection. A renderer reports datum selection through the provided context rather than dispatching an unrelated library event as public API.

## Renderer registry

`web/components/dashboard/charts/registry.ts` maps renderer names to plugins. A plugin implements:

- `mount(container, context)`;
- `update(payload, tokens)`;
- `resize()`;
- `clear()`;
- `dispose()`.

`renderers.ts` imports built-in registrations. ECharts is the first built-in plugin. The host disposes the previous handle when switching renderer or disconnecting.

## Adapter layer

ECharts adapters convert LibreDash types/shapes and rows into library options. The renderer supplies theme tokens, container lifecycle, selection-event translation, and a bounded renderer-options escape hatch.

Keep type/shape interpretation in adapters rather than scattering it through the custom element. Shared utilities should normalize values, labels, palettes, and interaction datum lookup consistently.

## Renderer-specific options

`renderer_options` exists for exceptional library-specific behavior. It should remain narrow and nested under the renderer name. Do not expose arbitrary ECharts configuration as the primary dashboard contract; doing so would couple saved dashboards to one library and bypass product validation.

When a behavior is useful across renderers—legend intent, axis meaning, stacking, empty-state semantics—add a renderer-neutral product field and adapt it in each plugin.

## Semantic interactions

The renderer receives mappings and current selection in the payload. On a click, it resolves the selected source datum and passes it to the host. Product interaction code builds typed semantic selection entries with field, fact/grain identity, value, and targets.

Graph and Sankey edge events require special care: only actual source rows should resolve to semantic mappings. Generated or transformed renderer nodes/edges must not invent identities.

## Tables are separate

Tables use a distinct stable signal contract for columns, bounded rows, total/window state, sorting, selection, matrices, pivots, and formatting. TanStack is the browser interaction engine behind that component, not a public payload format.

## Failure behavior

An unknown renderer or unsupported payload should fail visibly and predictably. Empty data clears the renderer and shows product empty state. Theme and resize changes update the mounted instance. Disconnect always releases observers and library resources.

The [visual catalog](/docs/charts/overview) exercises production lazy loading, each documented type, theme behavior, and live renderer integration.
