# Map

Use a map for regional comparisons or observations with geographic coordinates.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Choropleth

A choropleth joins a query dimension to a content-addressed geometry asset. The `join` and `value` properties reference query aliases, not model field names.

{{< visual id="state_order_map" >}}

```yaml visual-example=state_order_map
visuals:
  state_order_map:
    title: Orders by state
    description: Maps order count by Brazilian state.
    type: map
    query:
      dimensions:
        state: orders.state
      measures:
        order_count: null
      sort:
        - field: order_count
          direction: desc
      limit: 27
    geo:
      layers:
        - id: states
          kind: choropleth
          geometry_asset: brazil_states
          join: state
          value: order_count
```

## Points

Point layers bind numeric latitude and longitude query aliases. An optional value can drive future size or label policies without exposing MapLibre configuration.

{{< visual id="order_point_map" >}}

```yaml visual-example=order_point_map
visuals:
  order_point_map:
    title: Order locations
    type: map
    presentation:
      roam: true
    query:
      dimensions:
        latitude: orders.latitude
        longitude: orders.longitude
      measures:
        revenue: null
      limit: 100
    geo:
      layers:
        - id: orders
          kind: point
          latitude: latitude
          longitude: longitude
          value: revenue
```

## Heat

Heat layers aggregate a numeric value around each coordinate. Keep the query bounded so the browser receives a predictable frame.

{{< visual id="revenue_heat_map" >}}

```yaml visual-example=revenue_heat_map
visuals:
  revenue_heat_map:
    title: Revenue concentration
    type: map
    query:
      dimensions:
        latitude: orders.latitude
        longitude: orders.longitude
      measures:
        revenue: null
      limit: 100
    geo:
      layers:
        - id: revenue
          kind: heat
          latitude: latitude
          longitude: longitude
          value: revenue
```

## Density

Density layers emphasize the concentration of observations. The layer needs coordinates but does not require a value binding.

{{< visual id="order_density_map" >}}

```yaml visual-example=order_density_map
visuals:
  order_density_map:
    title: Order density
    type: map
    query:
      dimensions:
        latitude: orders.latitude
        longitude: orders.longitude
      measures:
        order_count: null
      limit: 100
    geo:
      layers:
        - id: orders
          kind: density
          latitude: latitude
          longitude: longitude
```
