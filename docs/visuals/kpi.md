# KPI

Use a KPI for one primary value with an optional status tone and supporting note.

{{< chart >}}

## Configuration

```yaml
visuals:
  revenue_kpi:
    title: Revenue
    kind: kpi
    shape: single_value
    renderer: html
    type: kpi
    options:
      tone: green
      note: This month
    query:
      measures:
        revenue: null
```
