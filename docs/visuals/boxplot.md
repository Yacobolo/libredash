# Boxplot

Use a boxplot to compare the distribution of a raw measure across categories.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Delivery distribution

Set `query.table` and select a numeric measure so LibreDash can derive the quartiles, median, whiskers, and outliers from raw delivery values.

{{< chart id="delivery_distribution" >}}

```yaml visual-example=delivery_distribution
visuals:
  delivery_distribution:
    title: Delivery day distribution
    description: Summarizes delivery-day distribution by speed bucket.
    shape: distribution
    renderer: echarts
    type: boxplot
    query:
      table: orders
      dimensions:
        delivery_bucket: orders.delivery_bucket
      measures:
        delivery_days: null
      sort:
        - field: delivery_bucket
          direction: asc
```

## Review distribution

Swap the numeric measure to compare review-score spread with the same `distribution` shape and raw-table query path.

{{< chart id="review_distribution" >}}

```yaml visual-example=review_distribution
visuals:
  review_distribution:
    title: Review score distribution
    shape: distribution
    renderer: echarts
    type: boxplot
    query:
      table: orders
      dimensions:
        status: orders.status
      measures:
        review_score: null
      sort:
        - field: status
          direction: asc
```

## Zoomable distribution

Use revenue as the raw measure and enable `options.data_zoom` when the range contains values that benefit from closer inspection.

{{< chart id="revenue_distribution" >}}

```yaml visual-example=revenue_distribution
visuals:
  revenue_distribution:
    title: Revenue distribution
    shape: distribution
    renderer: echarts
    type: boxplot
    options:
      data_zoom: true
    query:
      table: orders
      dimensions:
        category: orders.category
      measures:
        revenue: null
      sort:
        - field: category
          direction: desc
      limit: 12
```
