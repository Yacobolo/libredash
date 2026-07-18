# Sankey

Use a Sankey diagram to show the volume flowing between named stages.

{{< chart >}}

## Configuration

```yaml
visuals:
  conversion_flow:
    title: Conversion flow
    shape: graph
    renderer: echarts
    type: sankey
    query:
      dimensions:
        source: funnel.source
        target: funnel.target
      measures:
        visitor_count: null
```
