# Custom Vega-Lite

Use a custom visualization when the built-in visual types cannot express the analytical view. Custom programs run in a sandbox and receive only the compiled in-memory dataset.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Reference `primary` as the named dataset and use query aliases in encodings. LibreDash rejects network data, inline values, transforms, parameters, and executable expressions.

{{< visual id="custom_monthly_revenue" >}}

```yaml visual-example=custom_monthly_revenue
visuals:
  custom_monthly_revenue:
    title: Monthly revenue
    description: A constrained Vega-Lite bar chart over governed query data.
    type: custom
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
    custom:
      engine: vega_lite
      program:
        data:
          name: primary
        mark: bar
        encoding:
          x:
            field: purchase_month
            type: ordinal
          y:
            field: revenue
            type: quantitative
```
