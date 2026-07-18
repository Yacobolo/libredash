# Tree

Use a tree to explore a hierarchy from parent to child.

{{< chart >}}

## Configuration

```yaml
visuals:
  state_and_status:
    title: State and status tree
    shape: hierarchy
    renderer: echarts
    type: tree
    options:
      roam: false
    query:
      dimensions:
        state: customers.state
        status: orders.status
      measures:
        order_count: null
```
