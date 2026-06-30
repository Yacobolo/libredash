# Implement Configuration-as-Code Resource Graph

## Summary

Implement `cac-spec.md` as the new configuration contract. Remove the old single-catalog format and migrate `dashboards/` to a two-workspace Olist showcase: `sales` and `operations`. Global connections/sources are defined once; each workspace owns its model tables, semantic models, dashboards, access policy, and immutable deployments.

Assumptions:
- No backwards compatibility is required.
- Runtime/UI dashboard routes become workspace-scoped: `/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}`.
- Use no major new dependencies for v1.

## Key Changes

- Replace `dashboards/catalog.yaml` with `dashboards/libredash.yaml` and resource-envelope files for `Connection`, `Source`, `Workspace`, `ModelTable`, `SemanticModel`, and `Dashboard`.
- Add a project compiler that expands deterministic includes, validates references/scopes, builds a normalized graph, and emits per-workspace runtime definitions.
- Move Olist connections/sources into global config; create `sales` and `operations` workspaces that both reference those global sources.
- Update runtime, UI, API, Datastar signals, and asset lineage to require workspace context for dashboard/model operations.
- Add deterministic plan output comparing authored config to the active workspace deployment.
- Keep deployments immutable; rollback reactivates an artifact without recompiling.

## Library Decisions

- Keep `cuelang.org/go` for schemas/validation.
- Keep `gopkg.in/yaml.v3` with `KnownFields(true)` for strict YAML decode.
- Keep `github.com/santhosh-tekuri/jsonschema/v6` for JSON Schema validation where needed.
- Use stdlib `filepath.Glob`, sorted deterministically, for includes.
- Use stdlib path safety helpers and `crypto/sha256` for resource hashes.
- Keep DuckDB parser/JSON/explain analysis for SQL references; do not add a generic SQL parser.
- Implement graph validation and plan diff internally.
- Add `doublestar` only later if recursive `**` includes become required.

## Implementation Changes

- Config/schema:
  - Replace old catalog schema with resource-envelope schemas.
  - Reject duplicate IDs, unknown references, non-deterministic includes, hidden imports, and unsupported schema versions.
- Compiler:
  - Compile platform resources first, then workspace resources.
  - Enforce `Workspace.spec.uses.sources` as the allowed source list.
  - Verify `ModelTable.spec.sources` matches SQL `source."..."` references.
  - Generate fully qualified asset IDs such as `source:olist.orders`, `model_table:sales.orders`, and `dashboard:sales.executive-sales`.
- Runtime/routes:
  - Replace single-workspace assumptions with workspace-indexed runtime services.
  - Remove unscoped `/dashboards/...` routes.
  - Scope dashboard rendering, queries, filters, tables, refreshes, search, and lineage by workspace ID.
- CLI/deployment:
  - Update `validate`, `plan`, and `deploy` to use the project graph.
  - Deploy one explicit workspace to one environment.
  - Store active deployment state per `(workspace, environment)`.
  - Serve active runtime from the configured environment, defaulting local development to `dev`.

## Showcase Migration

- `sales` workspace:
  - focuses on revenue, AOV, categories, order volume, and sales tables.
  - owns `executive-sales`.
- `operations` workspace:
  - focuses on delivery speed, order status, reviews, geography, and operations tables.
  - owns `fulfillment-operations`.
- Both workspaces reuse global Olist sources without duplicating source definitions.

## Test Plan

- Project includes expand deterministically.
- Duplicate IDs, unknown references, cycles, cross-scope references, and hidden imports fail.
- Workspace reads outside `uses.sources` fail.
- SQL source references missing from `ModelTable.spec.sources` fail.
- Two workspaces can reference the same global sources.
- Duplicate dashboard IDs are allowed across workspaces and rejected within one workspace.
- Workspace-scoped dashboard routes render the correct workspace assets and reject cross-workspace lookups.
- Plan output is stable across repeated runs.
- `task ci` passes.
