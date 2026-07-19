# Candlestick chart

Use a candlestick chart to compare open, close, low, and high measures across an ordered category.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Delivery range

Use the `ohlc` shape with an ordered month dimension and a numeric measure to summarize each period as open, high, low, and close values.

{{< visual id="delivery_candlestick" >}}

```yaml visual-example=delivery_candlestick
visuals:
  delivery_candlestick:
    title: Delivery day candlestick by month
    shape: ohlc
    renderer: echarts
    type: candlestick
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        delivery_days_q1: null
        delivery_days_q3: null
        delivery_days_min: null
        delivery_days_max: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```

## Revenue range

Change the measure to revenue and enable `options.data_zoom` so dense monthly ranges remain explorable without changing the OHLC contract.

{{< visual id="revenue_candlestick" >}}

```yaml visual-example=revenue_candlestick
visuals:
  revenue_candlestick:
    title: Revenue OHLC by month
    shape: ohlc
    renderer: echarts
    type: candlestick
    options:
      data_zoom: true
    query:
      dimensions:
        purchase_month: orders.purchase_month
      measures:
        revenue_q1: null
        revenue_q3: null
        revenue_min: null
        revenue_max: null
      sort:
        - field: purchase_month
          direction: asc
      limit: 30
```
