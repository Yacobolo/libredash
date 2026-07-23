# KPI

Use a KPI for one primary value with an optional status tone and supporting note.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Neutral tone

Use one measure; `presentation.tone: neutral` presents an informational value without implying positive or negative status.

{{< visual id="total_orders" >}}

```yaml visual-example=total_orders
visuals:
  total_orders:
    type: kpi
    description: Shows the filtered count of distinct orders.
    query:
      measures:
        order_count: null
    presentation:
      note: Filtered order count
      tone: ink
```

## Success tone

Set `presentation.tone: success` for a favorable result and use `presentation.note` to state what the monetary value represents.

{{< visual id="revenue_kpi" >}}

```yaml visual-example=revenue_kpi
visuals:
  revenue_kpi:
    type: kpi
    description: Shows filtered total payment revenue.
    query:
      measures:
        revenue: null
    presentation:
      note: Payment value
      tone: success
```

## Warning tone

Use `tone: amber` to call attention to a value that may need review without presenting it as an error; keep the note concise.

{{< visual id="aov_kpi" >}}

```yaml visual-example=aov_kpi
visuals:
  aov_kpi:
    type: kpi
    description: Shows average order value for the current filters.
    query:
      measures:
        aov: null
    presentation:
      note: Revenue per order
      tone: warning
```

## Attention tone

Use `tone: coral` for a stronger attention state, and pair it with a note because color must not carry the meaning alone.

{{< visual id="review_kpi" >}}

```yaml visual-example=review_kpi
visuals:
  review_kpi:
    type: kpi
    description: Shows average review score for the current filters.
    query:
      measures:
        review_score: null
    presentation:
      note: Average score
      tone: danger
```
