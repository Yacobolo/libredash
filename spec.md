# LeapView Target Architecture

This document is the architectural north star for LeapView. It defines the intended product boundaries, dependency rules, runtime model, storage ownership, and scaling model. It is normative: implementation choices are evaluated against this architecture.

LeapView is a feature-oriented modular monolith with ports and adapters. One LeapView deployment is a complete, vertically scalable, single-node product and the unit of ownership, operation, failure isolation, backup, and recovery.

## Architecture Thesis

LeapView optimizes for local correctness and operational independence:

- One process owns the HTTP server, background work, runtime generations, and lifecycle coordination for one node.
- One SQLite database owns node-local control-plane state.
- One process-owned DuckDB instance owns node-local analytical execution and the DuckDB-backed DuckLake catalog.
- Runtime generations pin DuckLake snapshots, compiled plans, and cache scopes; they do not own DuckDB instances.
- Authored YAML in Git is the source of truth for product assets.
- Capability boundaries are explicit even though capabilities share one process and deployment.
- Vertical scaling is the primary scaling mechanism within a node.
- Product scale-out uses independent deployments partitioned by tenant, data domain, or operational boundary.

LeapView is not a distributed database, clustered application, or cross-node query engine. Independent deployments do not share control-plane transactions, runtime state, analytical catalogs, page streams, or refresh coordination.

## Core Rules

- Package by product capability first.
- Keep domain and use-case code free of transport, persistence, filesystem, DuckDB, Datastar, gomponents, and model-provider details.
- Define small ports at the consumer boundary.
- Let adapters import external systems and generated code; never let domain or use-case packages import adapters.
- Split packages by cohesion, workflow, dependency pressure, or test friction, not by generic layers.
- Prefer explicit composition over hidden service locators or broad runtime objects.
- Make immutable serving and compiler outputs immutable by construction, not merely by convention.
- Use typed Go ports and direct calls for synchronous invariants. Do not insert brokers, serialization, service discovery, or RPC to simulate distribution within a node.
- Use process-local fan-out only for ephemeral, reconstructible notifications. Represent durable asynchronous work in SQLite jobs or transactional outbox records.
- Potential extraction boundaries use coarse, transport-neutral contracts. Extraction adds an adapter; it does not introduce transport concerns into domain or use-case code.
- Every operation has explicit authorization, resource bounds, cancellation, idempotency, and audit behavior where applicable.
- No capability reaches through another capability's repository or storage schema.

Dependencies point inward:

```text
HTTP / CLI / Datastar / SQLite / DuckDB / filesystem / object storage / OpenAI
        -> capability adapters
        -> capability use cases
        -> capability domain types and ports
```

Allowed:

```text
deployment/http       -> deployment
deployment            -> release
deployment            -> servingstate
deployment            -> runtimehost ports
project/compiler      -> analytics/model
project/compiler      -> dashboard/report
dashboard/http        -> dashboard/stream
dashboard             -> analytics/query contracts
analytics/duckdb      -> analytics/query
refresh               -> analytics/materialize ports
refresh               -> manageddata ports
```

Forbidden outside adapters and composition:

```text
servingstate          -> servingstate/filesystem
servingstate          -> runtimehost implementation
servingstate          -> sqlc rows
dashboard/report      -> Datastar
analytics/query       -> DuckDB connection details
workspace             -> http.Request
agent                 -> OpenAI request payloads
capability A          -> capability B's SQLite adapter
```

## Deployment And Scaling Model

One LeapView deployment is a self-contained node:

```text
LeapView node
  process
    HTTP, API, CLI-facing server
    Datastar page streams
    compiler and deployment coordination
    governed query execution
    refresh and maintenance workers

  state
    SQLite control-plane database
    DuckDB-backed DuckLake catalog
    Parquet analytical data
    immutable artifacts
    ephemeral runtime files
```

Node invariants:

- A node can build, deploy, refresh, govern, query, back up, restore, and serve its assets without another LeapView service.
- A node serves exactly one configured environment; environment isolation is achieved with separate deployments, not shared runtime state inside one node.
- SQLite and DuckDB catalog files are never mounted as writable shared state across hosts or opened by independent DuckDB instances.
- One process-owned DuckDB instance serves bounded read and refresh connections against the node catalog.
- Process-local brokers, runtime registries, caches, and locks coordinate only the node that owns them.
- Node-local operations never require a cross-node transaction.
- A node continues serving known routes and active assets without a global control plane.
- Backup and recovery cover the complete node state, including SQLite, DuckLake metadata, analytical files, artifacts, and required secrets or configuration.

Vertical scaling rules:

- Query, refresh, stream, cache, and control-plane resources have explicit node-wide limits.
- Workload admission provides bulkheads by workload class and workspace.
- Interactive reads, exports and agent work, refresh writes, and maintenance work have separate limits.
- A workspace may borrow idle analytical capacity but cannot monopolize queued work, retained cache, logical result budgets, or refresh capacity when other workspaces demand service.
- Capacity limits are measurable and validated with load tests at the maximum supported node size.
- Overload is rejected or shed before unbounded work reaches DuckDB, SQLite, page streams, or external connectors.

