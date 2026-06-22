# Semantic Model Decision

## Summary

LibreDash should become a semantic BI engine with managed materialization, not a general transformation framework and not an OBT-only dashboard tool.

The authored truth is a logical semantic model:

- Sources describe where raw data comes from.
- Model tables describe lightly transformed DuckDB-backed fact and dimension tables.
- Relationships describe the governed join graph.
- Dimensions and measures describe reusable business fields.
- Metric views expose curated business-facing query surfaces.
- Dashboards and visuals request fields from metric views through a simple query API.

Physical serving tables, OBTs, rollups, and cache tables are implementation details that LibreDash may generate or materialize for performance. They should not become the primary authored contract.

## Decision

LibreDash will use a strict star-schema-oriented semantic model.

Authors define business tables and relationships. Visuals do not know SQL joins. Metric views expose dimensions and measures. The runtime decides whether to answer a query dynamically from the semantic graph or from a managed materialization.

## Final Long-Term Shape

The product contract is the semantic graph and metric view query API:

```text
connections
  -> sources
  -> model tables
  -> relationships
  -> semantic model
  -> metric views
  -> visual/query API
```

Managed physical serving shapes hang off that contract:

```text
semantic model / metric view
  -> optional materialized model table
  -> optional OBT serving table
  -> optional aggregate rollup
  -> optional query-result cache
```

Authors model business tables, fields, relationships, and metric views. LibreDash manages DuckDB materializations, OBT-like serving tables, rollups, and caches. Dashboard YAML and visual components never depend on physical serving table names.

The practical runtime shape may be:

```text
source -> raw DuckDB table/view -> model table -> metric view -> dashboard
```

or, when optimized:

```text
source -> raw DuckDB table/view -> model table -> managed serving table / rollup -> dashboard
```

Both are implementations of the same semantic contract.

## Naming Decisions

LibreDash should use one vocabulary consistently.

| Term | Meaning | User-facing? |
| --- | --- | --- |
| Connection | Access configuration for an external system or local data root. Secrets are never shown. | Yes |
| Source | Raw external object/table/file, close to the input data and without business logic. | Yes |
| Model table | Authored semantic table backed by DuckDB, usually a fact or dimension table. May be direct from a source or a light transform. | Yes |
| Relationship | Governed join path between model tables, with cardinality and active state. | Yes |
| Dimension | Groupable/filterable field exposed by a model table and optionally by metric views. | Yes |
| Measure | Governed aggregation owned by a clear base table/grain. | Yes |
| Metric view | Curated query surface for dashboards and consumers, anchored on one base table/grain. | Yes |
| Materialization | Physical DuckDB table, cache table, OBT, rollup, or query cache generated or managed for performance. | Mostly no |
| Dataset | Legacy/current implementation term. Long term it should become either a model table or a compatibility alias for a metric view's managed serving table. | Avoid |
| Cache table | Legacy/current implementation term for a physical backing table. Long term it should be treated as materialization, not a primary asset. | Avoid |

The preferred long-term language is `model table` for the authored semantic table and `materialization` for generated or backing physical tables.

## Rules We Enforce

LibreDash should force one good path:

1. Authors define sources, model tables, relationships, dimensions, measures, and metric views in Git/YAML.
2. Dashboards and visuals query metric views only.
3. Visuals request dimensions, measures, filters, sort, and limits; they do not reference SQL joins, cache tables, DuckDB schemas, or source files.
4. A metric view has one base table and one clear grain.
5. Measures belong to a base table/grain and must be valid under filtering and grouping.
6. Dimensions may come from the base table or safe related dimension tables.
7. Safe default relationship traversal is many-to-one from the metric view base table to dimensions.
8. One-to-many, many-to-many, ambiguous, or multi-fact paths require an explicit model table at the correct grain.
9. Heavy transformation, orchestration, and long SQL model chains belong upstream.
10. DuckDB SQL inside LibreDash is allowed only for light preparation, grain alignment, and semantic model clarity.
11. OBTs, rollups, and query caches are generated or managed optimizations, not authored dashboard contracts.
12. The UI presents semantic concepts first and implementation details second.

