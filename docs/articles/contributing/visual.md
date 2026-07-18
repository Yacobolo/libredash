# Add a visual type

A visual type is complete only when resource validation, semantic query shape, server payload, renderer adaptation, interactions, documentation, examples, and tests agree. Start by deciding whether the requirement is truly a new type, a new renderer-neutral option, or only an adapter improvement for an existing shape.

## Define the product contract

Specify:

- the visual type ID and compatible page component kind;
- the renderer-neutral shape;
- required and optional dimensions, measures, series, or table input;
- sorting and cardinality rules;
- stable payload fields and formatting behavior;
- whether point selection is supported and which datum fields identify it;
- empty, invalid, and partial data behavior.

Add the type/shape to the configuration contract in `internal/configschema/contracts/contracts.cue` and to owning Go dashboard report validation. Update tests that reject unsupported combinations. Regenerate JSON Schema and configuration reference.

Do not begin by adding a raw ECharts option. The server and non-ECharts consumers need to understand the product meaning first.

## Produce the server payload

Extend dashboard runtime normalization so a valid semantic query becomes the expected `ChartPayload`: type, shape, dimensions, measures, data, format, options, renderer, and interaction mappings.

Test:

- valid minimum and representative payloads;
- missing or excessive dimensions/measures;
- result alias handling;
- typed null/zero/false values;
- deterministic sorting and bounds;
- filtered and empty results;
- selection mappings where supported.

Keep renderer-specific data transformations out of Go unless they are part of the stable product payload.

## Extend browser types and adapters

Add the type/shape to `web/components/dashboard/charts/types.ts`. Implement the conversion in `echarts-adapters.ts` or an appropriately focused adapter module. Register a new rendering library only through `registry.ts`/`renderers.ts` and the `ChartRenderer` lifecycle.

The adapter must handle theme tokens, resize, update without remount when safe, clear, and dispose. Avoid global listeners or renderer instances that survive component disconnect.

Use `rendererOptions` only for a bounded exceptional setting. Prefer a product-level option when the behavior could apply to several renderer implementations.

## Implement interactions

If selection is supported, map library events back to the original source datum and let shared interaction code build semantic entries. Test rapid replacement, clear/toggle behavior, and identity containing the correct field, fact/grain, and scalar type.

For transformed structures such as graph edges, Sankey links, or hierarchy nodes, prove that only data with a real source mapping can become a semantic selection.

## Add a showcase example

Add a representative visual definition and page component to the visual showcase workspace. Use data that exercises meaningful labels, multiple series or edge cases, and empty/null behavior without making the example unbounded.

The example should demonstrate the recommended query shape, not every possible option.

## Add documentation

Create or update the visual Markdown under `docs/visuals`, register it in `docs/visuals/catalog.json`, and include a live chart shortcode/configuration example according to the existing visual pages. The page title, breadcrumb, chart ID, and source must remain aligned.

Run `task docs:generate`; the unified catalog will add the route and search entry. The generator rejects orphaned Markdown and broken internal links.

## Test end to end

Run focused Go report/runtime tests, TypeScript adapter/interaction tests, the chart-showcase DOM test, and site tests. Verify light/dark/system themes, compact width, focus modal, show/copy data, CSV export, selection clear, loading, empty, error, resize, and disconnect cleanup as applicable.

Finish with:

```sh
task generate
task docs:check
task ci
```

Do not expose the new type publicly until the generated dashboard schema, browser registry, live documentation route, and production bundle all recognize it.
