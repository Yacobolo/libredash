# LibreDash Query Planning Spec

This document captures the target design for source assets, model SQL planning, and source read optimization. The goal is to keep source assets as metadata contracts while letting LibreDash compile efficient execution SQL for DuckDB, Quack, files, and future adapters.

## Core Principle

`source.<name>` is an authoring and planning namespace. It is not the final execution contract.

Authors should be able to write model transformations against stable source asset names:

```sql
SELECT
  o.order_id,
  o.customer_id,
  p.payment_value
FROM source.orders AS o
JOIN source.payments AS p USING (order_id)
```

LibreDash should compile that SQL into an adapter-specific execution plan. The model table is the owned physical/cache boundary:

```text
source asset metadata -> planned source reads -> model.<table> -> semantic queries -> dashboards
```

Source assets describe external data. Model tables own cached data. Dashboard and semantic query serving should read only `model.*`.

## Source Assets

Source YAML should describe external metadata:

- connection
- object or path
- format/options
- schema hints or discovered schema
- classification/tags/freshness

Source YAML should not own:

- business fields
- measures
- joins
- model projection policy
- materialized table shape

The `source.*` SQL namespace may exist during planning or materialization, but it should be treated as transient compiler/runtime state.

## Model Tables

Model tables are the physical cache contract. They own:

- output columns and aliases
- primary key and grain
- transform SQL
- casts and business-safe projections
- PII exclusion
- materialization mode

Direct model tables can be compiled from a declared column contract:

```yaml
models:
  orders:
    source: orders
    columns:
      order_id:
      customer_id:
      revenue:
        source_field: payment_value
```

SQL model tables should normally need only their transform SQL:

```yaml
models:
  orders:
    sources: [orders, payments]
    transform:
      sql: |
        SELECT ...
        FROM source.orders o
        JOIN source.payments p USING (order_id)
```

LibreDash now infers source reads from SQL model transforms during materialization. Authored `source_reads` YAML is rejected so projection ownership stays in model SQL and model column contracts.

## Why DuckDB Pushdown Alone Is Not Enough

For normal DuckDB relations, this shape can be optimized by DuckDB:

```sql
CREATE VIEW source.orders AS SELECT * FROM read_parquet(...);

CREATE TABLE model.orders AS
SELECT order_id, customer_id
FROM source.orders;
```

DuckDB can often push the projection into the scan.

Quack is different because the remote query is an opaque SQL string inside a table function:

```sql
SELECT * FROM quack_query(uri, 'SELECT * FROM oeducklake.oe_aravind.fact')
```

DuckDB cannot rewrite the remote SQL string from `SELECT *` to `SELECT order_id, customer_id`. If LibreDash creates a source view with remote `SELECT *`, Quack may prepare or scan a wide fact before the model table projection can help.

Therefore, LibreDash must compile projected source reads before constructing Quack `quack_query(...)` calls.

## Target Planning Pipeline

The target model SQL planning flow:

```text
model transform SQL
  -> source metadata schemas
  -> local synthetic source planning stubs
  -> DuckDB bind/EXPLAIN JSON
  -> source dependency and projection map
  -> adapter-specific execution SQL
  -> materialize model.<table>
```

Planning should not read source data. It should use discovered schemas and local synthetic DuckDB stubs for `source.<name>`.

Example planning stubs:

```sql
CREATE TEMP TABLE source.orders (
  order_id VARCHAR,
  customer_id VARCHAR,
  order_purchase_timestamp TIMESTAMP
);

INSERT INTO source.orders VALUES (
  '__libredash_stub__',
  '__libredash_stub__',
  TIMESTAMP '1970-01-01 00:00:00'
);
```

Then LibreDash can run:

```sql
EXPLAIN (FORMAT json)
SELECT ...
FROM source.orders o
JOIN source.payments p USING (order_id);
```

The planner output should be used to derive:

- referenced source assets
- required columns per source asset
- output model columns and physical types when possible
- whether the query is eligible for full pushdown

## EXPLAIN JSON Findings

DuckDB's JSON explain output is produced by the tree renderer. The result is an array of plan nodes:

```json
[
  {
    "name": "SEQ_SCAN",
    "children": [],
    "extra_info": {
      "Table": "memory.\"source\".orders",
      "Type": "Sequential Scan",
      "Projections": ["order_id", "customer_id"],
      "Filters": "status='delivered'",
      "Estimated Cardinality": "1"
    }
  }
]
```

Important properties:

- The JSON shape is `name`, `children`, and `extra_info`.
- `extra_info` is operator-renderer output, not a formal source-lineage API.
- Physical table scans expose `extra_info.Table` for regular tables/views.
- Physical scans expose `extra_info.Projections` as the projected base column names.
- Filters may appear separately in `extra_info.Filters`.
- Projection lists are emitted as strings or arrays depending on whether DuckDB rendered one or multiple newline-separated values.