Fleet scale is achieved by adding independent LeapView deployments. Placement by tenant, data domain, or operational boundary is external to a node. Each deployment retains its own source of truth, authorization, serving state, analytical catalog, and failure domain.

## Capability Map

Top-level capability ownership:

```text
internal/
  project/
  workspace/
  access/
  manageddata/
  analytics/
  dashboard/
  agent/
  release/
  deployment/
  servingstate/
  refresh/
  runtimehost/
  workload/
  platform/
```

- `project`: authored project manifest, cross-capability compilation, normalized immutable workspace bundles, and stable asset graph extraction.
- `workspace`: workspace identity, node-local catalog surface, asset discovery, asset graph views, and workspace read models.
- `access`: principals, groups, roles, permissions, authorization decisions, credentials, tokens, sessions, and access auditing.
- `manageddata`: upload protocols, ingestion, connection revisions, source bindings, retained blobs, runtime views, and managed-data lifecycle.
- `analytics`: source and connection contracts, model tables, semantic models, query planning, query execution, materialization, connectors, and DuckDB adapters.
- `dashboard`: report pages, filters, visuals, BI tables, interactions, page state, typed query intents, and dashboard signal contracts.
- `agent`: conversations, runs, transcripts, tools, policy-filtered operation exposure, and model interaction ports.
- `release`: immutable project release manifests, workspace artifact intake, content verification, and release finalization.
- `deployment`: environment rollout of a release, multi-workspace activation coordination, rollback intent, and deployment status.
- `servingstate`: one workspace's immutable serving generation, artifact identity, validation state, analytical snapshot reference, and lifecycle invariants.
- `refresh`: refresh definitions, schedules, jobs, generations, materialization orchestration, data-version cutover, and supersession behavior.
- `runtimehost`: process-local active runtime lifecycle, prepared runtime publication, leases, draining, and closure.
- `workload`: node-local admission policy, workload and workspace fairness, deadlines, queue bounds, and admission telemetry. It imports no product capabilities and stores no durable work.
- `platform`: low-level node infrastructure: SQLite opening and migrations, process-level paths, backup primitives, and shared infrastructure configuration.

`admin` is not a domain capability. It is an interface surface over capability-owned use cases and read models. `admin/http` may compose read models but must not own their business workflows.

`internal/api` contains framework-neutral public wire contracts. `internal/ui` contains shared render primitives and typed UI transport contracts. Neither is a business capability.

## Capability Context Map

The capability graph is directed. Package-level acyclicity is necessary but not sufficient; reciprocal dependencies between top-level capabilities are forbidden unless both depend on a smaller neutral contract owned by neither implementation.

Primary relationships:

```text
project/compiler
  -> workspace contracts
  -> analytics/model contracts
  -> dashboard/report contracts
  -> access policy contracts
  -> refresh definition contracts

release
  -> project artifact validation port
  -> workspace identity port
  -> servingstate creation port

deployment
  -> release read port
  -> servingstate activation ports
  -> manageddata binding port
  -> runtimehost publication port

refresh
  -> servingstate read/create ports
  -> manageddata resolution port
  -> analytics materialization port
  -> runtimehost publication port

dashboard
  -> analytics query contracts

agent
  -> governed product-operation ports

composition, worker adapters, analytics execution adapters
  -> workload admission port
```

Rules:

- `analytics` never imports dashboard or workspace implementations.
- `dashboard` never constructs analytics adapters.
- `workspace` never owns analytics or dashboard behavior.
- `project/compiler` is the integration boundary allowed to understand multiple authored contract families.
- Bridge adapters belong to the consuming capability or the composition root, not to the provider's domain package.
- Cross-capability DTOs are explicit contracts; arbitrary domain structs are not shared as convenience models.
- Every top-level package is declared in the context map and has one accountable owner.

## Product Contract

The authored product contract is:

```text
sources -> models -> semantic model -> dashboards
```

LeapView is assets-as-code. Authored YAML in Git is the source of truth. The project compiler turns authored contracts into immutable normalized workspace bundles and stable asset graphs. Serving states publish those bundles. Runtime stores never become authoring sources.

The public product schema does not contain:

- metric views
- cache tables
- generated serving tables
- DuckDB secrets or attach statements
- physical runtime relation names

Those concepts are internal runtime implementation details or invalid authored input.

`semantic dataset` is allowed only as a headless API and agent-facing alias for a semantic model table. Domain code uses `model table` or `table` unless translating the public BI API contract. There is no parallel dataset domain model.

YAML contract ownership:

- `project` owns the project entrypoint and resource discovery.
- `analytics/model` owns source contracts, connection contracts, model table contracts, semantic model contracts, fields, relationships, measures, and materialization definitions.
- `dashboard/report` owns dashboards, pages, filters, visuals, tables, and interactions.
- `access` owns authored access policy contracts.
- `refresh` owns authored refresh definitions and schedules.
- `project/compiler` owns validation and normalization spanning multiple contract families.

