# Funnel chart

Use a funnel chart to show the reduction through an ordered process.

{{< chart >}}

## Configuration

```yaml
visuals:
  conversion_funnel:
    title: Conversion funnel
    shape: category_value
    renderer: echarts
    type: funnel
    query:
      dimensions:
        stage: funnel.stage
      measures:
        visitor_count: null
```
