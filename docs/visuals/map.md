# Map

Use a map to compare measures across a supported geographic boundary.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_by_state:
    title: Revenue by state
    shape: geo
    renderer: echarts
    type: map
    options:
      map: brazil_states
      roam: false
    query:
      dimensions:
        state: customers.state
      measures:
        revenue: null
```