## Package Shape

Use flat capability packages until cohesion breaks. Split by workflow or adapter, never by generic horizontal layer.

```text
project/
  manifest/       project entrypoint contract
  compiler/       loading, cross-contract validation, normalization, graph extraction
  artifact/       immutable compiled bundle contract

analytics/
  model/          semantic contracts, fields, relationships, measures
  query/          governed query requests, planning, path safety, SQL plans
  materialize/    materialization behavior and analytical write ports
  connectors/     connector registry, capabilities, option schemas
  duckdb/         DuckDB execution adapter

dashboard/
  report/         dashboard/page/filter/visual/table contracts
  stream/         page snapshots and update flow
  command/        filter, selection, table-window, refresh command handling
  datastar/       signal decoding, patch keys, SSE serialization
  http/           route handlers
  ui/             HTML and gomponents rendering adapter

manageddata/
  control/        upload and revision use cases
  binding/        serving-state binding resolution
  storage/        blob and multipart storage ports
  filesystem/     local storage adapter
  s3/             object storage adapter
  sqlite/         control-plane persistence adapter

release/
  finalize/       release verification and finalization
  sqlite/         release persistence adapter
  http/           release API adapter

deployment/
  activate/       multi-workspace activation use case
  rollback/       rollback intent and validation
  sqlite/         deployment persistence adapter
  http/           deployment API adapter

servingstate/
  state.go        shared domain language and lifecycle invariants
  validate/       bundle validation use case
  sqlite/         serving-state persistence adapter
  filesystem/     artifact storage adapter

refresh/
  plan/           dependency-aware refresh planning
  schedule/       schedule evaluation
  run/            durable job and generation behavior
  sqlite/         refresh persistence adapter

workspace/
  catalog/        node-local asset discovery and workspace identity
  sqlite/         read models and repositories
  http/           REST and UI handlers
  datastar/       workspace signal patches
```

Avoid global horizontal packages:

```text
handlers
services
repositories
models
utils
helpers
```

These names are acceptable only inside a capability and only when the package remains narrow and cohesive.

Shared contracts belong to the capability that owns their meaning. For example, a governed query request belongs to `analytics/query`; it does not belong in a generic root-level `dataquery` package.

## Project Compiler

Authored YAML contracts have one compilation boundary:

```text
project/manifest
  + workspace contracts
  + analytics/model
  + dashboard/report
  + access policy
  + refresh definitions
        -> project/compiler
        -> immutable compiled workspace bundles
        -> stable asset graphs
        -> versioned capability projections
```

The compiler owns cross-contract validation and normalization:

- Project resources resolve to declared workspaces.
- Catalog entries resolve to semantic models and dashboards.
- Dashboard semantic-model references resolve to loaded semantic models.
- Dashboard fields, measures, filters, tables, and visuals resolve against semantic contracts.
- Access policy references resolve to stable securable asset IDs.
- Refresh targets resolve to model tables or semantic models.
- Unsupported vocabulary is rejected at the compilation boundary.
- Runtime consumers never re-parse authored YAML.
- Serving state, UI, API, agents, and storage adapters consume compiler-produced projections instead of rediscovering lineage by walking arbitrary domain internals.

Compiled workspace bundles are immutable by construction:

- Collections are private and exposed through read-only lookup or iteration methods.
- Constructors defensively copy caller-owned slices, maps, pointers, and raw payloads.
- Capability projections expose only the data required by their consumer.
- A digest covers the canonical compiled representation and referenced managed-data pins.
- Runtime code cannot mutate compiled configuration.

Asset graph rules:

- Every authored object users can discover, govern, diff, or trace is an asset.
- Logical asset IDs are stable across serving states, such as `semantic_model:olist` or `visual:executive-sales.revenue`.
- Serving-state-scoped snapshot IDs may change per serving state.
- Asset payloads are explicit versioned projections, such as `semantic_model.v1`, `model_table.v1`, `measure.v1`, `dashboard.v1`, and `visual.v1`.
- Persisted payloads are never raw `json.Marshal` output of arbitrary domain structs.
- The complete authored YAML is retained in the immutable artifact.
- Read paths load exact supported projections; they do not repair, migrate, or reinterpret stored graph shapes.

## Release, Deployment, And Serving Lifecycle

The publication lifecycle is explicit:

```text
authored project
  -> compiled workspace artifacts
  -> immutable release
  -> deployment candidate for one environment
  -> prepared workspace runtimes
  -> durable activation transaction
  -> atomic in-process runtime publication
  -> old generations drain
  -> retention cleanup
```

Ownership:

- `release` verifies artifact membership, content digests, completeness, and immutability.
- `servingstate` represents one workspace generation and validates its legal state transitions.
- `deployment` coordinates one release across all targeted workspaces in an environment.
- `runtimehost` prepares and publishes process-local executable generations.
- `manageddata` resolves and retains the exact source revisions required by each candidate.
- `analytics` prepares executable query and materialization services.

Activation invariants:

