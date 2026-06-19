# Finish Technical Debt Cleanup

## Summary

Clean up the remaining debt from the semantic planner migration with one focused pass: make public vocabulary consistent, finish generated DB naming, and split the largest semantic/data files enough that future changes have clear homes.

## Key Changes

- Rename authored YAML collection keys from `metrics_views` to `metric_views` in catalog and dashboard contracts.
- Reject legacy `metrics_views` and `metrics_view` authored keys explicitly; do not add compatibility aliases.
- Normalize validation and product wording to `metric view`, `model table`, and `materialization`.
- Regenerate sqlc platform DB code so materialization job tables produce `MaterializationJob` types.
- Split semantic dashboard contracts/validation/filter helpers and large semantic/data tests into focused files.
- Keep runtime semantics, planner behavior, dashboard visuals, and frontend payloads unchanged.

## Test Plan

- Add semantic contract tests for `metric_views` acceptance and `metrics_views` rejection.
- Keep legacy raw SQL, scalar field, top-level `metrics_view`, KPI, and table shorthand rejection tests.
- Verify generated DB model names and materialization refresh route/permission behavior.
- Run `task test`.
- Run `task dev`, verify the Olist dashboard page and `/updates` stream, then stop the dev server.

## Assumptions

- No backwards compatibility is required.
- `metric_views` is the only authored collection key after this cleanup.
- Focused file splitting is enough; no arbitrary line-count target is required.
- OBTs, rollups, and query-result caches remain out of scope.
