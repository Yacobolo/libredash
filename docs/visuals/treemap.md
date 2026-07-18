# Treemap

Use a treemap to compare part-to-whole values when there are many categories.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_by_segment:
    title: Revenue by segment
    shape: category_value
    renderer: echarts
    type: treemap
    query:
      dimensions:
        segment: customers.segment
      measures:
        revenue: null
```
