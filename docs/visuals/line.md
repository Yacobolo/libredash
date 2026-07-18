# Line chart

Use a line chart to show a measure changing across an ordered category such as time.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_by_month:
    title: Revenue by month
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
