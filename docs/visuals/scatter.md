# Scatter chart

Use a scatter chart to compare values across observations and reveal clusters or outliers.

{{< chart >}}

## Configuration

```yaml
visuals:
  orders_by_month:
    title: Orders by month
    shape: category_value
    renderer: echarts
    type: scatter
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        order_count: null
```