- Every candidate runtime is fully prepared before durable activation begins.
- A refresh first commits an immutable candidate DuckLake snapshot. That snapshot is not serving state until activation records its exact snapshot identity.
- Durable activation changes all targeted workspace pointers in one SQLite transaction.
- SQLite activation and DuckLake snapshot creation do not require one cross-store transaction. A committed candidate that is not referenced by an active pointer is a recoverable orphan, never an implicitly active version.
- Candidate creation and activation are idempotent. Ownership and fencing prevent a duplicated, superseded, or stale worker from publishing.
- Restart reconciliation classifies every candidate as active, retained for a protected generation, retryable, or eligible for cleanup; it never infers activation from catalog recency.
- Runtime publication cannot fail after durable activation commits.
- Runtime publication installs all target pointers while request acquisition is excluded from the cutover.
- Closing retired runtimes, releasing leases, deleting files, and other cleanup happen after publication and cannot change a successful activation into a failed activation.
- If cleanup fails, the new deployment remains active and the failure is retried and surfaced operationally.
- Requests resolve one serving generation once and hold its runtime lease until work completes.
- A deployment response always reflects the durable state of the deployment.

Serving-state lifecycle:

```text
pending -> uploaded -> validated -> active -> draining -> delete_scheduled -> deleted
                         \-> failed
```

Rollback is a new deployment of a previously validated immutable release or serving artifact. It does not mutate historical artifacts.

## Source And Connector Boundaries

Source and connection support crosses authored contracts, security, managed data, and execution.

- `analytics/model` owns authored source and connection contracts.
- `analytics/connectors` owns supported connection/source kinds, formats, option schemas, and capability metadata.
- `manageddata` owns uploaded data, revisions, bindings, and retained physical source material.
- Credential and environment resolution belongs to infrastructure adapters.
- Path-scope and object-scope validation occurs before execution.
- DuckDB scan, secret, attach, and extension statements live only in `analytics/duckdb`.

Authored YAML describes what source to read and which governed connection to use. It never exposes DuckDB secret plumbing, internal relations, or scan implementation details.

Connector execution rules:

- Every connector declares capabilities and supported credential modes.
- Resolved credentials are short-lived, scoped, and never persisted in compiled asset payloads.
- Source access is constrained to declared path or object scope.
- Connector calls honor context cancellation, timeouts, and bounded retries.
- Errors returned to users are safe projections that do not leak credentials or infrastructure internals.
- Maintained DuckDB connectors and temporary secrets may acquire declared sources only inside an admitted refresh session. Serving, dashboard, API, agent, and Data Explorer queries never resolve source credentials or attach remote systems.
- Each refresh pins one DuckDB connection, stages non-managed sources into connection-local temporary tables, validates their schema, and removes attachments and secrets before DuckLake materialization. Existing node memory, temporary-storage, thread, workload, and refresh-deadline limits bound this work; no second staging store exists.
- The node installs and loads only LeapView's fixed official signed extension allowlist. Automatic installation, automatic loading, unsigned extensions, authored extension names, and custom repositories remain disabled.
- Managed data remains an immutable verified input beneath its approved runtime root. Source exploration exposes metadata only; row exploration begins at materialized model tables and semantic models.
- Source acquisition may overlap when credential scopes do not conflict. The short DuckLake transaction and commit phase is deliberately single-writer, retries only transient catalog conflicts from the already-staged data, and identifies its result through unique commit metadata.
- Infrastructure adapters resolve credential references only after refresh admission. Resolved values are never compiled, persisted, cached, logged, or returned to users. A cleanup failure that could leave credentials or attachments resident makes the analytical environment unhealthy and prevents candidate activation.

## Storage Ownership

SQLite is the node-local control-plane store. A DuckDB-backed DuckLake catalog and Parquet form the node-local analytical data plane.

- SQLite stores workspaces, releases, deployments, serving states, immutable asset graph projections, principals, roles, sessions, managed-data metadata, refresh jobs, agent conversations, idempotency records, leases, and audit data.
- DuckLake stores analytical table metadata, snapshots, statistics, schema evolution, and physical-file ownership.
- Parquet stores materialized analytical data.
- One process-owned DuckDB instance is the node's only analytical engine and the sole client of its DuckDB-backed DuckLake catalog. Bounded connections execute serving queries and refresh transactions.
- One logical analytical operation holds one physical DuckDB connection. Nested work reuses that connection; conflicting or unadmitted acquisition fails without creating another queue.
- Every serving query qualifies all physical relations with its runtime generation's DuckLake snapshot. Old and new generations may execute concurrently while snapshot leases protect their files.
- Runtime caches are disposable projections and never authoritative state.
- Asset tables are indexed read models of compiled code assets, not authoring storage.
- Message brokers, streams, and key-value transports are never authoritative node-local product storage. They may deliver notifications or external integration messages; SQLite remains the durable coordinator.

SQLite rules:

- Transactions are short and bounded.
- WAL mode permits concurrent reads while preserving SQLite's single-writer model.
- Long-running analytical or network work never executes inside a SQLite transaction.
- Busy handling, checkpointing, integrity checks, backup, restore, and retention are explicit operational behavior.
- Repositories expose capability types, not `database/sql`, sqlc rows, or raw queries.
- Capability adapters own capability-private generated query packages.
- `platform` owns database opening, migrations, and node-level maintenance primitives, not business workflows.

Capability persistence shape:

```text
access/sqlite/internal/db
workspace/sqlite/internal/db
manageddata/sqlite/internal/db
release/sqlite/internal/db
deployment/sqlite/internal/db
servingstate/sqlite/internal/db
refresh/sqlite/internal/db
agent/sqlite/internal/db
```

Migrations remain globally ordered because the node has one SQLite database. Query APIs and row mappings remain private to the capability that owns the tables.

When a workflow needs atomic writes spanning capabilities, its coordinating use case defines a unit-of-work port. The SQLite adapter implements that port without exposing `*sql.Tx` or capability-private generated clients.

## Domain And Use Cases

Domain code defines capability language:

- business types
- value objects
- statuses and state transitions
- validation rules
- business errors
- shared business-shaped ports

Domain and use-case packages never contain:

- `http.Request` or `http.ResponseWriter`
- chi, Datastar, or gomponents details
- sqlc row types
- `sql.NullString`
- DuckDB connection details
- OpenAI request or response payloads
- filesystem layout assumptions

Use-case services orchestrate one workflow. They may load domain objects, call domain methods, coordinate repositories, invoke ports, and return capability-level results. They never decode HTTP, render HTML, emit Datastar patches, return sqlc structs, or construct infrastructure clients.

## Ports And Interfaces

Prefer small interfaces defined where they are consumed.

Use-case-specific dependency:

```go
package activate

type Repository interface {
    Deployment(ctx context.Context, id deployment.ID) (deployment.Deployment, error)
    Activate(ctx context.Context, id deployment.ID) error
}
```

Shared business concepts live at the capability root:

```text
servingstate.State
servingstate.Status
servingstate.Artifact
deployment.Deployment
release.Release
```

Interface ownership:

- Shared business language lives in the owning capability.
- Single-use dependencies live beside the consuming use case.
- Adapter implementation details stay inside adapters.
- Adapters implement ports; they do not define business-facing ports for their consumers.
- Broad interfaces split when consumers require different subsets or lifecycle guarantees.
- Runtime access is lease-based. No API returns an unleased runtime that can be closed while a caller still uses it.

Avoid generic infrastructure interfaces in domain or use-case packages:

```go
type Store interface {
    Queries() *db.Queries
}
```

## Product Interfaces

LeapView has peer product interfaces:

```text
REST API / APIGen
CLI
built-in agent and MCP tools
UI / HTML / Datastar
```

None owns product behavior. Each translates its transport contract into capability use cases.

Rules:

- TypeSpec/APIGen owns the canonical headless REST contract and generator metadata.
- API DTOs live in `internal/api` as framework-neutral wire contracts only.
- CLI commands call capability use cases or generated API operations; they do not implement parallel business workflows.
- The built-in agent and MCP consume one governed tool catalog derived from product-operation metadata, with shared risk, permission, workspace, credential, execution, projection, audit, and error behavior.
- UI routes call the same capability use cases as API, CLI, and agent interfaces for equivalent behavior.
- Datastar signal shapes are UI-private adapter contracts and never become headless API DTOs.
- A versioned, paginated asset catalog API exposes node-local discovery without exposing storage internals.

Avoid a single cross-capability `internal/api/http` package.

## UI And Datastar

HTTP handlers are adapters. They parse route parameters, query strings, forms, JSON bodies, and Datastar signals; call one use case; translate results; and map errors to status codes.

Handlers do not own business workflows such as deployment activation, workspace access mutation, artifact validation, or dashboard query orchestration.

Datastar-specific logic lives near the owning capability:

- signal decoding
- patch keys
- SSE serialization
- client signal compatibility

Domain and analytics packages speak in typed commands, snapshots, events, query intents, and results.

Gomponents renderers are edge adapters. Shared `internal/ui` contains only design-system primitives, document shells, and generated signal contracts. Capability-specific pages live under capability `ui` packages.

Shared UI code performs no workflow orchestration, storage access, semantic planning, cross-contract validation, or domain mutation.

## Dashboard Runtime

Dashboard owns report-page behavior:

- `PageState`
- `PageSnapshot`
- `FilterState`
- `InteractionSelection`
- table-window command intents
- chart-selection command intents
- typed analytics query intents

Dashboard streaming services:

- accept `context.Context` and stop promptly on cancellation
- treat repeated requests and stale client updates as safe to ignore or replace
- produce immutable page snapshots or typed patch intents
- make cache invalidation and refresh behavior explicit
- treat Datastar as serialization and transport, not business state
- use bounded mailboxes with explicit coalescing and overflow behavior

Dashboard describes what data a page needs. Analytics owns semantic query planning and execution. Dashboard queries analytics through typed semantic query ports.