This means LibreDash can still be fast like an OBT/dashboard tool, but authors model the business instead of hand-maintaining one wide dashboard table.

## Why Not OBT-Only?

One Big Table is useful as a serving optimization, but it should not be the main semantic abstraction.

OBT-only modeling has real costs:

- It hides business relationships.
- It duplicates dimension attributes across fact rows.
- It grows wider as dashboards grow.
- It couples dashboard needs to physical table shape.
- It makes lineage less truthful.
- It struggles with multiple grains.
- It encourages adding columns instead of improving the model.

LibreDash can still create OBT-like materializations behind the scenes, especially for DuckDB performance. The important rule is that the author models the business graph, not a hand-maintained dashboard table.

## Why Not A General Transformation Framework?

LibreDash should allow DuckDB SQL for light preparation, but it should not become dbt or Rill-style flexible modeling.

General transformation frameworks tend to allow many valid ways to solve the same problem:

- Arbitrary chains of SQL models.
- Dashboard-specific denormalized tables.
- Mixed modeling conventions.
- Business logic spread across transformation SQL and metric definitions.

LibreDash should force one good path:

1. Bring data in as sources.
2. Define clean model tables.
3. Declare relationships.
4. Define measures and dimensions.
5. Expose curated metric views.
6. Build dashboards from metric views.

Transformations should be light, local, and in service of the semantic model. Heavy ETL, orchestration, and warehouse modeling should live upstream.

## Why Metric Views Instead Of Direct Semantic Model Queries?

Power BI visuals query the semantic model directly, but that works because Power BI has a rich semantic evaluation engine: DAX filter context, relationship filter propagation, active and inactive relationships, bidirectional filtering, cardinality handling, time intelligence, calculation semantics, perspectives, row-level security, and mature safeguards.

LibreDash should preserve the familiar mental model of a semantic model with measures, dimensions, relationships, and visuals. However, dashboards should not query the entire semantic model directly. They should query metric views.

Direct semantic model querying would be powerful, but it would make every visual query responsible for resolving:

- Base grain.
- Relationship paths.
- Filter propagation.
- Fanout safety.
- Multi-fact ambiguity.
- Measure validity under grouping and filtering.
- Time field and grain behavior.

Those rules are possible to implement, but they substantially increase engine complexity and make plausible-but-wrong numbers easier to produce.

Metric views are the deliberate simplification. A metric view is a curated query surface over the semantic model. It declares one base table, one clear grain, exposed dimensions, exposed measures, default time behavior, and safe relationship paths. Visuals still use semantic fields, but only through a metric view that defines where those fields are valid.

This is less free-form than Power BI, but it gives LibreDash a safer and more predictable BI-as-code contract:

```text
semantic model = authored business graph
metric view    = safe report/query surface
visual         = renderer/query consumer
```

LibreDash may later add a direct semantic explorer for modeling, debugging, or governed ad hoc analysis. That should be a separate experience from dashboard YAML. Dashboard visuals should continue to query metric views only.

## Core Concepts

### Source

A source is an external raw object or table.

Examples:

- Local CSV file.
- S3 parquet path.
- Postgres table.
- DuckDB table.
- MotherDuck table.

Sources should be close to the raw input and should not contain business logic.

```yaml
sources:
  orders_csv:
    connection: olist
    format: csv
    path: olist_orders_dataset.csv
```

### Model Table

A model table is a DuckDB-backed table or view used by the semantic model.

It may directly reference a source or define a light SQL transform. Model tables are the authored physical/logical tables in the semantic graph.

```yaml
tables:
  orders:
    kind: fact
    source: orders_csv
    primary_key: order_id
    grain: order_id

  customers:
    kind: dimension
    source: customers_csv
    primary_key: customer_id
```

Light transforms are allowed when they clarify the semantic model:

