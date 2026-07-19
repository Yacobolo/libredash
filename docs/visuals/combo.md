# Combo chart

Use a combo chart when related measures need different visual encodings.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Multiple measures

Select multiple `query.measures` to render related values against the same category axis. The combo shape assigns each measure its own series.

{{< visual id="revenue_orders_combo" >}}

```yaml visual-example=revenue_orders_combo
visuals:
  revenue_orders_combo:
    title: Revenue and orders by month
    description: Compares monthly revenue and order volume together.
    shape: category_multi_measure
    renderer: echarts
    type: combo
    options:
      series_types:
        Revenue: line
        Orders: column
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
        order_count: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```

## Per-series renderers

Use `options.series_types` to render review score as a line and delivery days as columns while retaining one shared status axis.

{{< visual id="review_delivery_combo" >}}

```yaml visual-example=review_delivery_combo
visuals:
  review_delivery_combo:
    title: Review and delivery by status
    shape: category_multi_measure
    renderer: echarts
    type: combo
    options:
      series_types:
        Review: line
        Delivery days: column
    query:
      dimensions:
        status: orders.status
      measures:
        review_score: null
        delivery_days: null
      sort:
        - field: status
          direction: asc
```

## Dual axes

Enable `options.dual_axis` when the measures use different scales, then assign line and column renderers explicitly with `series_types`.

{{< visual id="revenue_orders_dual_axis_combo" >}}

```yaml visual-example=revenue_orders_dual_axis_combo
visuals:
  revenue_orders_dual_axis_combo:
    title: Revenue and orders dual-axis combo
    shape: category_multi_measure
    renderer: echarts
    type: combo
    options:
      dual_axis: true
      series_types:
        Revenue: column
        Orders: line
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
        order_count: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 60
```
