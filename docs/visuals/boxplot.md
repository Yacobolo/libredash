# Boxplot

Use a boxplot to compare the distribution of a measure across categories.

{{< chart >}}

## Configuration

```yaml
visuals:
  delivery_time_distribution:
    title: Delivery time distribution
    shape: distribution
    renderer: echarts
    type: boxplot
    query:
      dimensions:
        state: customers.state
      measures:
        minimum: null
        first_quartile: null
        median: null
        third_quartile: null
        maximum: null
```
