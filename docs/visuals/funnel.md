# Funnel chart

Use a funnel chart to show ordered stages whose values usually decrease through a process.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Sort a categorical measure descending so the widest stage appears first and each following stage narrows with its value.

{{< chart id="status_funnel" >}}

```yaml visual-example=status_funnel
visuals:
  status_funnel:
    title: Orders by status funnel
    type: funnel
    query:
      dimensions:
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: desc
```

## Alternate dimension

Replace status with delivery buckets to reuse the funnel for an ordered operational distribution rather than a lifecycle stage.

{{< chart id="delivery_funnel" >}}

```yaml visual-example=delivery_funnel
visuals:
  delivery_funnel:
    title: Delivery speed funnel
    type: funnel
    query:
      dimensions:
        delivery_bucket: orders.delivery_bucket
      measures:
        order_count: null
      sort:
        - field: delivery_bucket
          direction: asc
```

## Aligned labels

Set `funnel_align: left` to anchor the stages, keep labels visible, and use `options.sort` to control the visual stage order independently.

{{< chart id="status_funnel_left" >}}

```yaml visual-example=status_funnel_left
visuals:
  status_funnel_left:
    title: Left aligned status funnel
    type: funnel
    options:
      funnel_align: left
      sort: ascending
      show_labels: true
    query:
      dimensions:
        status: orders.status
      measures:
        order_count: null
      sort:
        - field: value
          direction: asc
```