Visual renderer plugins adapt renderer-neutral visual intent to concrete libraries such as ECharts. Renderer plugins never own semantic query planning, dashboard filter behavior, or backend data contracts.

Page streams are node-local. Reconnection reconstructs canonical state from the active runtime and server-owned page state; correctness never depends on replaying an in-memory stream history.

## Workload Admission

`workload` is the node-wide capacity gate for `interactive`, `background`, `refresh`, `control`, and `maintenance` work. Admission is non-preemptive and hierarchical:

1. queued classes below reserved running capacity receive the next permits;
2. borrowable capacity is round-robin across eligible classes;
3. workspaces are round-robin within a class, with FIFO preserved per workspace;
4. idle reservations may be borrowed up to class maxima, but new borrowing stops when reserved demand appears.

Interactive, background, and refresh work require a fairness identity. It is the workspace ID except for deliberately cross-workspace agent runs, which use the bounded internal `_global` identity. Control and maintenance are node-scoped. Node, class, and per-identity queues are bounded; maintenance never queues and skips a pass when saturated. Queue and execution deadlines are distinct and observable.

Nested work reuses a permit only for the same controller, class, and workspace. Conflicting nested admission fails explicitly. Durable queues remain the source of truth: workers inspect at most one head per workspace, obtain admission, then atomically claim the exact durable ID. Losing that claim or admission never marks work failed.

Admission provides scheduling fairness and a node-wide concurrency bound, not hard per-workspace CPU or DuckDB intermediate-memory partitions. A workspace may consume idle capacity, and one admitted query may use much of the configured DuckDB memory envelope. Result and cache retention remain workspace-bounded. Workloads requiring hard CPU, memory, or failure isolation run in separate LeapView deployments or OS/container resource domains.

## Analytics Runtime

`analytics` owns:

- semantic model validation
- source and connection resolution contracts
- relationship validation
- semantic query planning
- path safety
- SQL plan generation
- DuckDB execution adapters
- materialization behavior
- query result normalization

DuckDB runtime construction belongs in analytics adapters and process composition. Workspace, dashboard, serving state, API, CLI, and agent code use typed analytics ports rather than constructing DuckDB runtimes directly.

The node owns one DuckDB `DatabaseInstance` with multiple bounded client connections. Serving and refresh share its scheduler, buffer manager, catalog visibility, extension set, access policy, memory limit, temporary-storage limit, and thread limit. Runtime generations own compiled plans, snapshot IDs, and cache scopes—not engines or connections.

Execution rules:

- Every query has queue and execution deadlines.
- Request cancellation interrupts queued and running work.
- Result row, byte, and cardinality limits are enforced before serialization.
- Arrow is the canonical in-process representation for governed analytical result batches through result limiting, coalescing, and cache retention. Every consumer owns a lease over immutable Arrow buffers; eviction, cancellation, and runtime retirement release only their own references. Result and cache budgets conservatively charge retained buffers and schema metadata, and transient capture bytes are observable. JSON, dashboard, agent, CLI, and Data Explorer adapters convert at their domain boundary, and control-plane contracts never depend on Arrow.
- Arrow API responses use the versioned `native-v1` contract: governed DuckDB record batches stream directly from the pinned connection with physical Arrow scalar types and true nulls. They never detour through `database/sql` row scans, Go maps, all-string projections, or whole-response buffering. JSON responses retain their independent JSON contract.
- Node-wide limits bound DuckDB memory, temporary storage, threads, connection concurrency, result size, and retained cache data. Container or cgroup limits provide the hard process envelope.
- Physical queries are admitted after governance and before DuckDB execution; one logical bundle or nested physical call holds one permit.
- A logical analytical operation acquires one connection after workload admission and retains it through its complete query bundle or refresh transaction. Nested execution reuses that connection. The connection pool has no queue or independent admission policy; valid workload sizing guarantees an admitted operation can acquire its required connection.
- Dashboard, REST, CLI, and data-explorer reads are interactive; agent and nested agent-tool queries are background.
- Query caches have per-runtime, per-workspace, and node-wide memory budgets.
- Cache keys include every security, policy, serving-generation, source-revision, and query input that affects results.
- Request cancellation interrupts its DuckDB connection. Resource or query failure must not invalidate unrelated connections. A fatal instance failure stops new admission, fails readiness, deterministically drains or cancels in-flight work, and terminates the process for external-supervisor restart; the instance is never hot-replaced underneath active generations.

DuckLake writes may run concurrently only when their changesets cannot violate product invariants. Coordination is node-local and keyed by workspace and affected table set; transaction conflict handling is bounded and observable. There is no unconditional node-global writer mutex.

## Refresh Runtime

`refresh` owns durable analytical refresh workflows:

- authored schedules and manual triggers
- dependency-aware table plans
- job claims and renewable leases
- refresh generations and supersession
- materialization orchestration
- candidate snapshot validation
- data-version activation
- failure and cancellation state

Refresh invariants:

