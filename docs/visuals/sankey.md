# Sankey

Use a Sankey chart to show weighted flow between two categorical stages.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic flow

Map two dimensions to source and target nodes and one measure to link width, revealing how orders flow from status to delivery speed.

{{< chart id="status_delivery_flow" >}}

```yaml visual-example=status_delivery_flow
visuals:
  status_delivery_flow:
    title: Status to delivery speed
    description: Shows flow from order status to delivery-speed bucket.
    shape: graph
    renderer: echarts
    type: sankey
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

## Alternate flow

Replace the source and target dimensions to inspect category-to-status flow without changing the weighted graph contract.

{{< chart id="category_status_flow" >}}

```yaml visual-example=category_status_flow
visuals:
  category_status_flow:
    title: Category to status flow
    shape: graph
    renderer: echarts
    type: sankey
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

## Spacious nodes

Increase `options.node_gap` when labels or links feel crowded, and tune `curveness` to keep parallel flows visually distinct.

{{< chart id="category_status_flow_spacious" >}}

```yaml visual-example=category_status_flow_spacious
visuals:
  category_status_flow_spacious:
    title: Spacious category to status flow
    shape: graph
    renderer: echarts
    type: sankey
    options:
      node_gap: 18
      curveness: 0.32
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
