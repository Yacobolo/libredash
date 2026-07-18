# Column chart

Use a column chart to compare categories when the category labels are short and naturally ordered.

{{< chart >}}

## Configuration

```yaml
visuals:
  orders_by_month:
    title: Orders by month
    type: column
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        order_count: null
      sort:
      - field: purchase_month
        direction: asc
```