- Work is restart-aware, idempotent, attributable, and bounded.
- A job lease includes an owner and fencing generation; a stale worker cannot publish after losing its lease.
- A later refresh generation supersedes earlier unfinished generations.
- Failed or canceled refreshes never replace active data.
- Materialization writes isolated candidate state before activation.
- The active data version changes through the same prepared-publication discipline as deployments.
- Scheduled, API, CLI, UI, and agent triggers converge on the same use case.

## Runtime Host

`runtimehost` owns process-local executable generation lifecycle:

- prepare candidate runtimes without exposing them
- track one active runtime per workspace and environment
- acquire and release typed runtime leases
- publish prepared runtimes atomically after durable activation
- drain and close replaced runtimes safely
- expose lease-backed runtime ports to dashboard, agent, refresh, and API use cases

Boundary rules:

- Runtime host never owns release, deployment, serving-state, or refresh status transitions.
- Runtime host never plans semantic queries or constructs dashboard patches.
- Runtime host never calls sqlc or raw SQL.
- Persistent snapshot leases are repository ports supplied by serving state.
- Retention expires snapshots and deletes their files only after proving that no serving generation lease protects them.
- Runtime generations never own or close the process DuckDB instance or its connections. Retirement releases snapshot protection and generation-scoped cache state.
- Losing a required persistent lease makes the affected runtime unavailable for new work until protection is re-established.
- Cleanup errors are retryable maintenance failures, not activation rollback signals.

## Composition Root

`internal/app` is composition only.

It may:

- load configuration
- open infrastructure adapters
- construct capability repositories, services, handlers, and workers
- register routes
- manage process lifecycle, logging, shutdown, health checks, and shared middleware
- mount generated APIGen routing and delegate operations to capability adapters

It never:

- owns business workflows
- contains capability DTO mapping
- contains domain validation
- calls sqlc or raw SQL directly
- owns REST, CLI, agent, or UI behavior
- stores `*platform.Store` for lazy repository construction
- passes `*app.Server`, `*platform.Store`, or broad runtime objects into capability adapters

Capability-local composition adapters expose narrow module surfaces:

```text
workspace/module.Module
access/module.Module
dashboard/module.Module
agent/module.Module
release/module.Module
deployment/module.Module
manageddata/module.Module
refresh/module.Module
```

A module package may import its capability's adapters and use cases. It may expose route mounting, worker lifecycle, and explicitly named ports. Modules do not expose their internal dependency container and are treated as composition packages by architecture guardrails.

Target route ownership:

```text
internal/app
  -> workspace/http
  -> access/http
  -> release/http
  -> deployment/http
  -> manageddata/http
  -> analytics/query/http
  -> analytics/materialize/http
  -> dashboard/http
  -> agent/http
  -> admin/http
```

The CLI is a transport and process entrypoint. Server assembly is defined once and reused by CLI startup rather than duplicated across `internal/cli` and `internal/app`.

## Reliability And Consistency

Every durable workflow documents:

- source of truth
- transaction boundary
- idempotency key
- state machine
- retry policy
- timeout and cancellation behavior
- crash recovery behavior
- supersession or fencing rule
- audit record
- cleanup ownership

Use a transactional outbox only when a durable state change must trigger asynchronous work. In-process domain events are not used to hide required synchronous consistency.

Exactly-once delivery is not assumed. Consumers are idempotent, sequence-aware where ordering matters, and safe under duplicate execution.

Background workers:

- inspect durable heads, obtain workload admission, then atomically claim the exact ID
- present at most one candidate per workspace to each workload class
- renew leases while working
- stop publication after lease loss
- never hold SQLite transactions during analytical or network work
- recover abandoned work after process restart
- expose queue depth, age, attempts, lease state, and terminal outcome

## Independent Deployments

Multiple LeapView deployments are independent data-product nodes. Each owns its workspaces, authorization, asset catalog, serving state, and analytical data.

LeapView provides stable seams for external integrations:

- durable, globally unique node identity
- stable workspace and asset identifiers within the node, globally addressed as opaque node-and-object identities
- versioned asset graph projections
- versioned, paginated catalog APIs
- artifact and serving-generation digests
- explicit ownership and lineage metadata

## Possible Future Products — Not Current Architecture

The products below are possible future directions, not part of the current target architecture or roadmap. No current package, abstraction, protocol, remote lease, or cross-node coordination mechanism is required solely to support them. Adoption requires an explicit product decision, an architecture decision record, and a revision of this specification.

### Possible Future: Federated Discovery Catalog

A federated discovery catalog could index published metadata from independent nodes, but LeapView would not depend on it for local operation.

The external federation boundary obeys these rules:

- A federated catalog is discovery metadata, never the authoring source for node assets.
- Node-local authorization remains authoritative.
- A federated catalog does not own node serving pointers, query leases, refresh state, dashboard sessions, or analytical files.
- Discovery-catalog unavailability does not stop a node from serving known workspaces and assets.
- Cross-node publication is asynchronous and cannot participate in local deployment activation.
- Cross-node analytical queries and transactions are outside LeapView's product boundary.
- Independent nodes never share writable SQLite files, DuckLake catalogs, runtime directories, or in-memory coordination.

