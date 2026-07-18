# Waterfall chart

Use a waterfall chart to explain how positive and negative changes arrive at a total.

{{< chart >}}

## Configuration

```yaml
visuals:
  monthly_recurring_revenue:
    title: Monthly recurring revenue bridge
    shape: category_delta
    renderer: echarts
    type: waterfall
    query:
      dimensions:
        movement: revenue_bridge.movement
      measures:
        start: null
        value: null
```
