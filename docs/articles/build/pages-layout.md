# Pages and layout

Pages arrange dashboard components on a deterministic grid. Query definitions remain at dashboard scope; the page decides which reusable filters and visuals appear together and where they are placed.

## Build the page structure

### Define the canvas and grid

```yaml
pages:
  - id: overview
    title: Overview
    description: Revenue and order performance at a glance.
    canvas:
      width: 1366
      height: 900
    grid:
      columns: 12
      row_height: 48
      gap: 16
      padding: 16
    components: []
```

The canvas documents the design reference size. The grid creates predictable coordinates and spans. Use a consistent column count, gap, and row height across related pages so users do not experience a different visual rhythm on each route.

### Place components

Each page component has a stable ID, a component kind, exactly the relevant reference, and placement:

```yaml
components:
  - id: revenue
    kind: visual
    visual: revenue_kpi
    placement: {col: 1, row: 1, col_span: 3, row_span: 3}
  - id: revenue-trend
    kind: visual
    visual: revenue_by_month
    placement: {col: 4, row: 1, col_span: 9, row_span: 8}
  - id: order-details
    kind: visual
    visual: orders_table
    placement: {col: 1, row: 10, col_span: 12, row_span: 8}
```

Use `kind: visual` for charts, KPIs, tables, matrices, and pivots; rendering is inferred from the referenced visual's `type`. Use `kind: slicer` for an on-canvas presentation of a report or page filter binding, and `kind: header` for headings. The Filters pane presents those same bindings independently of canvas placement.

Coordinates are one-based. Keep `col + col_span - 1` within the configured column count. Avoid accidental overlaps unless a future component contract explicitly supports layering.

### Design reading order

YAML order should follow the intended document and keyboard order, not just visual coordinates. A practical report order is:

1. page context or header;
2. high-value filters;
3. summary KPIs;
4. primary analytical visual;
5. supporting comparisons;
6. record-level or multidimensional detail.

This order gives compact layouts a sensible fallback and makes the source easier to review. Coordinates still control desktop placement, but source order communicates meaning.

### Size for content

Choose spans based on the information a component must display:

- KPI cards need enough width for formatted values and labels.
- Legends and long category labels require more chart width.
- Time-series charts need enough horizontal space for the selected period.
- Tables need enough height for a useful initial window.
- Slicers need space for selected values, summaries, search, and operators.

Do not solve overcrowding by shrinking every component. Split a page when users are expected to answer distinct questions or when details push the primary analysis below several screenfuls.

## Stabilize and test the layout

### Keep IDs stable

Page component IDs participate in interaction targeting and client state. Renaming `revenue-trend` can break a filter or selection target even when the referenced visual remains unchanged. Treat IDs as local API names: change them intentionally and search the dashboard for references.

Visual definition IDs should also remain stable. Moving a component only requires a placement edit; it should not require copying or renaming its query.

### Test layouts

After deployment to development:

- inspect the page at the reference canvas width;
- test a compact browser width and the mobile navigation shell;
- check long titles, large currency values, and empty states;
- use keyboard navigation to confirm source order is understandable;
- verify charts resize without clipped labels or legends;
- confirm tables remain usable without forcing document-wide horizontal scrolling.

Use the sample Sales dashboard and visual showcase as working layout examples. See [Dashboard configuration](/docs/config/dashboard) for the current page and placement contract.
