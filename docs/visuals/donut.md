# Donut chart

Use a donut chart for part-to-whole data when the center label adds context.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_share:
    title: Revenue share by segment
    shape: category_value
    renderer: echarts
    type: donut
    options:
      center_label: Revenue
    query:
      dimensions:
        segment: customers.segment
      measures:
        revenue: null
```
