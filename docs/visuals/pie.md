# Pie chart

Use a pie chart for a small set of categories that form one whole.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_share:
    title: Revenue share by segment
    shape: category_value
    renderer: echarts
    type: pie
    options:
      show_labels: true
    query:
      dimensions:
        segment: customers.segment
      measures:
        revenue: null
```
