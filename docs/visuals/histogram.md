# Histogram

Use a histogram to show how raw values are distributed across generated numeric bins.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic distribution

Set `query.table` and one numeric measure so LibreDash can bin raw delivery values and count observations in each interval.

{{< chart id="delivery_histogram" >}}

```yaml visual-example=delivery_histogram
visuals:
  delivery_histogram:
    title: Delivery days histogram
    description: Buckets order volume by delivery duration.
    shape: binned_measure
    renderer: echarts
    type: histogram
    options:
      bin_count: 16
    query:
      table: orders
      measures:
        delivery_days: null
```

## Custom bins

Change the raw measure to revenue and use `options.bin_count` to balance distribution detail against the available chart width.

{{< chart id="revenue_histogram" >}}

```yaml visual-example=revenue_histogram
visuals:
  revenue_histogram:
    title: Revenue histogram
    shape: binned_measure
    renderer: echarts
    type: histogram
    options:
      bin_count: 18
    query:
      table: orders
      measures:
        revenue: null
```

## Labeled bins

Use fewer bins for the bounded review scale and enable `show_labels` when every bin count should be visible without hovering.

{{< chart id="review_histogram" >}}

```yaml visual-example=review_histogram
visuals:
  review_histogram:
    title: Review score histogram
    shape: binned_measure
    renderer: echarts
    type: histogram
    options:
      bin_count: 10
      show_labels: true
    query:
      table: orders
      measures:
        review_score: null
```