The physical scan projection list is the closest currently available DuckDB output for the source read plan we need.

### Planning Stubs Must Not Be Empty

An empty planning table can be optimized into `EMPTY_RESULT`, which removes the scan node and loses the source projection information.

Avoid this:

```sql
CREATE TEMP VIEW source.orders AS
SELECT
  NULL::INTEGER AS order_id,
  NULL::VARCHAR AS status
WHERE false;
```

Prefer synthetic one-row planning tables:

```sql
CREATE SCHEMA source;

CREATE TEMP TABLE source.orders (
  order_id INTEGER,
  customer_id INTEGER,
  status VARCHAR
);

INSERT INTO source.orders VALUES (0, 0, '__libredash_stub__');
```

The data is local synthetic metadata, not external source data. It exists only so the optimizer keeps scans visible during planning.

### Disable Filter Pushdown During Read Planning

With normal optimization, DuckDB may push a filter into the scan. Then the filtered column can disappear from `Projections` because the scan can apply the filter internally:

```json
{
  "Table": "memory.\"source\".orders",
  "Projections": ["order_id", "customer_id"],
  "Filters": "status='delivered'"
}
```

That is not enough if LibreDash will execute the original model SQL against a transient `source.orders` view, because the model SQL still references `status`.

For source-read inference, run planning with filter pushdown disabled:

```sql
SET disabled_optimizers = 'filter_pushdown';

EXPLAIN (FORMAT json)
SELECT order_id, customer_id
FROM source.orders
WHERE status = 'delivered';
```

Then the scan projection includes all columns needed by the unchanged model SQL:

```json
{
  "Table": "memory.\"source\".orders",
  "Projections": ["status", "order_id", "customer_id"]
}
```

This is the correct projection for the projected-source-view strategy.

### `explain_output = 'all'`

DuckDB can return unoptimized logical, optimized logical, and physical plans:

```sql
PRAGMA explain_output = 'all';
EXPLAIN (FORMAT json) SELECT ...
```

The unoptimized logical plan preserves source scans even when the physical optimizer removes them, but it does not expose scan projections. The physical plan exposes scan projections, so the physical plan is the primary input for read planning.

The unoptimized logical plan can still be useful as a sanity check that declared source dependencies match the bound source scans.

### Table Functions

When explaining a table function directly, DuckDB may expose `Projections` but not a stable LibreDash source name:

```json
{
  "name": "READ_CSV",
  "extra_info": {
    "Function": "READ_CSV",
    "Projections": ["status", "order_id", "amount"]
  }
}
```

Therefore source-read inference should explain model SQL against LibreDash-owned `source.<asset>` planning stubs, not final adapter SQL like `read_csv(...)` or `quack_query(...)`.

### Go API Exposure

`duckdb-go` exposes:

- prepared statement result metadata via `ColumnCount`, `ColumnName`, `ColumnType`, and `ColumnTypeInfo`
- statement type via `StatementType`
- profiling trees via `GetProfilingInfo`
- lower-level extracted statement preparation internally

It does not expose a public base-table/base-column lineage map. For now, LibreDash should use SQL `EXPLAIN (FORMAT json)` through `database/sql` for read planning, and prepared statement metadata for output schema/type validation.

### Recommended Read Planner Algorithm

For each SQL model table:

1. Discover source schemas using adapter metadata discovery.
2. Open an isolated planning DuckDB connection or transaction.
3. Create `source` schema and synthetic one-row planning tables for declared sources.
4. Set `disabled_optimizers = 'filter_pushdown'` for the planning query.
5. Run `EXPLAIN (FORMAT json) <model transform SQL>`.
6. Walk the JSON tree and collect scan nodes whose `extra_info.Table` resolves to `source.<asset>`.
7. Normalize `extra_info.Projections` into ordered source column sets.
8. Validate that every discovered scan belongs to the model table's declared sources.
9. Compile adapter-specific projected reads.
10. Materialize the model table using transient projected `source.<asset>` views.

The planner should restore connection settings or use an isolated connection so planning-only optimizer settings never leak into user queries.

### Limits

The EXPLAIN JSON plan is practical, but it is not a dedicated public lineage API. LibreDash should pin expected shapes with tests and keep a narrow compatibility layer around it.

Known limits:

- `extra_info` key names can vary by operator/function.
- `Projections` can be a string or array depending on DuckDB rendering.
- Filter text is not structured enough to parse safely; disabling filter pushdown avoids relying on it for column inference.
- Whole-query pushdown still needs token-aware or AST-aware SQL rewriting. EXPLAIN validates and derives projections; it does not rewrite SQL for us.
- If future optimizer rules remove scans despite synthetic rows and disabled filter pushdown, the planner must fail closed and ask for an explicit override.

