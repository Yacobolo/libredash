# Architecture overview

LibreDash is a Go monolith with explicit generated contracts and focused browser components. The monolith keeps routing, authorization, project compilation, query execution, and lifecycle state in one deployable application while package boundaries separate domains and infrastructure.

## Server domains

`cmd/libredash` starts the application and CLI. Packages under `internal/` own access, administration, agents, analytics, configuration, dashboards, deployments, execution, managed data, query audit, serving state, storage, workspaces, and UI composition.

Transport packages parse HTTP or Datastar commands and call domain services. Domain services validate authorization and lifecycle invariants. Repository and storage adapters implement SQLite, DuckLake, object storage, filesystem, and external connector behavior.

Avoid introducing a second path around these services. Browser, CLI-backed API, and agent tools should converge on the same authorization and semantic boundaries.

## Configuration and deployment

Project and workspace YAML is loaded and validated as a graph. A project discovers global connections/sources and workspace manifests. Each workspace discovers its model, semantic, dashboard, access, and agent resources.

Project deployment compiles validated candidates into immutable artifacts and serving metadata, then changes the instance's serving pointers to accepted state. Runtime requests read the active deployment rather than mutable repository files. Managed-data revision pins move with the project candidate.

## Analytical storage

The platform SQLite database owns application state: identities, grants, environments, deployments, jobs, audit history, and active serving pointers.

One global DuckLake catalog owns analytical table metadata, snapshots, schema evolution, statistics, and physical-file manifests. Parquet holds analytical data. DuckDB attaches the resolved snapshot and executes materialization and governed BI queries.

The active pointer is a LibreDash concern; snapshot and file ownership are DuckLake concerns. Cleanup reconciles both before expiring metadata or deleting physical files.

## Query execution

Dashboard and headless handlers resolve a workspace, active deployment, semantic model, principal, data policies, filters, selections, sorting, and limits. The semantic query layer turns governed field/measure requests into bounded DuckDB work.

Execution separates interactive reads from refresh writes with independent concurrency, queues, and timeouts. Query cancellation and refresh generations prevent obsolete work from replacing newer state.

## Browser architecture

Go uses gomponents to render page shells and initial signal contracts. Datastar transports server-owned state and commands. Lit custom elements render application chrome, workspace/catalog pages, dashboards, filters, charts, tables, administration, data, and chat/agent surfaces.

Components bind to typed signal paths. They can keep ephemeral presentation state, but authoritative filters, selections, refresh state, permissions, and query results return from the server.

## Generated contracts

- TypeSpec under `api/typespec` defines the versioned headless API.
- TypeSpec under `api/signals` defines UI signal models.
- APIGen produces Go surfaces, OpenAPI, CLI operation registry, and generated models.
- CUE/config-schema code exports JSON Schemas for YAML resources.
- The Cobra command tree generates CLI reference pages.
- Runtime configuration specifications generate Go accessors, environment reference, schema, and example environment.
- Documentation generation composes authored navigation with generated catalogs.

Change a source contract and regenerate; do not patch generated output as an independent authority.

## Deployment units

The product application and public documentation site are separate binaries in one monorepo. They share versioned contracts and examples but have independent HTTP packages and build outputs. This preserves documentation proximity without coupling production application availability to the marketing/docs site.

Read [Runtime architecture](/docs/architecture/runtime), [Datastar signal flow](/docs/architecture/datastar), [Visual plugin architecture](/docs/architecture/visual-plugins), and [Storage architecture](/docs/storage-architecture) for deeper boundaries.
