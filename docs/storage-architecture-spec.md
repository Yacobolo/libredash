# LibreDash DuckLake Storage Architecture Spec

## Summary

LibreDash storage uses DuckLake as the analytical table catalog and DuckDB as the execution engine.

One metadata catalog stores both LibreDash control-plane tables and DuckLake analytical metadata. Parquet files hold analytical data. DuckDB attaches DuckLake, plans queries, and executes against local files.

Refreshes replace the served data atomically. DuckLake snapshots provide internal consistency and cleanup boundaries; they are not a customer-facing data-versioning model in v1.

## Goals

- Use DuckLake for materialized model tables, snapshots, schema history, statistics, commit metadata, and physical data-file ownership.
- Use the metadata catalog for LibreDash control-plane state: workspaces, environments, active serving pointers, permissions, and application job state.
- Store analytical data as DuckLake-managed Parquet files in the LibreDash data store.
- Execute BI queries through DuckDB attached to the active DuckLake snapshot.
- Activate refreshed data by flipping a metadata pointer, not by moving files.
- Keep failed refreshes non-destructive by serving the previous active snapshot until the new snapshot is committed and validated.
- Support deterministic cleanup of superseded physical data.

## Non-Goals

- Do not expose DuckDB files, internal serving states, or historical snapshots as the normal BI user abstraction.
- Do not require one DuckDB file per semantic model.
- Do not require one DuckDB file per serving state as the long-term architecture.
- Do not use filesystem layout as the serving isolation mechanism.
- Do not duplicate DuckLake catalog semantics inside LibreDash metadata.
- Do not make rollback, time travel, or last-N data-version retention a v1 default.

## Architecture

Each LibreDash instance has one metadata catalog and one analytical data store:

```text
.libredash/
  libredash.db              # LibreDash control-plane tables + DuckLake metadata tables
  data/                     # DuckLake-managed Parquet files
  artifacts/                # workspace bundles
  runtime/                  # ephemeral extracted/runtime files
```

Local and production use the same storage topology. Development mode changes application behavior such as auth bypass, inspectors, logging, and bootstrapping; it does not change catalog or data-store isolation. The local default uses DuckLake's SQLite catalog backend because it supports multiple local clients better than a DuckDB-backed DuckLake catalog. The same architecture can use PostgreSQL as the metadata catalog when LibreDash needs a multi-user serving layer.

LibreDash owns application metadata that DuckLake cannot own:

- Workspaces and environments.
- Active serving pointer: workspace/environment -> DuckLake snapshot id.
- Refresh intent, run history, and serving-state lifecycle.
- Semantic model, dashboard, and permission metadata.
- Refresh job state for work not yet committed to DuckLake.
- Audit records for application actions.

LibreDash must not mirror DuckLake table schemas, row counts, file lists, schema versions, or cleanup queues.

DuckLake owns analytical metadata:

- Schemas and tables used by LibreDash workspaces.
- Snapshots, changesets, authors, commit messages, and commit extra info.
- Table schema versions and schema evolution.
- Data-file manifests and file-level ownership.
- Table and file statistics exposed by DuckLake metadata functions.
- Table layout settings such as compression, row-group size, target file size, partitioning, and sort order.
- Snapshot expiration, files scheduled for deletion, orphan-file detection, and cleanup settings.

Parquet stores physical table data:

- Columnar storage for materialized model tables.
- Local file-store layout managed by DuckLake.
- Data files that DuckLake can compact, expire, copy, or inspect independently of application metadata.

DuckDB owns execution:

- Attaching DuckLake catalogs.
- Running model-table replacement SQL.
- Running dashboard, export, API, and agent queries.
- Reading DuckLake-managed Parquet files through the DuckLake catalog.

A committed DuckLake snapshot is immutable. Later writes create new snapshots. LibreDash maps a workspace/environment to the currently served DuckLake snapshot id. Environment is a serving dimension, not a physical catalog boundary. That snapshot is the consistent analytical state for the current served data.

```text
SQLite:
  workspace=sales
  environment=dev
  active_serving_state=state_94b8...
  state_94b8... -> ducklake snapshot 42

DuckLake:
  snapshot 42:
    model.orders -> data/model/orders/*.parquet
    model.customers -> data/model/customers/*.parquet

DuckDB:
  ATTACH 'ducklake:sqlite:.libredash/libredash.db' AS lake
    (DATA_PATH '.libredash/data', SNAPSHOT_VERSION 42)
  SELECT ... FROM lake.model.orders
```