## Profiling vs Planning

DuckDB profiling is useful for observing what happened after a query ran. It is not the primary mechanism for source read planning because LibreDash needs the read plan before materialization.

Useful DuckDB tools:

- `EXPLAIN (FORMAT json)`: preferred input for pre-execution planning.
- Prepared/bound statement APIs: useful for result columns and types.
- Profiling / `EXPLAIN ANALYZE`: useful for validation and performance diagnostics after execution.

Profiling can answer “did this query scan what we expected?” It should not be required to answer “what should we ask Quack to read?”

## SQL Rewriting

LibreDash should not blindly string-replace `source.orders` with external objects. Rewriting must be token-aware or AST-aware so it does not replace text inside:

- string literals
- comments
- CTE names
- aliases
- quoted identifiers
- unrelated schema/table names

The safe rewrite scope:

- only qualified table references in `FROM`/`JOIN` positions
- only `source.<asset>` references declared by the model table
- only after DuckDB successfully binds the query against planning stubs

## Execution Strategies

### Source-Reference Rewrite

Default strategy:

1. Derive projected source reads from the bound model SQL.
2. Compile each `source.<name>` ref to an adapter relation expression.
3. Rewrite only executable source table refs in the model SQL.
4. Materialize `model.<table>` from the rewritten SQL.

For local files:

```sql
CREATE TABLE model.orders AS
SELECT order_id, customer_id
FROM (SELECT order_id, customer_id FROM read_csv('orders.csv')) o;
```

The `source.*` namespace is only an authoring and planning namespace. It is not an execution view namespace.

### Whole-Query Pushdown

When all source dependencies share one adapter and connection, and the adapter declares SQL pushdown support, LibreDash may compile the entire model transform into one remote query. Eligibility is decided from the serialized SQL AST and adapter capabilities before projected source-read inference, so valid transforms do not depend on `EXPLAIN` exposing scan projections.

For Quack, author SQL:

```sql
SELECT ...
FROM source.orders o
JOIN source.payments p USING (order_id)
```

could become:

```sql
CREATE TABLE model.orders AS
SELECT *
FROM quack_query(uri, '
  SELECT ...
  FROM remote_schema.orders o
  JOIN remote_schema.payments p USING (order_id)
');
```

This is opt-in per adapter capability because the remote side must support the SQL dialect and functions used by the model transform.

Fallback remains inline source-reference rewrite.

## Adapter Contract

Source adapters should expose capabilities, not leak implementation details into model YAML:

```text
Prepare(ctx, model)
Discover(ctx, source) -> schema metadata
CompileRead(read_plan) -> DuckDB SQL relation
CompileTransform?(transform_plan) -> DuckDB SQL relation
```

`CompileRead` handles inline relation compilation for one source asset.

`CompileTransform` handles whole-query pushdown when supported.

Generated SQL must not contain secret tokens. Credentials belong in DuckDB secrets or adapter-managed credential state.

## Current Source Read Inference

SQL model tables declare source dependencies with `sources: [...]` and write `transform.sql` against `source.<asset>` names. For adapter-capable whole-query pushdown, LibreDash validates and rewrites executable source refs from DuckDB's serialized SQL AST without requiring `EXPLAIN` projections. Otherwise, LibreDash binds and explains the SQL against synthetic planning tables, derives the required source projections, and rewrites executable source refs to adapter relation SQL.

If inline source-reference rewrite needs projections and DuckDB optimizes a declared source scan away, planning fails closed with a clear error instead of emitting a broad `SELECT *` read.

Authored `source_reads` is no longer part of the normal model contract. If it appears in YAML, validation fails and asks the author to rely on inferred reads from `transform.sql`.

## Validation Rules

Compile-time validation should reject:

- dashboard queries that reference `source.*` or raw external assets
- model SQL that references undeclared sources
- unqualified source table references
- unsafe cross-fact dashboard queries
- dimensions or filters unreachable from a query base table
- measures not owned by the selected query base table

Runtime query planning should keep the same checks as defense in depth.

## Open Questions

- Which DuckDB JSON plan nodes provide the most stable source projection details?
- Can DuckDB expose bound base-column lineage directly through a C API in the future?
- How should LibreDash represent `SELECT *` in model SQL: expand from source metadata, reject for remote/wide sources, or allow only for local adapters?
- Which SQL constructs should block whole-query pushdown and force inline source-reference rewrite?
- Should model output column/type discovery come from prepared statements, `DESCRIBE`, or post-materialization schema inspection?
