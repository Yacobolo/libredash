# Bar chart

Use a bar chart to compare a measure across a ranked set of categories.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_by_category:
    title: Revenue by category
    type: bar
    query:
      dimensions:
        category: orders.category
      measures:
        revenue: null
      sort:
      - field: value
        direction: desc
      limit: 10
```