Human-readable BI semantics and application ownership come from LibreDash metadata. Analytical table state and file ownership come from DuckLake.

## Serving Model

LibreDash is a BI serving layer. Assets define what can be queried. Refreshes replace the served data atomically.

- Each active serving state points to one DuckLake snapshot id.
- Refresh commits all planned table changes in one DuckLake transaction.
- The DuckLake commit message or extra info records workspace id, environment, target asset, semantic digest, artifact digest, source data digest when available, and internal serving-state id.
- After commit and schema validation, LibreDash records the committed DuckLake snapshot id and flips the active serving pointer.
- Failed or incomplete refreshes are never active and never serve queries.
- Refresh history is job history, not retained data-version history.
- Old snapshots are retained only while active query/runtime leases still reference them unless a future policy explicitly enables rollback/time travel.

Serving states are explicit:

```text
staging -> validated -> active -> draining -> delete_scheduled -> deleted
                  \-> failed
```

DuckLake snapshots that are not active and not protected by an in-process query lease are retention candidates.

Snapshot ids are scoped to the metadata catalog. Because environments share the catalog, cleanup must protect every active serving reference in the catalog plus every in-process query lease, not only references for the environment currently being served or inspected.

## Query Resolution

Runtime queries never hard-code serving-state-specific physical files or table names.

Resolution invariants:

- Runtime resolves the active serving pointer once per request.
- DuckDB attaches DuckLake at that snapshot version for the request.
- Each request holds a runtime lease until the query completes, so refresh cutover cannot close or expire the snapshot being read.
- Logical table refs are resolved through the semantic model to stable DuckLake schema/table names.
- All DuckDB reads within one dashboard/page refresh, API request, export, or agent query use one resolved DuckLake snapshot.
- Active pointer changes made during a request do not affect that request.
- DuckDB connections attach DuckLake read-only for query serving when possible.

Transform SQL that references `model.<table>` uses the same resolver during materialization.

## Cleanup

Cleanup is metadata-driven.

- Default retention protects the active snapshot and any snapshot currently held by a query/runtime lease.
- Superseded serving states move to `draining` with `superseded_at`; live query leases, not timestamps, determine cleanup protection.
- Draining states move to `delete_scheduled`/`deleted` once retention reconciliation runs without an active lease protecting their snapshot.
- Server startup treats existing draining states as cleanup-eligible because no in-process query leases survive restart.
- DuckLake snapshots not referenced by the active serving state or an in-process lease are candidates for expiration.
- DuckLake cleanup functions identify files scheduled for deletion and orphaned Parquet files.
- Cleanup supports dry-run inspection before destructive action.
- Cleanup reconciles all LibreDash serving references in the metadata catalog against DuckLake snapshots before expiration.

Snapshot expiration and physical file cleanup remain separate operations.

## Design Defaults

- Use one metadata catalog per LibreDash instance.
- Use SQLite as the local metadata catalog backend and PostgreSQL as the server/multi-user backend.
- Use DuckLake schemas for workspace/table namespaces and metadata columns for environment-specific serving pointers.
- Use immutable DuckLake snapshots for atomic refresh isolation.
- Use local Parquet as the analytical data format.
- Use DuckLake DDL and scoped options for table layout; LibreDash should not own physical file-layout policy.
- Use metadata transactions for active pointer flips.
- Treat DuckDB as a stateless execution engine over DuckLake, not as the durable serving container.
- Use LibreDash control-plane tables as the authority for application ownership, lifecycle state, permissions, and active serving pointers.
- Use DuckLake as the authority for analytical table state, schema versions, snapshot history, statistics, and physical data-file ownership.

## Acceptance Criteria

- LibreDash active serving pointers can be reconciled with DuckLake snapshots.
- Failed refresh cannot alter active query results.
- Cleanup can report and remove expired snapshots and orphaned physical data through DuckLake.
- Tests prove query routing changes when only the active serving pointer changes.
- Tests prove one request attaches exactly one DuckLake snapshot version.
- Tests prove DuckDB query serving does not depend on per-serving-state DuckDB database files.
