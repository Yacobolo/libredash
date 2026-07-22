# LibreDash Quack And Browser Compute Spec

## Summary

LibreDash can use DuckDB-Wasm plus the Quack extension to run a DuckDB-native dashboard runtime in the browser while keeping governed data behind a Quack server.

The important distinction is where a query is completed:

- Browser compute mode uses DuckDB-Wasm as the dashboard query engine and Quack as a secure remote relation/source.
- Remote compute mode sends full visual/table SQL to Quack and lets the remote DuckDB server complete the query.
- Hybrid mode chooses per table, visual, or query shape.

Quack removes the need to expose raw DuckLake Parquet URLs to the browser. It does not remove the basic rule that browser compute requires the browser to receive the data it computes over. For governed workspaces, Quack must enforce authentication, authorization, row-level security, and query rewriting before any rows reach the browser.

## Goals

- Allow users to bring DuckDB-compatible compute endpoints through Quack.
- Support DuckDB-Wasm as a browser-side dashboard runtime.
- Avoid exposing raw DuckLake data-file URLs to end users.
- Keep LibreDash as the control plane for workspaces, semantic models, dashboard contracts, active serving states, and signal payload shapes.
- Use Quack authz rewrite as the RLS and query-policy enforcement boundary.
- Preserve small dashboard outputs through existing signal contracts, virtual scrolling, chart limits, and table windows.
- Support low-latency interaction by keeping result payloads bounded, typically around the visible table window or chart data limit.

## Non-Goals

- Do not make DuckDB-Wasm the default for all governed data.
- Do not expose DuckLake catalog files or raw Parquet file manifests directly to ordinary dashboard clients.
- Do not rely on browser-side code to protect data the user must not possess.
- Do not expose arbitrary SQL authoring as a normal dashboard user protocol.
- Do not require Quack to serve from the browser. The browser is a Quack client only.

## Architecture

```text
LibreDash control plane
  - workspace/catalog loading
  - active serving-state resolution
  - semantic model and dashboard contracts
  - auth/session/token minting
  - signal payload contracts

Browser
  - Lit dashboard components
  - Datastar signals
  - DuckDB-Wasm runtime
  - Quack client extension
  - bounded table/chart query orchestration

Quack server
  - DuckDB execution
  - active DuckLake snapshot attachment
  - authn/authz
  - RLS SQL rewrite
  - secure schemas/views
  - bounded result streaming
```

The browser receives a compute manifest rather than file paths:

```json
{
  "workspaceId": "sales",
  "environment": "dev",
  "servingStateId": "state_123",
  "duckLakeSnapshotId": 42,
  "compute": {
    "mode": "browser",
    "transport": "quack",
    "endpoint": "quack:analytics.example.com:443",
    "exposure": "authorized_rows"
  },
  "limits": {
    "visualRows": 1000,
    "tableWindowRows": 1000
  }
}
```

The manifest identifies the active serving state and the Quack endpoint. It must not expose raw DuckLake data-file URLs for governed workspaces.

## Compute Modes

### Browser Compute

Browser compute uses DuckDB-Wasm as the dashboard engine. Quack provides governed remote relations. The browser executes interactive query plans and requests only bounded windows or chart outputs.

```text
Browser DuckDB-Wasm
  -> query remote secure relation through Quack
  -> receive authorized chunks/results
  -> run local dashboard composition where useful
  -> patch Datastar signal payloads
```

This mode moves dashboard runtime work to the browser:

- filter-state orchestration
- local query planning for dashboard interactions
- small transforms
- table-window state
- chart result shaping
- joins or aggregates only when the required inputs are already bounded or acceptable to expose

Quack still performs secure data access, RLS rewrite, remote scans, and any pushed-down SQL.

Browser compute is appropriate when:

- the user may possess the authorized rows returned to the browser
- result windows are small
- dashboard queries are bounded
- data can be cached or reused across interactions
- source data is user-owned, public, tenant-scoped, or pre-aggregated

### Remote Compute

Remote compute sends full visual/table SQL to Quack. The Quack server rewrites and executes the complete query.

```sql
FROM quack_query(
  'quack:analytics.example.com:443',
  'SELECT status, SUM(revenue) AS revenue
   FROM secure.orders
   WHERE order_date >= DATE ''2025-01-01''
   GROUP BY status
   ORDER BY revenue DESC
   LIMIT 1000'
);
```

This mode minimizes network transfer and keeps raw rows hidden:

```text
Browser
  -> full bounded query
Quack server
  -> RLS rewrite
  -> scan/join/filter/aggregate/sort/limit
Browser
  <- small result set
```

Remote compute is appropriate when:

- RLS-sensitive raw rows should not reach the browser
- fact tables are large
- aggregates are much smaller than base rows
- low network transfer is more important than browser CPU usage

### Hybrid Compute

Hybrid compute chooses the execution path per table or visual:

```yaml
compute:
  default: quack
  tables:
    customers:
      mode: browser
      exposure: authorized_rows
    sales_daily:
      mode: browser
      exposure: pre_aggregate
    orders:
      mode: quack
      exposure: aggregate_results
```

This is the expected long-term product shape. Small dimensions and dashboard-shaped aggregates can run through browser DuckDB-Wasm. Large governed facts should stay remote unless the workspace explicitly allows authorized row exposure.

## RLS And Query Rewrite

Quack authz is the enforcement point. The authorization function may reject a query or return replacement SQL. LibreDash should use replacement SQL for RLS and object-policy enforcement.

Client query:

```sql
SELECT country, COUNT(*) AS customers
FROM customers
GROUP BY country
ORDER BY customers DESC
LIMIT 1000;
```

