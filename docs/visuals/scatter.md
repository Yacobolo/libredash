# Scatter chart

Use a scatter chart to compare category positions, expose series, or emphasize individual points.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use an ordered category and numeric measure to place one point per period, making isolated delivery values and gaps easy to spot.

{{< visual id="delivery_scatter" >}}

```yaml visual-example=delivery_scatter
visuals:
  delivery_scatter:
    title: Delivery days scatter by month
    type: scatter
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        delivery_days: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```

## Multiple series

Map status through `query.series` to split points into comparable groups while retaining the same axes.

{{< visual id="delivery_scatter_status" >}}

```yaml visual-example=delivery_scatter_status
visuals:
  delivery_scatter_status:
    title: Delivery days scatter by status
    type: scatter
    query:
      dimensions:
        purchase_month: orders.purchase_month
      series:
        field: orders.status
        alias: status
      measures:
        delivery_days: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 60
```

## Labeled points

Enable labels and place them above larger symbols when exact point values matter and the dataset is small enough to avoid overlap.

{{< visual id="delivery_scatter_labeled" >}}

```yaml visual-example=delivery_scatter_labeled
visuals:
  delivery_scatter_labeled:
    title: Labeled delivery scatter
    type: scatter
    presentation:
      show_labels: true
      label_position: top
      symbol_size: 12
    query:
      dimensions:
        delivery_bucket: orders.delivery_bucket
      measures:
        delivery_days: null
      sort:
        - field: delivery_bucket
          direction: asc
```
