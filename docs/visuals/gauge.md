# Gauge

Use a gauge to communicate progress toward a single known maximum.

{{< chart >}}

## Configuration

```yaml
visuals:
  target_attainment:
    title: Target attainment
    shape: single_value
    renderer: echarts
    type: gauge
    options:
      max: 100
    query:
      measures:
        attainment_percent: null
```
