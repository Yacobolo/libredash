# Sunburst

Use a sunburst to explore a hierarchy while preserving each level's share of the whole.

{{< chart >}}

## Configuration

```yaml
visuals:
  category_hierarchy:
    title: Category hierarchy
    shape: hierarchy
    renderer: echarts
    type: sunburst
    query:
      dimensions:
        category: orders.category
        subcategory: orders.subcategory
      measures:
        revenue: null
```