```yaml
tables:
  payments_by_order:
    kind: fact
    transform:
      sql: |
        SELECT
          order_id,
          SUM(payment_value) AS revenue
        FROM raw.payments
        GROUP BY order_id
    primary_key: order_id
    grain: order_id
```

Allowed transform use cases:

- Type casting.
- Column normalization.
- Small derived columns.
- Light cleaning.
- Aggregating a source to the declared table grain.
- Joining simple lookup/reference data when query-time lookup is not appropriate.

Discouraged transform use cases:

- Full ETL pipelines.
- Dashboard-specific wide tables.
- Long model chains.
- Duplicating metric logic.
- Pre-aggregating metrics that should remain measures.

### Relationship

A relationship declares how model tables join.

Relationships must be explicit and typed.

```yaml
relationships:
  - from: orders.customer_id
    to: customers.customer_id
    cardinality: many_to_one
    active: true
```

Long-term relationship metadata should support:

- Cardinality.
- Active/inactive state.
- Join type.
- Relationship direction.
- Optional referential integrity hints.

Default query behavior should be conservative:

- Prefer fact-to-dimension paths.
- Avoid ambiguous join paths.
- Reject unsafe many-to-many behavior unless explicitly modeled.
- Require explicit modeling for multi-fact queries.

### Dimension

A dimension is a reusable groupable/filterable field.

Dimensions belong to a model table.

```yaml
dimensions:
  customers.state:
    label: State
    expression: customers.customer_state
```

Dimensions may be exposed through metric views. Visuals should only request dimensions exposed by the active metric view.

### Measure

A measure is a reusable aggregation.

Measures belong to a base fact table and should be defined at a clear grain.

```yaml
measures:
  orders.order_count:
    label: Orders
    expression: COUNT(DISTINCT orders.order_id)

  orders.revenue:
    label: Revenue
    expression: SUM(payments_by_order.revenue)
    format: currency
```

Measure rules:

- Measures should not be pre-aggregated unless the model table grain makes that explicit.
- Measures should declare their owning/base table.
- Measures should be valid under filtering and grouping.
- Multi-fact measures require explicit modeling.

### Metric View

A metric view is the business-facing query surface.

It should usually declare one base table and one grain. It exposes a curated set of dimensions and measures.

```yaml
id: orders
title: Orders Metrics
semantic_model: olist
base_table: orders
grain: order_id
time:
  default_field: orders.purchase_timestamp
  allowed_grains: [day, week, month, quarter, year]
dimensions:
  - orders.purchase_month
  - orders.status
  - customers.state
  - products.category
measures:
  - orders.order_count
  - orders.revenue
  - orders.aov
```

Metric view rules:

- A visual queries one metric view at a time.
- A metric view defines the safe field universe for dashboards.
- The base table anchors join planning.
- Dimensions can come from related dimension tables when the relationship path is valid.
- Measures must be valid for the metric view grain.

### Dashboard Query API

Dashboards and visuals use a simple semantic query API.

The query contract is field-driven:

```text
metric view + dimensions + measures + filters + options -> result
```

It is not table-driven:

```text
raw SQL + joins -> result
```

The API should be the same mental model whether the caller is:

- Dashboard YAML.
- A Lit visual component.
- A Datastar command handler.
- A future public HTTP API.
- A future AI or SDK consumer.

The visual declares the business question. LibreDash resolves fields, joins, materialization, SQL, filtering, and result shape.

```yaml
query:
  metric_view: orders
  dimensions:
    - field: customers.state
      alias: state
  measures:
    - field: orders.revenue
      alias: revenue
    - field: orders.order_count
      alias: order_count
  filters:
    - field: orders.status
      operator: equals
      value: delivered
```

The dashboard layer should not reference:

- Source files.
- Physical cache table names.
- SQL joins.
- DuckDB schemas.
- Internal materializations.

### Query Object

The long-term query object should be small, stable, and renderer-neutral.

