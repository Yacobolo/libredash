# Build dashboards

LeapView authoring follows the dependency direction of the project. Start with governed data inputs and finish with presentation. Working in that order makes validation errors local and prevents dashboards from becoming the place where source cleanup and business logic accumulate.

> [!TIP]
> Treat this sequence as a diagnostic boundary as well as a build order. Verify each layer before moving upward so a dashboard never has to compensate for an ambiguous source, grain, or measure.

## Author dashboards in layers

### Authoring sequence

Use this sequence for a new analytical surface:

1. Add or reuse a project connection.
2. Describe physical inputs as project sources.
3. Permit those sources in the target workspace.
4. Build workspace model tables with documented keys and grain.
5. Expose dimensions, measures, metrics, and relationships through a semantic model.
6. Define dashboard filters, visual queries, tables, pages, and layout.
7. Validate locally, exercise semantic queries, and review a deployment plan.

The sequence is also a debugging tool. If a chart returns the wrong total, first determine whether the model-table grain is correct, then test the semantic measure, and only then inspect dashboard filters and presentation.

### Keep each layer focused

Project sources document physical input identity and shape. Model tables parse, normalize, join, and materialize reusable analytical rows. Semantic models define business meaning and valid relationships. Dashboards select and present that meaning.

Avoid these common shortcuts:

- putting environment-specific URLs or secrets in dashboard YAML;
- repeating source parsing in several model tables without a clear reason;
- embedding business formulas separately in each visual;
- joining incompatible grains because the fields happen to share a name;
- using page placement entries as the definition of a query;
- loading unbounded chart or table results.

## Validate as you build

### Work in small validated steps

Validate after adding each resource layer:

```sh
leapview validate --project dashboards/leapview.yaml
```

Once the project is valid, inspect how it differs from the active target:

```sh
leapview plan \
  --project dashboards/leapview.yaml \
  --environment dev \
  --target "$LEAPVIEW_TARGET" \
  --token "$LEAPVIEW_API_TOKEN"
```

The local plan is useful even without a target; a target-aware plan adds active deployment differences. Use `--json` in automation and keep human-readable output for review.

### Test below the dashboard

Before debugging a report component, inspect the semantic surface directly:

```sh
leapview semantic-models describe sales \
  --workspace sales \
  --target "$LEAPVIEW_TARGET" \
  --token "$LEAPVIEW_API_TOKEN"
```

The generated [`semantic-models` command reference](/docs/cli/semantic-models) includes dataset discovery, field listing, preview, explain, and query operations. These commands help distinguish semantic or data failures from rendering problems.

### Review checklist

Before deployment, confirm:

- all source and resource references resolve;
- model-table keys and grains match observed data;
- measure results are correct for filtered and empty inputs;
- relationships have valid cardinality;
- chart, option, and table queries are bounded and deterministically sorted;
- component IDs and filter URL parameters are stable;
- the page works at desktop and compact widths;
- the plan contains only the intended resource and revision changes.

The following guides walk through each layer. Use generated configuration reference pages for exact fields; these guides explain how the contracts should work together.
