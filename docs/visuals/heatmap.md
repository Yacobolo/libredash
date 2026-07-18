# Heatmap

Use a heatmap to show values at the intersections of two categorical dimensions.

{{< chart >}}

## Configuration

```yaml
visuals:
  orders_by_state_and_month:
    title: Orders by state and month
    shape: matrix
    renderer: echarts
    type: heatmap
    query:
      dimensions:
        state: customers.state
        purchase_month: orders.purchase_month
      measures:
        order_count: null
```