### Possible Future: Shared Analytical Catalog

A shared analytical catalog could centralize DuckLake metadata and analytical files while preserving node-local DuckDB compute and application state. Quack is the leading candidate, not a selected production dependency. The current architecture preserves immutable snapshot identities and coarse capability boundaries but does not prebuild a generic remote-catalog abstraction.

If this product is pursued, its design must prove:

- One catalog service exclusively owns its catalog database file; clients never mount or open it directly.
- Opaque namespaces and least-privilege credentials isolate nodes, workspaces, and object-storage paths.
- Catalog-enforced write fencing and snapshot protection prevent stale publication and premature retention across clients.
- Remote commits create immutable candidates; node-local SQLite activation selects an exact snapshot, and unreferenced commits remain recoverable orphans rather than becoming implicitly active.
- The shared catalog and analytical files form an explicit consistency, availability, security, backup, upgrade, and recovery domain. Initial operation may use one vertically scaled catalog server without HA.
- The production protocol passes failure-injection tests for atomic commit and rollback, fencing, disconnect recovery, restart convergence, snapshot ordering and protection, bounded retry, authorization, backup and restore, and client/server compatibility.

## Package Splitting Rules

Split when cohesion breaks:

- a file mixes unrelated workflows
- tests for one behavior need unrelated setup
- a service has methods with different dependency sets
- a package imports several unrelated external systems
- one product change risks accidental edits in another capability
- domain language diverges
- a package becomes a de facto dependency container

Split by use case before generic layer:

```text
deployment/activate
servingstate/validate
release/finalize
refresh/run
```

Prefer this over:

```text
deployment/services
```

Create adapter subpackages when code imports or exposes:

- sqlc generated packages
- `database/sql`
- DuckDB-specific SQL or runtime details
- `net/http`
- chi
- Datastar SSE or signal machinery
- filesystem layout
- object-storage SDKs
- model-provider API payloads

Line count is only a signal. Cohesion, dependency direction, ownership, and test friction determine package boundaries.

## Naming Rules

Prefer capability names:

```text
project
workspace
access
manageddata
analytics
dashboard
agent
release
deployment
servingstate
refresh
runtimehost
platform
```

Prefer use-case names:

```text
compile
finalize
activate
validate
materialize
stream
grant
revoke
refresh
```

Prefer adapter names:

```text
http
sqlite
duckdb
filesystem
s3
datastar
openai
```

Avoid package names that describe a generic layer unless scoped inside a capability:

```text
handlers
services
repositories
models
utils
helpers
```

## Architecture Guardrails

The architecture is enforced with package-boundary and contract tests.

Guardrails include:

- A declarative list of top-level capabilities and their allowed dependency edges.
- No reciprocal capability dependencies.
- Domain and use-case packages cannot import adapter packages.
- Domain and use-case packages cannot import `net/http`, chi, Datastar, gomponents, sqlc generated packages, DuckDB adapters, filesystem adapters, object-storage adapters, or model-provider adapters.
- Capabilities import another capability only through its declared public contract packages.
- Capability-private `internal` packages prevent unauthorized imports where Go visibility rules can enforce the boundary.
- `internal/api` remains transport-contract only.
- shared `internal/ui` remains render-only and capability-neutral.
- `platform.Store`, `database/sql`, and sqlc types do not leak into handlers, use cases, runtime managers, or domain packages.
- SQL query generation is capability-private even though migrations are globally ordered.
- Compiled artifact types are immutable by construction.
- Runtime access is always lease-backed.
- Exactly one process-owned DuckDB instance is constructed through composition and analytical adapters; product capabilities never construct or replace it.
- Every physical relation in a serving plan is qualified by the runtime's exact snapshot identity, including relations reached through joins, subqueries, views, and generated plans.
- Activation tests inject cleanup failures and prove durable and in-memory state remain consistent.
- Restart tests prove abandoned jobs recover safely and stale workers cannot publish.
- Capacity tests prove the node resource envelope and workload fairness at the maximum supported node size; they do not claim hard per-workspace DuckDB memory isolation.

String matching for known function names is not a sufficient architecture boundary. Import classification, capability dependency matrices, Go visibility, contract tests, and failure-injection tests enforce structural and behavioral invariants.

## Success Criteria

The architecture succeeds when:

- A developer can understand and test one capability without loading the whole application into their head.
- A capability's business behavior is reusable from HTTP, CLI, UI, and agent interfaces without duplication.
- A deployment either becomes durably and visibly active for every target or remains inactive.
- Every request executes against one immutable serving generation and protected analytical snapshot.
- One workspace cannot starve queued work from other workspaces, and aggregate analytical work remains inside the documented node envelope.
- A node reaches a documented, load-tested capacity envelope and fails predictably beyond it.
- Independent LeapView deployments operate without shared runtime or transactional state.
- Published node metadata remains versioned and independently consumable without becoming a dependency of local serving.
- External transports and providers can change behind adapters without changing domain behavior; node-local capability collaboration remains direct.
