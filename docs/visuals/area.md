# Area chart

Use an area chart to emphasize the magnitude of a measure over an ordered category.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use an ordered dimension and one measure to fill the area between the series and its baseline. The ascending sort preserves the time sequence.

{{< chart id="revenue" >}}

```yaml visual-example=revenue
visuals:
  revenue:
    title: Revenue by month
    description: Tracks monthly revenue over the selected period.
    type: area
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```

## Stacked series

Map `query.series` to status and enable `options.stacked` to show how each status contributes to the monthly total.

{{< chart id="revenue_area_status" >}}

```yaml visual-example=revenue_area_status
visuals:
  revenue_area_status:
    title: Stacked revenue area
    shape: category_series_value
    renderer: echarts
    type: area
    options:
      stacked: true
    query:
      dimensions:
        purchase_month: orders.purchase_month
      series:
        field: orders.status
        alias: status
      measures:
        revenue: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 60
```

## Smoothed line

Enable `options.smooth` to interpolate the boundary, hide symbols to reduce clutter, and add `data_zoom` when the ordered range grows.

{{< chart id="revenue_area_smooth" >}}

```yaml visual-example=revenue_area_smooth
visuals:
  revenue_area_smooth:
    title: Smooth revenue area
    renderer: echarts
    type: area
    options:
      smooth: true
      show_symbols: false
      data_zoom: true
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```
