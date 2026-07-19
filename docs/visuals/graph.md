# Graph

Use a graph to explore weighted relationships between categories as nodes and edges.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic network

Map two dimensions to source and target nodes, then use the measure as edge weight to reveal relationships between statuses and delivery buckets.

{{< visual id="status_delivery_graph" >}}

```yaml visual-example=status_delivery_graph
visuals:
  status_delivery_graph:
    title: Status and delivery network
    description: Shows status and delivery-speed relationships as a network.
    shape: graph
    renderer: echarts
    type: graph
    query:
      dimensions:
        status: orders.status
        delivery_bucket: orders.delivery_bucket
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 40
```

## Alternate relationships

Replace the source dimension with category to inspect a different relationship while retaining the same weighted graph shape.

{{< visual id="category_status_graph" >}}

```yaml visual-example=category_status_graph
visuals:
  category_status_graph:
    title: Category and status network
    shape: graph
    renderer: echarts
    type: graph
    query:
      dimensions:
        category: orders.category
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 60
```

## Circular layout

Set `options.layout: circular` for a stable ring, curve overlapping edges, and use `focus: adjacency` to emphasize connected nodes on interaction.

{{< visual id="category_status_graph_circular" >}}

```yaml visual-example=category_status_graph_circular
visuals:
  category_status_graph_circular:
    title: Circular category and status network
    shape: graph
    renderer: echarts
    type: graph
    options:
      layout: circular
      curveness: 0.28
      focus: adjacency
    query:
      dimensions:
        category: orders.category
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
      limit: 60
```
