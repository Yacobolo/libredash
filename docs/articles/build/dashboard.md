# Create a dashboard

A dashboard chooses one workspace semantic model and composes reusable filters, visual queries, tabular queries, and report pages. Build the smallest useful page first, verify its query behavior, and add interactions only after the standalone results are correct.

## Before you begin

Verify the semantic model with direct queries and choose a small decision-oriented page to build first. Prepare expected values for each initial visual at an unfiltered state and at least one filtered state.

Use this sequence:

1. Create the dashboard and one bounded, deterministically sorted visual query.
2. Place that visual on a page with a compact-layout reading order.
3. Add a KPI and verify both against direct semantic queries.
4. Add filters and interactions one at a time.
5. Validate, plan, deploy to development, and review every state.

## Define the dashboard surface

### Create the resource

Create `dashboards/workspaces/sales/dashboards/executive-sales.yaml`:

```yaml
apiVersion: libredash.dev/v1
kind: Dashboard
metadata:
  workspace: sales
  name: executive-sales
  title: Executive Sales
  description: Revenue and order trends for sales leadership.
  tags: [sales, revenue]
spec:
  semanticModel: sales
  visuals:
    revenue_by_month:
      title: Revenue by month
      type: area
      query:
        dimensions:
          purchase_month: orders.purchase_month
        measures:
          revenue:
        sort:
          - field: purchase_month
            direction: asc
        limit: 30
  pages:
    - id: overview
      title: Overview
      grid:
        columns: 12
        row_height: 48
        gap: 16
        padding: 16
      components:
        - id: revenue-trend
          kind: visual
          visual: revenue_by_month
          placement: {col: 1, row: 1, col_span: 12, row_span: 8}
```

The visual definition owns semantic query and presentation settings. The page entry references it by stable ID and owns placement. This separation keeps layout edits from rewriting data logic.

### Design the query result

Names on the left of `dimensions` and `measures` are result aliases consumed by the visual shape. Values on the right refer to semantic fields. Choose clear aliases and keep them stable when custom options or interactions depend on them.

Every chart query should have a bounded limit and deterministic sort. For time series, sort the time field ascending. For ranked bars, sort the value descending and choose a limit that users can read. Do not rely on database default order.

### Add a KPI

KPI visuals use a single-value shape:

```yaml
visuals:
  total_revenue:
    type: kpi
    shape: single_value
    query:
      measures:
        revenue:
    options:
      note: Filtered order revenue
      tone: green
```

Place it on the page with `kind: visual` and `visual: total_revenue`. Its `type: kpi` selects the KPI renderer. The semantic measure supplies empty and formatting behavior; the dashboard supplies context-specific note and tone.

### Add filters after the base query works

Define filters against semantic fields and place filter-card components on the page. Exercise each filter independently before combining several. Use stable URL parameters when users should share filtered links.

## Validate the dashboard

### Discover and validate

Ensure the workspace manifest includes dashboard files, then run:

```sh
libredash validate --project dashboards/libredash.yaml
libredash plan --project dashboards/libredash.yaml
```

Validation checks contract shape and references. The plan shows the resource-level candidate. Neither proves that a visual communicates the right result, so deploy to development and verify the rendered page with representative data.

## Verify the rendered page

Confirm that:

- the dashboard appears in the intended workspace;
- titles, descriptions, and tags support discovery;
- chart and KPI results match direct semantic queries;
- filters change every intended component and no unintended component;
- empty, loading, and failure states are readable;
- component order makes sense for keyboard and compact layouts;
- limits and sorting remain useful for high-cardinality data.

## Troubleshooting

If a visual is empty, first run its semantic query without dashboard filters, then add filters one at a time. If values are correct but order changes between loads, add an explicit sort with a stable tie-breaker. If a compact layout reads poorly, fix component source order and placement together rather than using visual-only CSS reordering.

## Next steps

Continue with [Pages and layout](/docs/guides/build/pages-layout), [Filters and interactions](/docs/guides/build/filters-interactions), and [Tables, matrices, and pivots](/docs/guides/build/tables). Use [Dashboard configuration](/docs/config/dashboard) and [Visual types](/docs/visuals/overview) for exact contracts.
