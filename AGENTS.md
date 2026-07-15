# LibreDash Project Overview

LibreDash is a lightweight BI monolith for dashboards-as-code. It combines file-backed catalog/dashboard YAML, semantic model YAML, DuckDB import-mode cache/compute, gomponents-rendered Go pages, Datastar SSE signal streaming, and Lit web components for the interactive dashboard surface.

The current product goal is to feel like a small Power BI-style workspace: dashboard discovery, semantic models, report pages, filter panes/cards, chart visuals, and BI tables all defined from code.

## Architecture

- `dashboards/catalog.yaml` is the workspace/catalog entrypoint. It discovers semantic models and dashboards.
- Semantic model YAML defines reusable data concepts: sources, cache tables, datasets, dimensions, measures, and relationships.
- Dashboard YAML defines presentation: filters, KPIs, visuals, tables, pages, layout, and interactions.
- DuckDB is the backend BI engine. It reads local Olist CSVs, materializes cache tables, and runs filtered/aggregated queries.
- Go owns routing, YAML loading/validation, DuckDB query services, cache refresh, and Datastar SSE patches.
- Datastar signals are the data transport between backend and browser. `/updates` streams page-scoped signal patches.
- Lit web components render the interactive client surface: report canvas, filters, charts, tables, model graph, sidebars, footer, and inspector.
- Components bind to Datastar signal payloads through attributes, usually by signal path, and emit small command/events back to Go.
- Visuals use a plugin-style architecture: LibreDash defines renderer-neutral visual shapes and payloads, then renderer plugins adapt those shapes to concrete libraries.
- ECharts is the first built-in chart renderer plugin behind LibreDash visual shape contracts.
- TanStack is the internal interaction engine for BI tables; LibreDash keeps the table signal/query contract.

## Runtime Flow

1. `GET /` renders the dashboard catalog.
2. `GET /dashboards/{dashboard}/pages/{page}` renders a report page shell from dashboard YAML.
3. The page opens `/updates?dashboard={dashboard}&page={page}`.
4. The Go service reads current Datastar signals, resolves dashboard filters/selections, queries DuckDB, and patches KPI/chart/table/filter-option signals.
5. Lit components receive signal payloads through `data-*`/attribute bindings and re-render from those signal paths.
6. Components emit commands/events for filters, visual interactions, table windows, and refreshes; Go responds through Datastar patches rather than ad hoc REST data fetches.

## Important Files

- `cmd/libredash/main.go` starts the app.
- `internal/app/` contains HTTP routes, Datastar command handlers, SSE broker logic, and refresh/update orchestration.
- `internal/data/service.go` is the DuckDB query/cache service for KPIs, charts, tables, filter options, and model graph data.
- `internal/semantic/` loads and validates catalog, model, and dashboard YAML contracts.
- `internal/dashboard/types.go` defines runtime signal payloads for filters, visuals, tables, pages, and status.
- `internal/ui/page.go` renders gomponents HTML shells and initial Datastar signal state.
- `dashboards/olist/model.yaml` is the Olist semantic model.
- `dashboards/olist/executive-sales.yaml` is the main demo dashboard and report-page definition.
- `web/components/` contains Lit source components; `web/components/chart/` contains the visual renderer registry, ECharts renderer, adapters, maps, and shared chart types/utilities.
- `static/` contains the built browser assets served by Go.
- `internal/tools/bootstrapolist` downloads the Olist CSV dataset to the explicit `--out` directory before managed-data plan/sync.

## Useful Commands

- `task dev`
- `task bootstrap`
- `task ci`
- `task test`
- `task build`
- `task dev:stop`
- `task dev:status`
- `task dev:logs`

Use `task ci` as the default full verification command before handing off substantial changes.
