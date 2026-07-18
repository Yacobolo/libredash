# Histogram

Use a histogram to show the distribution of a binned measure.

{{< chart >}}

## Configuration

```yaml
visuals:
  delivery_time_distribution:
    title: Delivery time distribution
    shape: binned_measure
    renderer: echarts
    type: histogram
    query:
      dimensions:
        delivery_days_bin: orders.delivery_days_bin
      measures:
        order_count: null
      sort:
      - field: delivery_days_bin
        direction: asc
```
