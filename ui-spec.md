# LibreDash UI North Star

LibreDash UI is an MPA with Datastar-streamed page signals and Lit-rendered web components. It is not a React-style SPA.

Server routes remain real URLs and full document navigations:

```text
/dashboards/{dashboard}/pages/{page}
/chat
/workspaces
/connections
/admin
```

Go owns the route and document. Datastar owns page-local reactive state and SSE patches. Lit owns UI composition below explicit server-mounted component roots.

## Runtime Model

```text
server route
  -> HTML document shell
  -> initial Datastar signals
  -> top-level web component call
  -> /updates patches page signals
  -> Lit re-renders from signal-backed properties
  -> Lit emits domain events
  -> Datastar command wiring patches or posts signals
```

Lit components receive properties, render DOM, hold local UI state, and emit events. They do not route, fetch BI data, own backend state, or duplicate semantic/query rules.

## Ownership

- Go: auth, permissions, HTTP routing, contracts, initial signal envelopes, commands, `/updates`, BI query/model logic.
- Datastar: page-local signal graph, signal patching, SSE transport, declarative event-to-command wiring.
- Lit: layout, visual composition, local UI state, component events, lazy UI modules.
- Gomponents: HTML document shell, assets, Datastar root, and explicit top-level custom element mounting.

Gomponents should not build product UI internals long term. Lit should not become a client router or data-fetching framework.

## Route Mounting

Go chooses the route component. `ld-app-shell` provides global chrome and a named page slot; it does not switch on route kind.

```html
<main data-signals="{...}" data-init="@get('/updates?...')">
  <ld-app-shell data-attr:chrome="$chrome">
    <ld-dashboard-page
      slot="page"
      data-attr:page="$page"
      data-attr:filters="$filters"
      data-attr:filter-options="$filterOptions"
      data-attr:visuals="$visuals"
      data-attr:tables="$tables"
      data-attr:status="$status"
    ></ld-dashboard-page>
  </ld-app-shell>
</main>
```

Other routes mount their own page roots:

```html
<ld-app-shell data-attr:chrome="$chrome">
  <ld-chat-page slot="page" data-attr:agent="$agent"></ld-chat-page>
</ld-app-shell>
```

Inside a route root, Lit composes from properties. Domain-specific dispatch belongs inside the domain root, such as dashboard visual kind to chart, KPI, filter, or table component.

## Component Map

App and navigation:

- `ld-app-shell`
- `ld-sidebar`
- `ld-sub-sidebar`

Dashboard:

- `ld-dashboard-page`
- `ld-dashboard-header`
- `ld-dashboard-visual-frame`
- `ld-report-canvas`
- `ld-report-footer`
- `ld-visual-modal`

Dashboard data UI:

- `ld-filter-dock`
- `ld-filter-panel`
- `ld-filter-card`
- `ld-echart`
- `ld-kpi-card`
- `ld-data-table`

Other route roots:

- `ld-chat-page`
- `ld-workspace-page`
- `ld-workspace-asset-page`
- `ld-connections-page`
- `ld-admin-page`
- `ld-login-page`

Shared support:

- `ld-data-grid`
- `ld-code-block`
- `ld-workspace-access-control`
- `ld-asset-lineage-graph`
- `datastar-inspector`

## Signal Contracts

Signals are product API contracts, not convenient maps.

- Go structs are the source of truth.
- JSON Schema and TypeScript types are generated from Go signal structs.
- Lit imports generated types for route props, visuals, tables, filters, status, and chrome.
- Contract tests prove each page references existing visuals, tables, and filters.
- Contract tests fail on unused visual/table/filter payloads unless explicitly marked as preloaded.
- `/updates` patches dynamic signals such as `filters`, `filterOptions`, `visuals`, `tables`, and `status`.
- Route and page view models are seeded by the first response unless a feature deliberately makes them dynamic.

Recommended top-level dashboard signal shape:

```text
$chrome
$page
$filters
$filterOptions
$visuals
$tables
$status
```

## Styling

LibreDash keeps a locality-first styling model.

- Tailwind remains for the outer light DOM and minimal Go shell.
- Lit Shadow DOM components use local CSS.
- Lit CSS consumes Primer and LibreDash CSS variables.
- Tailwind utilities are not injected into Shadow DOM.
- Global CSS contains tokens, imports, source declarations, and minimal document defaults.
- Product UI should not depend on global selectors.

## Target Folder Layout

```text
web/components/
  app/
  navigation/
  dashboard/
    filters/
    charts/
    table/
  workspace/
  chat/
  admin/
  login/
  shared/
  inspector/
```

Route bundles should be coarse enough to support lazy loading without duplicating shared dependencies. The server should include only the route root and common shell assets needed for the current document.
