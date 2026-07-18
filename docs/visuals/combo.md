# Combo chart

Use a combo chart when related measures need different visual encodings.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_and_margin:
    title: Revenue and margin by month
    shape: category_multi_measure
    renderer: echarts
    type: combo
    options:
      series_types:
        revenue: bar
        margin: line
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
        margin: null
```
