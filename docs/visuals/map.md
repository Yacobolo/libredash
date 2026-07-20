# Map

Use a map for regional comparisons or observations with geographic coordinates.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

Maps use LibreDash's vendored, content-addressed Natural Earth world basemap by default. It is deterministic, works offline, and does not send coordinates or browsing activity to a third-party tile service. Set `presentation.basemap: none` when geographic context should be omitted.

Point and choropleth maps can originate semantic crossfilters. Select a mark by click or tap, or use the visible **Select map data** menu for keyboard access. Blank map space clears only that map's selection. `presentation.roam` controls pan and zoom independently of selection.

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
    interaction:
      point_selection:
        toggle: true
        mappings:
          - field: orders.state
            fact: orders
            value: state
            label: state
        targets: [order_point_map, revenue_heat_map, order_density_map]
```

## Points

Point layers bind numeric latitude and longitude query aliases. An optional value controls marker size without exposing MapLibre configuration. Coordinate layers include a subtle geographic reference grid when no basemap asset is present.

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
        order_id: orders.order_id
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
    interaction:
      point_selection:
        toggle: true
        mappings:
          - field: orders.order_id
            fact: orders
            value: order_id
            label: order_id
        targets: [state_order_map, revenue_heat_map, order_density_map]
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

## Crossfilter two maps

Map interactions use compiled query aliases for `value` and `label`, while `field` names the governed semantic field. The identity value must come from a stable dimension or time field. Explicit `targets` keep filtering predictable.

```yaml
visuals:
  orders_by_state:
    title: Orders by state
    type: map
    query:
      dimensions:
        state: customers.state
      measures:
        order_count: null
    geo:
      layers:
        - id: states
          kind: choropleth
          geometry_asset: brazil_states
          join: state
          value: order_count
    interaction:
      point_selection:
        toggle: true
        mappings:
          - field: customers.state
            fact: orders
            value: state
            label: state
        targets: [customer_locations]

  customer_locations:
    title: Customer locations
    type: map
    presentation:
      roam: true
    query:
      dimensions:
        customer_id: customers.customer_id
        latitude: customers.latitude
        longitude: customers.longitude
      measures:
        order_count: null
    geo:
      layers:
        - id: customers
          kind: point
          latitude: latitude
          longitude: longitude
          value: order_count
    interaction:
      point_selection:
        toggle: true
        mappings:
          - field: customers.customer_id
            fact: orders
            value: customer_id
            label: customer_id
        targets: [orders_by_state]
```

Heat and density layers are display-only. They can receive filters from other visuals, but they cannot originate selections. Lasso, bounding-box, and radius filtering require a future typed spatial-selection interaction rather than semantic datum selection.