```yaml
metric_view: orders
dimensions:
  - field: customers.state
    alias: state
measures:
  - field: orders.revenue
    alias: revenue
  - field: orders.order_count
    alias: orders
filters:
  - field: orders.status
    operator: equals
    value: delivered
time:
  field: orders.purchase_timestamp
  grain: month
  range:
    preset: last_12_months
sort:
  - field: revenue
    direction: desc
limit: 50
```

Core fields:

| Field | Meaning |
| --- | --- |
| `metric_view` | Required. The curated semantic query surface. |
| `dimensions` | Optional groupable fields exposed by the metric view. |
| `measures` | Optional aggregations exposed by the metric view. |
| `filters` | Optional field filters, usually merged with dashboard/page filters. |
| `time` | Optional time field, grain, and range. |
| `sort` | Optional sort fields, usually using output aliases. |
| `limit` | Optional result row limit. |

Operators should be semantic and portable:

- `equals`
- `not_equals`
- `in`
- `not_in`
- `contains`
- `starts_with`
- `is_null`
- `is_not_null`
- `greater_than`
- `greater_than_or_equal`
- `less_than`
- `less_than_or_equal`
- `between`

The query API should reject:

- Fields not exposed by the metric view.
- Ambiguous relationship paths.
- Unsafe fanout paths.
- Measures invalid for the selected grain.
- Raw SQL from dashboard YAML.
- References to physical materializations.

### Visual YAML Contract

A visual should declare its renderer, semantic query, and visual encodings separately.

```yaml
visuals:
  revenue_by_month:
    title: Revenue by month
    type: chart
    renderer: echarts
    chart:
      kind: line
    query:
      metric_view: orders
      time:
        field: orders.purchase_timestamp
        grain: month
      measures:
        - field: orders.revenue
          alias: revenue
      filters:
        - field: orders.status
          operator: equals
          value: delivered
      sort:
        - field: orders.purchase_timestamp
          direction: asc
    encode:
      x: orders.purchase_timestamp
      y: revenue
      color: null
```

The `query` block asks for data. The `encode` block maps returned fields to visual channels. The renderer receives a normalized result and does not know how the data was joined or materialized.

For a grouped bar chart:

```yaml
visuals:
  revenue_by_state:
    title: Revenue by state
    type: chart
    renderer: echarts
    chart:
      kind: bar
    query:
      metric_view: orders
      dimensions:
        - field: customers.state
          alias: state
      measures:
        - field: orders.revenue
          alias: revenue
      sort:
        - field: revenue
          direction: desc
      limit: 10
    encode:
      x: state
      y: revenue
```

For a KPI:

```yaml
visuals:
  total_revenue:
    title: Revenue
    type: kpi
    query:
      metric_view: orders
      measures:
        - field: orders.revenue
          alias: revenue
    encode:
      value: revenue
      format: currency
```

For a BI table:

```yaml
tables:
  orders_summary:
    title: Orders
    query:
      metric_view: orders
      dimensions:
        - field: customers.state
          alias: state
        - field: orders.status
          alias: status
      measures:
        - field: orders.order_count
          alias: orders
        - field: orders.revenue
          alias: revenue
      sort:
        - field: revenue
          direction: desc
      limit: 500
    columns:
      - field: state
        label: State
      - field: status
        label: Status
      - field: orders
        label: Orders
      - field: revenue
        label: Revenue
        format: currency
```

### Dashboard Filters And Selections

Dashboard filters should be semantic fields too.

```yaml
filters:
  period:
    label: Period
    field: orders.purchase_timestamp
    type: time_range
    default:
      preset: all

  state:
    label: State
    field: customers.state
    type: multi_select
```

At runtime, LibreDash merges filter sources in a predictable order:

1. Metric view required filters.
2. Dashboard/page filters.
3. Visual-local filters.
4. Cross-filter selections.
5. User drill context.

The merged query still resolves through the same semantic planner. Filters never introduce new ungoverned join paths.

### Result Payload

The query API should return renderer-neutral data.

