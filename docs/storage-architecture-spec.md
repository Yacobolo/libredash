# LibreDash Storage Architecture Spec

## Summary

LibreDash storage should follow a DuckLake-style split between metadata and data.

SQLite is the catalog and source of truth. DuckDB is one environment-scoped physical data catalog used for materialized tables and query execution. Deployments are isolated by immutable DuckDB namespaces and SQLite metadata, not by separate DuckDB files.

## Goals

- Use one DuckDB database per LibreDash environment/cache root.
- Present storage as DuckDB databases, schemas, and tables enriched with catalog governance metadata.
- Activate deployments by flipping a SQLite pointer, not by moving files.
- Keep rollback cheap by preserving previous deployment table versions until retention removes them.
- Make failed refreshes non-destructive to the active deployment.
- Support deterministic cleanup of unreferenced physical data.

## Non-Goals

- Do not expose DuckDB files as the primary storage abstraction.
- Do not require one DuckDB file per semantic model.
- Do not require one DuckDB file per deployment as the long-term architecture.
- Do not use filesystem layout as the deployment isolation mechanism.
- Do not implement full DuckLake compatibility in v1.

## Architecture

SQLite catalog tables own governance metadata:

- Workspaces and environments.
- Deployments and active deployment pointers.
- Logical tables from workspace model definitions.
- Physical table references for each deployment.
- Materialization runs, row counts, schema hashes, sizes, errors, and refresh timestamps.
- Retention state for expired deployments and orphaned physical tables.

DuckDB owns physical storage:

- Physical materialized tables.
- Query execution.
- System metadata for physical storage inspection.

Physical tables are immutable after validation and addressed by stable internal IDs, not user-facing names. A deployment maps logical names such as `model.orders` to physical relations such as `dep_94b8ef633ecdee010579fc56.tbl_orders_01HF...`.

```text
SQLite:
  workspace=sales
  environment=dev
  active_deployment=dep_94b8...
  model.orders -> duckdb schema dep_94b8..., table tbl_orders_01HF...

DuckDB:
  dep_94b8ef633ecdee010579fc56.tbl_orders_01HF...
  dep_94b8ef633ecdee010579fc56.tbl_customers_01HG...
```

Physical relation names are generated identifiers. Human-readable names come from SQLite metadata.

## Deployment Model

Deployments are immutable catalog versions.

- Each deployment owns a complete set of physical table references for its required model tables.
- A deployment is active only when SQLite marks it active for a workspace and environment.
- Activation is a catalog state change, not a DuckDB file or table move.
- Previous deployments remain addressable for rollback and audit until retention expires them.
- Failed or incomplete deployments are never active and never serve queries.

Deployment states are explicit:

```text
staging -> validated -> active -> expired -> delete_scheduled -> deleted
                  \-> failed
```

Physical tables that exist in DuckDB without SQLite ownership are `orphaned`.

## Query Resolution

Runtime queries never hard-code physical DuckDB names.

Resolution invariants:

- Runtime resolves the active deployment pointer once per request.
- Logical table refs are resolved through SQLite metadata.
- Query planning rewrites logical refs to physical DuckDB refs before execution.
- All DuckDB reads within one dashboard/page refresh, API request, export, or agent query use one deployment version.
- Activation changes made during a request do not affect that request.

Transform SQL that references `model.<table>` uses the same resolver during materialization.

## Cleanup

Cleanup is metadata-driven.

- Retention policy determines when inactive deployments expire.
- Expired deployments move to `delete_scheduled` before physical deletion.
- Physical deletion respects a safety window and active-query grace period.
- Physical DuckDB tables without SQLite ownership are orphans.
- Cleanup supports dry-run inspection before destructive action.
- Cleanup reconciles SQLite ownership against DuckDB system tables before deletion.

This mirrors DuckLake’s separation of snapshot expiration from physical file cleanup.

## Design Defaults

- Use one DuckDB database per LibreDash environment/cache root.
- Use immutable DuckDB schemas and generated physical table names for deployment/table isolation.
- Use SQLite transactions for activation and rollback.
- Treat DuckDB files, WALs, schemas, and physical relation names as governance facts, not user-managed authoring primitives.
- Use SQLite as the authority for logical ownership, lifecycle state, and active deployment pointers.

## Acceptance Criteria

- DuckDB storage can be reconciled with SQLite ownership metadata.
- Rollback does not require rematerialization.
- Failed materialization cannot alter active query results.
- Cleanup can report and remove expired or orphaned physical data.
- Tests prove query routing changes when only the SQLite active deployment pointer changes.
- Tests prove one request cannot mix tables from different active deployment versions.
