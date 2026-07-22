# Filters and interactions

Filters and selections let users change a report without giving the browser unrestricted query control. Dashboard YAML defines allowed fields, operators, option sources, URL state, semantic mappings, and targets; the server validates and applies each command.

## Define dashboard filters

### Define a multi-select filter

```yaml
filters:
  state:
    type: multi_select
    label: State
    description: Limit results to one or more customer states.
    url_param: state
    operator: in
    values:
      source: distinct
      limit: 50
    field: customers.state
```

Distinct option loading must be bounded. A multi-select is appropriate when cardinality is understandable and users recognize the values. For thousands of identifiers, prefer a more focused search or text-filter workflow rather than loading an enormous option set.

Place the filter on a page:

```yaml
- id: state-filter
  kind: filter_card
  filter: state
  placement: {col: 1, row: 1, col_span: 4, row_span: 2}
```

### Define a date range

Date filters can provide shareable URL bounds and named presets:

```yaml
filters:
  purchase_date:
    type: date_range
    label: Purchase date
    url_param: period
    from_url_param: from
    to_url_param: to
    field: orders.purchase_date
    default:
      preset: all
    presets:
      - {value: all, label: All time}
      - {value: "2018", label: "2018", from: "2018-01-01", to: "2018-12-31"}
```

Choose calendar bounds in the same business timezone and date semantics used by the modeled field. A date derived from a UTC timestamp may differ from a local-business date near midnight; resolve that in the model table instead of compensating in each filter.

### Define a text filter

Text filters can expose an allowed operator set and persist the selected operator separately:

```yaml
filters:
  category:
    type: text
    label: Category
    url_param: category
    operator_url_param: category_op
    default_operator: contains
    operators: [contains, equals, starts_with, not_contains]
    field: orders.category
```

Offer only operators that make sense for the field and data volume. Stable URL parameter names let users bookmark and share report state. Renaming one invalidates old links, so treat it as a compatibility change.

## Control interaction scope

### Scope filter targets

By default, a dashboard filter may participate broadly according to the runtime contract. Use explicit targets when a filter should affect only a subset of page components. Narrow targeting makes dashboards easier to explain and avoids unnecessary refresh work.

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
