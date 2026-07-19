# Treemap

Use a treemap to compare part-to-whole values when rectangular area communicates magnitude.

Every preview on this page is generated from the YAML shown below it using a fixed documentation dataset.

## Basic

Use one categorical dimension and one measure to size rectangular areas, making dominant categories visible within limited space.

{{< visual id="category_treemap" >}}

```yaml visual-example=category_treemap
visuals:
  category_treemap:
    title: Category revenue treemap
    type: treemap
    query:
      dimensions:
        category: orders.category
      measures:
        revenue: null
      sort:
        - field: value
          direction: desc
      limit: 18
```

## Alternate measure

Replace the dimension and measure to compare revenue by state without changing the category-value shape.

{{< visual id="state_treemap" >}}

```yaml visual-example=state_treemap
visuals:
  state_treemap:
    title: State revenue treemap
    type: treemap
    query:
      dimensions:
        state: orders.state
      measures:
        revenue: null
      sort:
        - field: value
          direction: desc
      limit: 18
```

## Navigable hierarchy

Enable `breadcrumb` and `roam` when readers should navigate into dense or nested rectangles instead of viewing a fixed overview.

{{< visual id="category_treemap_roam" >}}

```yaml visual-example=category_treemap_roam
visuals:
  category_treemap_roam:
    title: Navigable category treemap
    type: treemap
    options:
      roam: true
      breadcrumb: true
    query:
      dimensions:
        category: orders.category
      measures:
        revenue: null
      sort:
        - field: value
          direction: desc
      limit: 18
```
