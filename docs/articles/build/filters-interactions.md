# Filters and interactions

Filters and selections let users change a report without giving the browser unrestricted query control. Dashboard YAML defines allowed fields, operators, option sources, URL state, semantic mappings, and targets; the server validates and applies each command.

This guide describes the currently accepted dashboard configuration. See [Filter and slicer target architecture](/docs/architecture/filters-slicers) for the state, command, targeting, and option-domain design.

## Define dashboard filters

### Define a categorical filter

```yaml
filters:
  state:
    label: State
    description: Limit results to one or more customer states.
    field: customer_state
    predicates:
      - kind: set
        operators: [in, not_in]
    options: {kind: distinct, limit: 50}
filter_bindings:
  state:
    filter: state
    default: {kind: unfiltered}
    selection: {mode: multiple, max_selected_values: 50}
    url: {param: state, encoding: typed_v1}
    pane: {visible: true, order: 10}
```

The definition owns semantic meaning and legal predicates. The binding owns state, scope, targets, selection limits, URL identity, editability, and Filters-pane presentation. Distinct options are loaded lazily in bounded pages. For thousands of identifiers, enable search and keep page sizes conservative.

Optionally present the same report binding as a page slicer:

```yaml
- id: state-filter
  kind: slicer
  binding: {scope: report, id: state}
  presentation: {style: dropdown, search: true}
  placement: {col: 1, row: 1, col_span: 4, row_span: 2}
```

The pane card and slicer are separate shells around the same canonical binding state. Removing either presentation does not remove the binding or its filtering effect.

### Define date and relative-period predicates

```yaml
filters:
  purchase_date:
    label: Purchase date
    field: purchase_date
    predicates:
      - kind: range
      - kind: relative_period
filter_bindings:
  purchase_date:
    filter: purchase_date
    default: {kind: unfiltered}
    url: {param: period, encoding: typed_v1}
    pane: {visible: true, order: 20}
```

Date, timestamp, calendar, timezone, and week-start semantics come from the semantic field. A date derived from a UTC timestamp may differ from a local-business date near midnight; resolve that in the model instead of compensating in each presentation.

### Define a text filter

Text definitions expose only their allowed operator set:

```yaml
filters:
  category:
    label: Category
    field: category
    predicates:
      - kind: comparison
        operators: [contains, equals, starts_with, ends_with, not_contains]
filter_bindings:
  category:
    filter: category
    default: {kind: unfiltered}
    url: {param: category, encoding: typed_v1}
    pane: {visible: true, order: 30}
```

`typed_v1` serializes the canonical typed expression as unpadded base64url. The server parses and normalizes it; the browser does not maintain a second predicate parser. Default and unfiltered values are omitted. Stable parameter names are compatibility-sensitive, so rename them intentionally.

## Control interaction scope

### Scope filter targets

By default, a binding applies to every semantically compatible consumer in its scope. Use `targets: {include: [...]}` or `targets: {exclude: [...]}` when it should affect only part of a page or report. Page targets are component IDs; report targets are qualified `pageID/componentID` identities. Include and exclude are mutually exclusive.

Test combinations, not just filters in isolation. Two individually valid filters can produce an empty intersection, and users should see a deliberate empty state rather than a broken chart.

### Map visual or table selections

Selection interactions map delivered row values back to semantic fields:

```yaml
interaction:
  row_selection:
    toggle: true
    mappings:
      - field: orders.order_id
        fact: orders
        value: order_id
        label: order_id
    targets:
      - revenue_kpi
      - revenue_by_month
```

`value` names a delivered result field. `field` and `fact` establish semantic identity. The server rejects incomplete or forged mappings before applying them. Targets are dashboard definition IDs, not arbitrary CSS selectors or browser element IDs.

Point selection on supported visuals uses the same principle. Keep mapping values typed: a numeric zero, boolean false, string `"0"`, and null are not interchangeable.

## Verify predictable interactions

- Make selection state visually apparent.
- Provide a clear way to toggle or clear it.
- Target only components whose change users can anticipate.
- Avoid cycles where several selections continually redefine one another.
- Verify behavior when page filters and selections are both active.
- Ensure a superseded interaction cannot restore an older result.

Start with standalone correct visuals, then add one interaction at a time. The generated [Dashboard configuration](/docs/config/dashboard) lists current filter and interaction fields.
