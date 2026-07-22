# Semantic Model Design

LeapView uses a model-scoped, multi-fact semantic layer:

```text
sources -> models -> semantic model -> dashboards
```

Sources describe physical inputs. Models prepare DuckDB tables at useful grains. A semantic model owns the governed relationship graph, conformed dimensions, atomic measures, and derived metrics. Dashboards select those semantic members and contain no SQL.

## Contract

```yaml
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: movielens
  name: activity
spec:
  tables: [ratings, tags, movies]

  relationships:
    - id: ratings_movies
      from: ratings.movie_id
      to: movies.movie_id
      cardinality: many_to_one
    - id: tags_movies
      from: tags.movie_id
      to: movies.movie_id
      cardinality: many_to_one

  dimensions:
    activity_date:
      type: timestamp
      grains: [day, week, month, quarter, year]
      bindings:
        ratings: {field: ratings.rated_at}
        tags: {field: tags.tagged_at}
    release_decade:
      type: string
      bindings:
        ratings:
          field: movies.release_decade
          path: [ratings_movies]
        tags:
          field: movies.release_decade
          path: [tags_movies]

  measures:
    rating_count:
      fact: ratings
      aggregation: count
      empty: zero
    rating_total:
      fact: ratings
      aggregation: sum
      input: {field: ratings.rating}
      empty: "null"
    tag_count:
      fact: tags
      aggregation: count
      empty: zero

  metrics:
    tags_per_rating:
      expression: safe_divide(${tag_count}, ${rating_count})
      format: decimal
```

Facts are inferred from atomic measure ownership. Tables have no semantic `kind`, and the same table may own measures while also serving as the one-side of another fact's relationship.

## Atomic measures

Atomic measures support `sum`, `count`, `count_distinct`, `avg`, `min`, and `max`. `count` has no input; every other aggregation has exactly one input field or scalar expression. Input fields belong to the owning fact. Filter fields may follow safe many-to-one or one-to-one paths.

Scalar expressions are parsed, not interpolated SQL. They support numeric literals, arithmetic, parentheses, `${table.field}` references, and the allowlisted `coalesce`, `nullif`, `abs`, and `round` functions. Aggregate functions and arbitrary SQL are rejected.

Every measure declares its empty behavior. Counts use `zero`; other aggregations may use `zero` or `null`.

## Metrics

Metrics are parsed arithmetic expressions over measures and other metrics. `safe_divide` returns null when its denominator is zero. Unknown members, physical field references, aggregate SQL, invalid types, and dependency cycles fail validation.

Metrics are aggregate-only. Row previews, histograms, distributions, and raw-value operations require a table scope and a typed atomic measure input.

## Conformed dimensions

A semantic dimension has an unqualified name, a canonical type, and a binding for every compatible fact. A binding identifies a qualified physical field and optionally an ordered relationship-ID path. The path may be omitted only for a local field or when exactly one safe path exists.

Date and timestamp dimensions may declare time grains. Each fact compiler casts its binding to the canonical type before stitching.

Qualified physical dimensions remain available to single-fact aggregate and row queries. Multi-fact output dimensions must be conformed semantic dimensions.

## Query planning

A table-scoped aggregate query is single-fact and rejects members owned by another fact. A model-scoped aggregate query expands the metric dependency graph and partitions atomic measures by fact.

Each fact is independently joined only to its required safe dimension paths, filtered, masked, and aggregated. Scalar fact CTEs are cross-joined. Grouped fact CTEs are chained with `FULL OUTER JOIN` on every canonical dimension using `IS NOT DISTINCT FROM`. Stitch keys are coalesced; measure values use only their declared empty policy. Metrics are evaluated after stitching.

Dimension-only model queries union distinct canonical values from every compatible binding, which gives filter controls complete values without inventing a default fact.

## Filters and governance

Conformed filters apply to every participating fact. A fact-local filter in a multi-fact query names its `fact`. Boolean groups must be entirely conformed or resolve to one fact.

Dependency resolution happens before authorization and planning. It returns logical semantic fields, transitive metric dependencies, facts, physical fields, and relationship paths. Model-scoped queries authorize the semantic model and every selected or transitive semantic field, then apply policies from every resolved physical dependency. Row filters are targeted to every affected fact. A mask on any measure or metric dependency rejects the aggregate.

## Product language

Use Source, Model, Model table, Semantic model, Relationship, Conformed dimension, Fact, Measure, Metric, Dashboard, and Materialization. Facts are roles inferred from measures, not authored table classifications. Generated serving tables are internal implementation details.
