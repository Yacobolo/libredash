# Gauge

Use a gauge to communicate one value against a known range or threshold scale.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use the `single_value` shape with one measure when the value is meaningful against an implied range.

{{< visual id="total_orders_gauge" >}}

```yaml visual-example=total_orders_gauge
visuals:
  total_orders_gauge:
    title: Total orders gauge
    shape: single_value
    renderer: echarts
    type: gauge
    query:
      measures:
        order_count: null
```

## Bounded score

Use a bounded measure such as review score so the gauge position has an immediately understood minimum and maximum.

{{< visual id="review_gauge" >}}

```yaml visual-example=review_gauge
visuals:
  review_gauge:
    title: Average review gauge
    shape: single_value
    renderer: echarts
    type: gauge
    query:
      measures:
        review_score: null
```

## Threshold bands

Declare `min` and `max`, then add ordered `thresholds` to give score ranges semantic tones; `progress_width` controls the arc weight.

{{< visual id="review_gauge_thresholds" >}}

```yaml visual-example=review_gauge_thresholds
visuals:
  review_gauge_thresholds:
    title: Review gauge with thresholds
    shape: single_value
    renderer: echarts
    type: gauge
    options:
      min: 0
      max: 5
      progress_width: 16
      thresholds:
        - value: 3
          color: '#cf222e'
        - value: 4
          color: '#bf8700'
        - value: 5
          color: '#1a7f37'
    query:
      measures:
        review_score: null
```