Effective server query:

```sql
WITH customers AS (
  SELECT *
  FROM raw.customers
  WHERE country IN (
    SELECT country
    FROM auth.allowed_countries
    WHERE principal_id = current_principal()
  )
)
SELECT country, COUNT(*) AS customers
FROM customers
GROUP BY country
ORDER BY customers DESC
LIMIT 1000;
```

The rewrite must avoid recursive table-name ambiguity by targeting raw/internal schemas explicitly and exposing logical names through secure CTEs, secure views, or table functions.

Policy requirements:

- Only authenticated sessions can connect.
- Tokens must be short-lived and scoped to workspace/environment/serving state.
- Authz must reject unsafe statement classes for dashboard clients.
- Authz must reject direct access to raw schemas, filesystem reads, secrets, extension loading, arbitrary `ATTACH`, `COPY`, `EXPORT`, and write statements unless a future admin-only capability explicitly allows them.
- Dashboard clients should query secure logical names only.
- RLS predicates must be mandatory and server-side.
- Column masking and object access rules must be applied before results stream to the browser.

## DuckLake Snapshot Binding

LibreDash control-plane state remains the authority for the active serving state:

```text
workspace=sales
environment=dev
active_serving_state=state_123
state_123.ducklake_snapshot_id=42
```

The Quack server attaches the active DuckLake snapshot for the request/session. The browser must not discover snapshots by reading DuckLake metadata directly.

Recommended session binding:

```text
LibreDash mints token:
  workspace=sales
  environment=dev
  serving_state=state_123
  ducklake_snapshot_id=42
  principal=user_456
  expires_at=...

Quack authn/authz maps token -> principal + serving snapshot.
```

When a newer serving state becomes active, new dashboard sessions receive a new token and manifest. Existing sessions may finish against their bound snapshot until their lease/token expires.

## Signal And Virtual Scroll Contract

LibreDash dashboard outputs are already small by contract:

- visual queries return bounded chart payloads
- BI tables use virtual scroll/table windows
- table windows should default to bounded row counts such as 1000
- Datastar patches carry page-scoped signal payloads, not full table exports

Quack/Wasm integration should preserve this contract. Queries generated for dashboards must include the visual limit, table window limit, sort, offset/keyset cursor, and filter state required for the current signal patch.

Table window example:

```sql
SELECT order_id, status, revenue, order_date
FROM secure.orders
WHERE order_date >= DATE '2025-01-01'
ORDER BY order_date DESC, order_id
LIMIT 1000 OFFSET 5000;
```

Chart example:

```sql
SELECT date_trunc('month', order_date) AS month, SUM(revenue) AS revenue
FROM secure.orders
WHERE order_date >= DATE '2025-01-01'
GROUP BY month
ORDER BY month
LIMIT 1000;
```

Bounded outputs keep network payloads small even when compute happens remotely. They also make browser compute viable when the authorized input is already small or pre-aggregated.

## Query Generation Rules

- Generate bounded SQL for every dashboard visual and table window.
- Prefer full `quack_query` execution when minimal transfer or strict RLS secrecy matters.
- Prefer browser-local execution only when the required input rows are safe to expose and bounded enough to transfer.
- Avoid broad attached-table scans that could stream large raw relations before filters, limits, or aggregates are applied.
- Do not depend on local browser SQL rewriting for security.
- Keep semantic model planning server-authored or manifest-authored; the browser may execute plans but must not invent privileged data paths.

## Product Modes

```yaml
compute:
  mode: quack
  transport: quack
  exposure: aggregate_results
  rls: server_enforced
```

Use for governed dashboards where raw rows must remain hidden.

```yaml
compute:
  mode: browser
  transport: quack
  exposure: authorized_rows
  rls: server_enforced
```

Use for governed browser compute where the user may possess the authorized rows returned by Quack.

```yaml
compute:
  mode: browser
  transport: files
  exposure: raw_allowed
```

Use for public, personal, local, or explicitly exportable datasets.

## Implementation Phases

1. Quack remote compute
   - Keep the current server-side dashboard flow.
   - Use `quack_query` for full visual/table SQL.
   - Add authz rewrite and bounded query generation.
   - Validate RLS and result limits.

2. Browser DuckDB-Wasm runtime
   - Load DuckDB-Wasm and the Quack client extension in a worker.
   - Accept a LibreDash compute manifest.
   - Execute bounded dashboard queries.
   - Patch existing signal shapes from browser-side results.

3. Hybrid planner
   - Choose browser or Quack execution per table/visual/query.
   - Cache safe authorized extracts or pre-aggregates in memory or OPFS.
   - Keep large governed facts in remote compute mode.

4. BYO Quack endpoint
   - Let users register a Quack endpoint as a governed connection.
   - Validate token scope, endpoint capabilities, and required security settings.
   - Surface clear policy labels: aggregate results, authorized rows, or raw allowed.

## Acceptance Criteria

- Governed dashboard clients never receive raw DuckLake Parquet URLs.
- Quack authz can rewrite logical dashboard SQL into RLS-safe effective SQL.
- Dashboard queries are bounded by visual/table limits before execution.
- Virtual-scroll table requests fetch only the current window.
- Browser compute mode is opt-in and declares its row-exposure policy.
- Remote compute mode returns aggregate/window results without exposing base rows.
- Active serving-state tokens bind Quack access to one workspace, environment, principal, and DuckLake snapshot.
- Tests cover RLS rewrite, unsafe SQL rejection, bounded SQL generation, and compute-mode routing.
