# Waterfall chart

Use a waterfall chart to show how category contributions build from one total to another.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use an ordered dimension and one measure with `type: waterfall`; the compiler derives the cumulative fields needed to show how each period contributes to the running total.

{{< visual id="revenue_waterfall" >}}

```yaml visual-example=revenue_waterfall
visuals:
  revenue_waterfall:
    title: Monthly revenue contribution
    description: Shows each month contribution to total revenue.
    type: waterfall
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 18
```

## Alternate measure

Replace revenue with order count to reuse the same running-contribution structure for volume rather than value.

{{< visual id="orders_waterfall" >}}

```yaml visual-example=orders_waterfall
visuals:
  orders_waterfall:
    title: Monthly order contribution
    type: waterfall
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        order_count: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 18
```

## Labels and zoom

Enable `show_labels` for exact contributions and `data_zoom` when many categories make the running sequence too dense.

{{< visual id="revenue_waterfall_labeled" >}}

```yaml visual-example=revenue_waterfall_labeled
visuals:
  revenue_waterfall_labeled:
    title: Labeled revenue waterfall
    type: waterfall
    presentation:
      show_labels: true
      data_zoom: true
    query:
      dimensions:
        category: orders.category
      measures:
        revenue: null
      sort:
        - field: value
          direction: desc
      limit: 12
```