```json
{
  "fields": [
    {
      "name": "state",
      "semantic_field": "customers.state",
      "kind": "dimension",
      "type": "string",
      "label": "State"
    },
    {
      "name": "revenue",
      "semantic_field": "orders.revenue",
      "kind": "measure",
      "type": "number",
      "format": "currency",
      "label": "Revenue"
    }
  ],
  "rows": [
    { "state": "SP", "revenue": 1240000.25 },
    { "state": "RJ", "revenue": 820100.5 }
  ],
  "meta": {
    "metric_view": "orders",
    "row_count": 2,
    "materialization": "orders_rollup_month_state",
    "served_from_cache": true
  }
}
```

`meta.materialization` is diagnostic. It can appear in inspector/dev tooling, but dashboards should not depend on it.

### HTTP Shape

The same contract can become a public API later.

```http
POST /api/workspaces/{workspace}/query
Content-Type: application/json
```

```json
{
  "metric_view": "orders",
  "dimensions": [{ "field": "customers.state", "alias": "state" }],
  "measures": [{ "field": "orders.revenue", "alias": "revenue" }],
  "filters": [
    { "field": "orders.status", "operator": "equals", "value": "delivered" }
  ],
  "sort": [{ "field": "revenue", "direction": "desc" }],
  "limit": 10
}
```

The HTTP API should use the same validation and planner as dashboard rendering. There should not be a separate "visual SQL" path.

## Query Behavior

The runtime should resolve a visual query by:

1. Loading the metric view.
2. Resolving requested dimensions and measures.
3. Determining the base table.
4. Finding required relationship paths.
5. Rejecting ambiguous or unsafe paths.
6. Generating SQL for DuckDB.
7. Applying dashboard filters and visual selections.
8. Choosing a dynamic query or managed materialization.
9. Returning a renderer-neutral result payload.

## Managed Materialization

LibreDash may materialize model tables, OBTs, rollups, or query-result caches for performance.

Materialization is an optimization layer:

```text
semantic model graph
  -> planner
  -> optional serving table / rollup / cache
  -> query result
```

Materialization decisions should not leak into dashboard YAML.

Possible materialization strategies:

- Materialize individual model tables.
- Generate an OBT serving table for a metric view.
- Generate aggregate rollups for common dashboard queries.
- Cache query results by filter context.
- Use live dynamic joins for smaller data or development mode.

The user-facing UI should explain materialization as runtime/deployment detail, not as the semantic model itself.

## UI Implications

The workspace UI should show business concepts first:

- Semantic models.
- Model tables.
- Metric views.
- Dashboards.
- Connections and sources.

Implementation details should be de-emphasized:

- Cache tables should not be a primary user-facing asset.
- If shown, they should appear as backing materializations for model tables or metric views.
- Lineage should distinguish authored semantic dependencies from physical serving dependencies.

Recommended user-facing lineage:

```text
connection -> source -> model table -> metric view -> dashboard
```

Optional technical lineage:

```text
model table -> materialized cache table -> rollup/cache
```

## Comparison With Rill

Rill is a useful reference, but LibreDash should be stricter.

Rill's common flow is:

```text
connectors / sources
  -> SQL models
  -> materialized OLAP table or view
  -> metric view over one model/table
  -> dashboards
```

Rill recommends One Big Table modeling for dashboarding and uses metric views as the semantic layer over one model/table. That is pragmatic and fast, but it makes the authored model less relationship-aware.

LibreDash should keep Rill's good ideas:

- BI-as-code.
- Local DuckDB-first development.
- Metric views as the dashboard-facing contract.
- Simple query API for dashboards and custom consumers.
- Materialized serving tables for speed.

LibreDash should avoid copying Rill's flexibility:

- Arbitrary model chains as the main modeling style.
- OBT as the default authored truth.
- Many equivalent ways to model the same dashboard.

LibreDash should be more opinionated:

- Star-schema-oriented semantic models.
- Strict metric view grain.
- Explicit relationships.
- Managed materialization.
- No dashboard-authored SQL joins.

