# Candlestick chart

Use a candlestick chart to show open, close, low, and high values for each period.

{{< chart >}}

## Configuration

```yaml
visuals:
  daily_price:
    title: Daily price movement
    shape: ohlc
    renderer: echarts
    type: candlestick
    query:
      dimensions:
        day: market.day
      measures:
        open: null
        close: null
        low: null
        high: null
```
