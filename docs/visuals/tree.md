# Tree

Use a tree to show hierarchical paths when parent-child structure should remain explicit.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Two-level hierarchy

Order dimensions from parent to child and use the measure to annotate the resulting state-to-status hierarchy.

{{< chart id="state_status_tree" >}}

```yaml visual-example=state_status_tree
visuals:
  state_status_tree:
    title: State and status tree
    description: Shows customer state and order status as a hierarchy.
    shape: hierarchy
    renderer: echarts
    type: tree
    query:
      dimensions:
        state: orders.state
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 80
```

## Alternate hierarchy

Replace the parent dimension with category to present a different two-level hierarchy using the same tree shape.

{{< chart id="category_status_tree" >}}

```yaml visual-example=category_status_tree
visuals:
  category_status_tree:
    title: Category and status tree
    shape: hierarchy
    renderer: echarts
    type: tree
    query:
      dimensions:
        category: orders.category
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 80
```

## Three-level hierarchy

Add state as an intermediate level, use `initial_depth` to limit the initial expansion, and set `orient` to fit the available reading direction.

{{< chart id="category_state_status_tree" >}}

```yaml visual-example=category_state_status_tree
visuals:
  category_state_status_tree:
    title: Category, state, and status tree
    shape: hierarchy
    renderer: echarts
    type: tree
    options:
      orient: TB
      initial_depth: 2
    query:
      dimensions:
        category: orders.category
        state: orders.state
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 120
```