## Comparison With Lightdash

Lightdash is the closest reference for the long-term semantic layer direction, but LibreDash should still be more opinionated.

Lightdash's common flow is:

```text
warehouse / dbt model / Lightdash YAML model
  -> table or explore
  -> dimensions and metrics
  -> generated SQL
  -> charts, API, AI, and dashboards
```

Lightdash treats the semantic layer as the business-facing contract. Data teams define tables, joins, dimensions, and metrics in YAML. End users query those governed building blocks rather than writing raw SQL.

LibreDash should keep these Lightdash ideas:

- Metrics and dimensions are the user-facing business language.
- Tables/model objects should have explicit primary keys, relationships, and cardinality.
- Visuals and dashboards should ask for fields, filters, and metrics instead of SQL.
- The semantic API should generate SQL behind the scenes.
- Pre-aggregates and managed materializations should serve matching semantic queries when possible.
- Authoring should remain code-first and deployment-oriented.

Lightdash also exposes the risk LibreDash should avoid. Lightdash supports flexible joins, explores, joined fields, joined metrics, custom SQL, and multiple modeling styles. Their own guidance still recommends wide/materialized tables where possible because too many BI-layer joins can make self-service harder, slow queries down, confuse AI/user experiences, and create fanout problems.

Lightdash join behavior has important lessons:

- Joins are explicit and relationship-aware.
- Primary keys and relationship metadata are required for safe aggregation.
- Join paths are not globally transitive by default.
- One-to-many and many-to-many joins can inflate metrics.
- Joined metrics, filtered joined metrics, rolled-up metrics, and multi-level metric references have edge cases.
- Dedicated models at the correct grain are often better than clever query-time joins.

LibreDash should borrow the semantic rigor but encode a stricter default path:

1. Authors model a star-schema-oriented semantic graph.
2. Metric views anchor on one base table and one clear grain.
3. Dimensions can come through safe many-to-one paths by default.
4. Unsafe one-to-many, many-to-many, or multi-fact analysis requires explicit model tables at the right grain.
5. LibreDash may generate OBT-like serving tables and aggregate rollups automatically.
6. The dashboard API remains field-driven and does not expose joins.

The key difference is that Lightdash often relies on dbt or warehouse models as the transformation boundary. LibreDash may own light DuckDB-backed model tables, but those transforms must stay in service of the semantic graph. LibreDash should not become a flexible modeling framework with many equally valid patterns.

If Rill is the reference for simple metric views over materialized tables, Lightdash is the reference for a richer semantic layer. LibreDash should combine those lessons:

```text
Lightdash semantic rigor
  + Rill-style simple metric query contract
  + LibreDash-managed DuckDB materialization
  + stricter modeling rules
```

The long-term design is not "always dynamic joins" and not "always authored OBT." It is semantic graph first, with managed physical serving shapes chosen by LibreDash.

## Migration From Current Implementation

Current LibreDash already has pieces of this model:

- Connections.
- Sources.
- Model tables.
- Relationships.
- Metric views.
- Dashboards.

The current runtime effectively does:

```text
source -> raw DuckDB view -> model table -> metric view -> dashboard
```

The long-term migration should reinterpret these concepts:

- `source` remains source.
- `model table` is the authored semantic fact/dimension table.
- Physical cache tables, OBTs, and rollups become implementation details or backing materializations.
- `relationships` become active query-planning metadata.
- `metric view` remains anchored on one base table plus safe related dimensions.

## Non-Goals

LibreDash should not become:

- A full ETL orchestrator.
- A dbt replacement.
- A general SQL notebook.
- A dashboard tool that requires users to hand-build OBTs.
- A semantic layer that allows arbitrary unsafe joins.
- A visual layer that accepts raw SQL as the primary path.

## Guiding Principle

Authors model the business.

LibreDash manages the serving shape.

Dashboards ask for governed fields.

DuckDB executes the plan.
