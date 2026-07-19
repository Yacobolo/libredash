# Matrix

Use a matrix for grouped rows and measures, optionally split across a column dimension.

{{< visual id="status_matrix" >}}

```yaml visual-example=status_matrix
visuals:
  status_matrix:
    type: matrix
    title: Orders by category and status
    query:
      rows:
        category: orders.category
      columns:
        status: orders.status
      measures:
        order_count: null
        revenue: null
```
