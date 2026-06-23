# Semantic Model Decision

## Summary

LibreDash uses a semantic-model-first BI contract:

```text
sources -> models -> semantic model -> dashboards
```

The browser UI and dashboard YAML query governed semantic models directly. LibreDash does not expose a separate curated query layer between semantic models and dashboards in v1.

## Decision

Use one authored path:

- `sources` describe raw physical inputs.
- `models` describe DuckDB-backed model tables with light preparation.
- `semantic_models` define tables, fields, relationships, and measures.
- `dashboards` reference one semantic model and query its fields/measures.

Generated physical serving shapes are internal optimizations. They are not authored dashboard contracts and should not appear as primary workspace assets.

## Product Vocabulary

| Term | Meaning |
| --- | --- |
| Connection | Global data-access configuration. Secrets are never shown. |
| Source | Raw file/table/object read through a connection. No business semantics. |
| Model | Light DuckDB preparation over one or more sources. |
| Model table | Semantic table exposed by a semantic model. |
| Field | Groupable/filterable semantic field on a model table. |
| Relationship | Governed join path between model tables. |
| Measure | Governed aggregate expression with table, grain/scope, time, and format metadata. |
| Semantic model | The governed business model used by dashboards. |
| Dashboard | Presentation layer that queries a semantic model. |
| Materialization | Internal generated physical serving structure. |

## Authored Shape

```yaml
sources:
  olist_orders:
    connection: olist
    path: olist_orders_dataset.csv
    format: csv

models:
  orders:
    sources: [olist_orders]
    sql: |
      SELECT
        order_id,
        customer_id,
        CAST(order_purchase_timestamp AS TIMESTAMP) AS purchase_timestamp,
        order_status AS status
      FROM source.olist_orders

semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          status:
            expr: status
          purchase_month:
            expr: strftime(purchase_timestamp, '%Y-%m')
            order_expr: strftime(purchase_timestamp, '%Y-%m')

      customers:
        model: customers
        primary_key: customer_id
        fields:
          state:
            expr: customer_state

    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true

    measures:
      defaults:
        table: orders
        grain: order_id
        time: orders.purchase_timestamp
        grains: [day, week, month, quarter, year]

      revenue:
        expr: SUM(orders.revenue)
        format: currency
```

Dashboards query that model directly:

```yaml
semantic_model: olist

visuals:
  revenue_by_state:
    query:
      dimensions:
        state: customers.state
      measures:
        revenue:
```

## Rules

LibreDash should force a safe default path:

1. Sources are raw-only and never define joins, measures, fields, or business logic.
2. Models are light preparation only: casts, cleanup, naming, and grain-alignment preparation.
3. Semantic models own fields, relationships, and measures.
4. Dashboards never reference SQL joins, physical files, source names, or generated serving structures.
5. Measures declare or inherit one table and one grain/scope.
6. Multiple measures in one query must use a compatible table and grain/scope.
7. Dimensions may come from the base table.
8. Dimensions may come from related tables through active `many_to_one` or `one_to_one` paths.
9. One-to-many, many-to-many, circular, ambiguous, inactive, or missing paths are rejected for dashboard queries.
10. Cross-fact measures are rejected in v1.
11. Row/detail queries without measures must declare a table.

## Why This Shape

This keeps the product close to the Power BI mental model: a semantic model is the governed business layer, and measures are the simplified DAX-like contract. We get a clear star-schema workflow without adopting DAX, Power Query, or a general transformation framework.

The semantic model can still be optimized internally. LibreDash may generate physical tables, rollups, or cached results for speed, but those are runtime concerns. Authors model the business graph and dashboard queries remain semantic.

## Future Extensions

If repeated query subsets become painful, add optional semantic views later. A view should be a DRY, permission, or curation layer over the semantic model, not a required v1 modeling layer.

If heavier transformations are needed, they should live upstream. LibreDash can support small local SQL preparation, but it should not become a full transformation orchestrator.
