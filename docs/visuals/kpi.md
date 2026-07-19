# KPI

Use a KPI for one primary value with an optional status tone and supporting note.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Neutral tone

Use one measure with `shape: single_value`; `tone: ink` presents an informational value without implying positive or negative status.

{{< chart id="total_orders" >}}

```yaml visual-example=total_orders
visuals:
  total_orders:
    kind: kpi
    description: Shows the filtered count of distinct orders.
    shape: single_value
    query:
      measures:
        order_count: null
    options:
      note: Filtered order count
      tone: ink
```

## Success tone

Set `options.tone: green` for a favorable result and use `options.note` to state what the monetary value represents.

{{< chart id="revenue_kpi" >}}

```yaml visual-example=revenue_kpi
visuals:
  revenue_kpi:
    kind: kpi
    description: Shows filtered total payment revenue.
    shape: single_value
    query:
      measures:
        revenue: null
    options:
      note: Payment value
      tone: green
```

## Warning tone

Use `tone: amber` to call attention to a value that may need review without presenting it as an error; keep the note concise.

{{< chart id="aov_kpi" >}}

```yaml visual-example=aov_kpi
visuals:
  aov_kpi:
    kind: kpi
    description: Shows average order value for the current filters.
    shape: single_value
    query:
      measures:
        aov: null
    options:
      note: Revenue per order
      tone: amber
```

## Attention tone

Use `tone: coral` for a stronger attention state, and pair it with a note because color must not carry the meaning alone.

{{< chart id="review_kpi" >}}

```yaml visual-example=review_kpi
visuals:
  review_kpi:
    kind: kpi
    description: Shows average review score for the current filters.
    shape: single_value
    query:
      measures:
        review_score: null
    options:
      note: Average score
      tone: coral
```
