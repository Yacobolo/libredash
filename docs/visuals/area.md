# Area chart

Use an area chart to emphasize the magnitude of a measure over an ordered category.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_by_month:
    title: Revenue by month
    type: area
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
      sort:
      - field: purchase_month
        direction: asc
```
