# Line chart

Use a line chart to show a measure changing across an ordered category such as time.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use one ordered `query.dimensions` field for the horizontal axis and one `query.measures` field for the plotted value. Sorting by month keeps the line chronological.

{{< visual id="revenue_line" >}}

```yaml visual-example=revenue_line
visuals:
  revenue_line:
    title: Revenue line by month
    type: line
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

## Multiple series

Set `shape: category_series_value` and map `query.series` to split the measure into one line per order status.

{{< visual id="revenue_line_status" >}}

```yaml visual-example=revenue_line_status
visuals:
  revenue_line_status:
    title: Revenue line by status
    shape: category_series_value
    renderer: echarts
    type: line
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

## Stepped line

Set `options.step: middle` for discrete changes between periods, hide point symbols for a quieter trace, and enable `data_zoom` for long ranges.

{{< visual id="revenue_line_step" >}}

```yaml visual-example=revenue_line_step
visuals:
  revenue_line_step:
    title: Stepped revenue line
    type: line
    options:
      step: middle
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
