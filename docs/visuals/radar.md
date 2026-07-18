# Radar chart

Use a radar chart to compare several measures on one shared scale.

{{< chart >}}

## Configuration

```yaml
visuals:
  service_quality:
    title: Service quality
    shape: category_value
    renderer: echarts
    type: radar
    query:
      dimensions:
        measure: service_quality.measure
      measures:
        score: null
```
