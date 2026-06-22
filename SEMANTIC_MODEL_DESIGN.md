# Semantic Model Design

LibreDash should feel like a Power BI semantic model without DAX or Power Query.

The product shape is:

```text
sources -> models -> semantic model -> dashboards
```

## Layers

### Sources

Sources describe raw physical data only: where rows come from and how to read them.

```yaml
sources:
  olist_orders:
    connection: olist
    path: olist_orders_dataset.csv
    format: csv

  olist_customers:
    connection: olist
    path: olist_customers_dataset.csv
    format: csv
```

Sources do not define business fields, joins, measures, or dashboard-facing semantics.

### Models

Models are DuckDB-backed model tables prepared for semantic consumption.

Each model can point directly at a source or apply light SQL. This is where casts, cleanup, naming, and grain-alignment preparation live.

```yaml
models:
  orders:
    source: olist_orders
    sql: |
      SELECT
        order_id,
        customer_id,
        CAST(order_purchase_timestamp AS TIMESTAMP) AS purchase_timestamp,
        order_status AS status
      FROM source.olist_orders

  customers:
    source: olist_customers
```

LibreDash is not a transformation framework. Heavy ETL and long model chains belong upstream.

### Semantic Models

Semantic models define the governed data model: tables and relationships.

Tables are not required to be labeled as facts or dimensions. A table becomes fact-like when a measure uses it as its base table. A table becomes dimension-like when it is reached through a safe relationship path.

```yaml
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id

      customers:
        model: customers
        primary_key: customer_id

    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
```

The semantic model owns table identity, relationships, and measures.

### Measures

Measures are governed reusable analytics definitions.

They live on the semantic model and reuse the same query metadata a visual can define inline: table, grain/scope, expression, time behavior, and formatting.

```yaml
semantic_models:
  olist:
    measures:
      defaults:
        table: orders
        grain: order_id
        time: orders.purchase_timestamp
        grains: [day, week, month, quarter, year]

      revenue:
        expr: SUM(orders.revenue)
        format: currency

      order_count:
        expr: COUNT(DISTINCT orders.order_id)
        format: integer
```

This keeps the model close to Power BI: dashboards query the semantic model, and measures are the simplified DAX-like contract. LibreDash measures are SQL aggregate expressions with explicit semantic evaluation metadata, not full DAX.

Named measures are preferred for governed dashboards. Inline measures are allowed for one-off visual authoring and can later be promoted into the semantic model.

If repeated field sets or curated subsets become painful, LibreDash can add optional views later. Views should be a DRY/permission/curation layer, not a required v1 modeling layer.

### Dashboards

Dashboards consume semantic models. Visuals ask for dimensions, measures, filters, sort, and limits.

```yaml
semantic_model: olist

visuals:
  revenue_by_state:
    query:
      dimensions:
        state: customers.state
      measures:
        revenue:
    encode:
      x: state
      y: revenue
```

Dashboards do not reference SQL joins, source files, physical cache tables, or internal materialization names.

### Inline Queries

The visual query shape is the primitive contract. A visual may use named measures or define inline measures with the same metadata.

```yaml
semantic_model: olist

visuals:
  orders_by_state:
    query:
      table: orders
      grain: order_id
      dimensions:
        state: customers.state
      measures:
        order_count:
          expr: COUNT(DISTINCT orders.order_id)
          time: orders.purchase_timestamp
          grains: [day, week, month, quarter, year]
          format: integer
```

In query `measures`, the key is the local output name. A blank value uses the named semantic-model measure with the same name. Use `measure` to alias an existing measure, or `expr` to define an inline measure.

```yaml
query:
  measures:
    revenue:
    orders:
      measure: order_count
    one_off_orders:
      expr: COUNT(DISTINCT orders.order_id)
      table: orders
      grain: order_id
      time: orders.purchase_timestamp
      grains: [day, week, month, quarter, year]
      format: integer
```

Inline measures are useful for quick dashboards and experiments. Named semantic-model measures are the governance path.

Use expanded objects when a query field needs local metadata:

```yaml
query:
  dimensions:
    state:
      field: customers.state
      label: Customer state
  measures:
    revenue:
      measure: revenue
      label: Revenue
```

Row/detail visuals can query fields directly without measures, but must declare a table.

```yaml
semantic_model: olist

tables:
  orders:
    query:
      table: orders
      fields:
        - orders.order_id
        - orders.purchase_timestamp
        - customers.state
        - orders.status
      sort:
        - field: orders.purchase_timestamp
          direction: desc
      limit: 100
```

## Guardrails

- Measures declare one table.
- Measures declare their query grain/scope.
- Inline visual measures follow the same rules as named semantic-model measures.
- Multiple measures in one query must have a compatible table and grain.
- Row/detail queries must declare a table when they do not include a measure.
- Dimensions may come from the base table.
- Dimensions may come from related tables through active many-to-one or one-to-one paths.
- One-to-many, many-to-many, circular, ambiguous, inactive, or missing paths are rejected for dashboard queries.
- Cross-fact measures are not supported in v1.
- Unsafe analysis requires a new model at the correct grain or an explicit future view.

## Naming

Use these names in product language:

- Source
- Model
- Semantic model
- Relationship
- Measure
- Dashboard

Avoid these as primary user-facing concepts:

- Dataset
- Cache table
- OBT

Those are implementation or compatibility terms. If physical optimization is shown, call it a materialization.
