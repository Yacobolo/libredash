# Graph

Use a graph when the relationships between entities matter more than their order.

{{< chart >}}

## Configuration

```yaml
visuals:
  account_relationships:
    title: Account relationships
    shape: graph
    renderer: echarts
    type: graph
    options:
      roam: false
    query:
      dimensions:
        source: relationships.source
        target: relationships.target
      measures:
        relationship_count: null
```
