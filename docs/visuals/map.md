# Map

Use a map for regional comparisons or observations with geographic coordinates.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

Maps use LeapView's pinned, OSM-derived vector basemap by default. It retains global context through zoom 6 and adds South America business-region detail through zoom 10, including the roads and place labels needed by the Brazil showcase. The PMTiles archive, style, glyphs, and sprites are content addressed and served from LeapView's own origin, so rendering never sends governed coordinates or browsing activity to a third-party tile service. Set `geo.basemap: blank` when geographic context should be omitted.

Production operators can publish the verified inventory to S3-compatible managed object storage with `task map-assets:publish MAP_ASSET_S3_BUCKET=...`. Publication is conditional and idempotent: existing keys must match the compiled digest, size, content type, and immutable cache policy, and conflicting objects are rejected instead of overwritten. Route the published `map-assets/` prefix through the application origin or edge proxy so browser requests remain same-origin.

After the edge route is live, run `task map-assets:verify MAP_ASSET_BASE_URL=https://dash.example`. The verifier checks every content-addressed URL, immutable caching, media types, byte-range support, complete digests for styles, glyphs, and sprites, and exact first/last ranges for the PMTiles archive. `map-assets:publish` accepts the same optional `MAP_ASSET_BASE_URL` to make this a single publish-and-verify gate.

LeapView verifies the complete installed package before opening instance state. The same verifier backs the `mapAssets` readiness check: unchanged files use cached metadata, while any changed file is rehashed and a missing or mismatched asset immediately makes `/readyz` return `503`.

Point and choropleth maps can originate semantic crossfilters. Select a mark by click or tap, or use the visible **Select map data** menu for keyboard access. Blank map space clears only that map's selection. Camera, zoom, reset, compass, label density, and light/dark basemap themes are typed under `geo`.

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
      theme: light
      label_density: normal
      controls: {zoom: true, reset: true, compass: true}
      layers:
        - id: states
          kind: choropleth
          geometry_asset: brazil_states
          join: state
          value: order_count
          tooltip: [state, order_count]
          color:
            kind: sequential
            palette: teal
            null_color: "#d8dee4"
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

The Visual Showcase includes a dedicated `chart-map-scale` page backed by exactly one million deterministic locations. It demonstrates the production spatial-window path: LeapView aggregates the governed viewport at low zoom, returns raw governed points only when the visible cardinality fits, and never sends more than 5,000 rendered features to the browser.

{{< visual id="order_point_map" >}}

```yaml visual-example=order_point_map
visuals:
  order_point_map:
    title: Order locations
    type: map
    query:
      dimensions:
        order_id: orders.order_id
        latitude: orders.latitude
        longitude: orders.longitude
      measures:
        revenue: null
      limit: 100
    geo:
      camera: {mode: fit_data, padding: 32, max_zoom: 9}
      controls: {zoom: true, reset: true, compass: true}
      layers:
        - id: orders
          kind: point
          latitude: latitude
          longitude: longitude
          value: revenue
          label: order_id
          tooltip: [order_id, revenue]
          size: {minimum_radius: 5, maximum_radius: 28}
          stroke: {color: "#ffffff", width: 1.5, opacity: 1}
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
      theme: dark
      layers:
        - id: revenue
          kind: heat
          latitude: latitude
          longitude: longitude
          value: revenue
          heat: {radius: 28, intensity: 1.15}
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
          heat: {radius: 22, intensity: 1.35}
```

## Reference boundary

Reference layers add immutable, content-addressed point, line, or polygon context without joining query values into the geometry. They are display-only.

{{< visual id="state_reference_map" >}}

```yaml visual-example=state_reference_map
visuals:
  state_reference_map:
    title: Brazil state reference boundaries
    type: map
    query:
      dimensions:
        state: orders.state
      measures:
        order_count: null
      limit: 27
    geo:
      basemap: blank
      layers:
        - id: state_boundaries
          kind: reference
          geometry_asset: brazil_states
          color: {kind: sequential, palette: blue, null_color: "#d8dee4"}
          stroke: {color: "#57606a", width: 1.5, opacity: 1}
          opacity: 0.12
```

## Paths

Path layers group coordinate rows by a stable path alias and order vertices deterministically. Use them for governed routes, flows, and trajectories rather than routing-service output.

{{< visual id="state_order_paths" >}}

```yaml visual-example=state_order_paths
visuals:
  state_order_paths:
    title: State order paths
    type: map
    query:
      dimensions:
        state: orders.state
        order_id: orders.order_id
        latitude: orders.latitude
        longitude: orders.longitude
      measures:
        revenue: null
      limit: 100
    geo:
      controls: {zoom: true, reset: true, compass: true}
      layers:
        - id: state_paths
          kind: path
          latitude: latitude
          longitude: longitude
          path: state
          order: order_id
          value: revenue
          tooltip: [state, revenue]
          stroke: {color: "#0969da", width: 3, opacity: 0.9}
          line: {width: 3, curvature: 0}
          opacity: 0.9
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
    query:
      dimensions:
        customer_id: customers.customer_id
        latitude: customers.latitude
        longitude: customers.longitude
      measures:
        order_count: null
    geo:
      controls: {zoom: true, reset: true, compass: true}
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
      spatial_selection:
        gestures: [box, lasso, radius]
        latitude:
          source: latitude
          field: customers.latitude
          fact: orders
        longitude:
          source: longitude
          field: customers.longitude
          fact: orders
        targets: [orders_by_state]
```

Point and choropleth marks originate semantic datum selections. A map with stable coordinate fields can additionally originate exact bounding-box, lasso, and radius filters through `spatial_selection`; those filters are compiled against governed latitude and longitude fields and apply only to explicit targets. Heat and density layers remain display-only marks, but they can receive both semantic and spatial filters from another map.
