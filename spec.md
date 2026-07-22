# LeapView Architecture Spec

This document defines the target architecture for LeapView: a feature-oriented modular monolith with ports and adapters. The goal is cohesive product capabilities, explicit infrastructure boundaries, and a codebase that grows without turning `internal/app`, `platform`, or global service objects into new monoliths.

## Core Rules

- Package by product capability first.
- Keep domain and use-case code free of transport, persistence, filesystem, DuckDB, Datastar, gomponents, and model-provider details.
- Define small ports at the consumer boundary.
- Let adapters import external systems and generated code; never let domain or use-case packages import adapters.
- Split packages by cohesion, workflow, dependency pressure, or test friction, not by generic layers.
- Prefer explicit composition over hidden service locators or broad runtime objects.

Dependencies point inward:

```text
HTTP / CLI / Datastar / SQLite / DuckDB / filesystem / OpenAI
        -> capability adapters
        -> capability use cases
        -> capability domain types and ports
```

Allowed:

```text
servingstate/http       -> servingstate/activate
servingstate/http       -> servingstate/filesystem
servingstate/sqlite     -> servingstate
dashboard/http        -> dashboard/stream
dashboard/datastar    -> dashboard
analytics/duckdb      -> analytics/query
analytics/connectors  -> analytics/model
```

Forbidden outside adapters and composition:

```text
servingstate/activate -> servingstate/filesystem
servingstate          -> chi
servingstate          -> sqlc rows
dashboard/report    -> datastar
analytics/query     -> duckdb connection details
workspace           -> http.Request
agent               -> OpenAI request payloads
```

## Capability Map

Long-term package ownership:

```text
internal/
  workspace/
  servingstate/
  access/
  analytics/
  dashboard/
  agent/
  runtimehost/
  platform/
```

- `workspace`: workspace identity, catalog surface, asset discovery, asset graph views, workspace-level read models.
- `servingstate`: bundle lifecycle, validation, artifact identity, activation, rollback, serving state status, immutable asset snapshots.
- `access`: principals, groups, roles, permissions, authorization decisions, tokens, sessions, audit access.
- `analytics`: source and connection contracts, semantic model tables, semantic models, query planning, query execution, materialization, connectors, DuckDB adapters.
- `dashboard`: report pages, filters, visuals, BI tables, interactions, page state, typed query intents, signal contracts.
- `agent`: conversations, runs, transcripts, tools, policy-filtered operation exposure, model interaction ports.
- `runtimehost`: active runtime lifecycle, prepared runtime swap, runtime closure, active runtime ports.
- `platform`: low-level infrastructure: SQLite setup, migrations, shared DB plumbing, process-level storage paths.

`admin` is not a domain capability. It is an interface surface over capabilities and infrastructure. `admin/http` may aggregate read models from `access`, `workspace`, `servingstate`, `agent`, `analytics`, `runtimehost`, and `platform`, but it must not own their business workflows.

Historical or legacy vocabulary must not define long-term ownership when it conflicts with product capabilities.

## Product Contract

The authored product contract is:

```text
sources -> models -> semantic model -> dashboards
```

LeapView is assets-as-code. Authored YAML in Git is the source of truth. The compiler turns authored contracts into a normalized workspace and stable asset graph. Serving states publish immutable graph snapshots. Runtime stores never become authoring sources.

Not product/schema concepts in the v1 contract:

- metric views
- cache tables
- generated serving tables

If those appear in code, they are internal runtime implementation details, legacy rejection paths, or tests proving old vocabulary does not leak into user-facing surfaces.

`semantic dataset` is allowed only as a headless API and agent-facing alias for a semantic model table. Go domain code should prefer `model table` or `table` unless it is translating the public BI API contract. Do not introduce a separate dataset domain model parallel to `analytics/model.Table`.

YAML contract ownership:

- `workspace` owns catalog discovery and workspace asset surfacing.
- `analytics/model` owns source contracts, connection contracts, model table contracts, semantic model contracts, fields, relationships, measures, and materialization definitions.
- `dashboard/report` owns dashboard, page, filter, visual, and table contracts.
- `servingstate` owns bundle-level validation, artifact identity, activation, rollback, and artifact storage.

## Target Package Shape

Use flat capability packages until cohesion breaks. Then split by workflow or adapter.

Target examples:

```text
analytics/
  model/          semantic contracts, fields, relationships, measures
  query/          semantic query requests, planning, path safety, SQL plans
  materialize/    refresh and materialization behavior
  connectors/     connector registry, source capabilities, option schemas
  duckdb/         DuckDB execution adapter

dashboard/
  report/         dashboard/page/filter/visual/table contracts
  stream/         page snapshots and update flow
  command/        filter, selection, table-window, refresh command handling
  datastar/       signal decoding, patch keys, SSE serialization
  http/           route handlers
  ui/             HTML/gomponents rendering adapter

servingstate/
  state.go   shared domain language
  activate/       activation use case
  validate/       validation use case
  sqlite/         SQLite persistence adapter
  filesystem/     artifact storage and bundle adapter
  http/           route handlers

workspace/
  catalog/        catalog discovery and workspace identity
  compiler/       cross-contract loading, validation, normalization, graph extraction
  refresh/        workspace asset refresh use cases
  sqlite/         SQLite read models and repositories
  http/           REST/UI handlers
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

These names are acceptable only inside a capability and only when they stay narrow.

## Asset Compiler

Authored YAML contracts have one compilation boundary:

```text
workspace/catalog + analytics/model + dashboard/report
        -> workspace/compiler
        -> normalized workspace + stable asset graph
        -> immutable asset configuration snapshot
```

The compiler owns cross-contract validation and normalization:

- Catalog entries resolve to semantic models and dashboards.
- Dashboard `semantic_model` references resolve to loaded semantic models.
- Dashboard fields, measures, filters, tables, and visuals resolve against the semantic model.
- Legacy vocabulary such as metric views is rejected at the boundary.
- Runtime consumers receive a normalized workspace without re-parsing YAML.
- Serving state, UI, API, agents, and storage adapters consume the compiler-produced asset graph instead of rediscovering lineage by walking semantic or dashboard internals.

Capability packages own local contracts and validation. The compiler owns validation spanning multiple contracts.

Asset graph rules:

- Every authored object users can discover, govern, diff, or trace is an asset.
- Logical asset IDs are stable across serving states, such as `semantic_model:olist` or `visual:executive-sales.revenue`.
- Serving state-scoped snapshot IDs may change per serving state.
- Asset payloads are explicit versioned projections, such as `semantic_model.v1`, `model_table.v1`, `measure.v1`, `dashboard.v1`, and `visual.v1`.
- Persisted payloads must not be raw `json.Marshal` output of arbitrary Go structs.
- The full authored YAML remains in the serving state artifact.
- Read paths may load asset snapshots but must not repair, migrate, or reinterpret stale graph shapes during ordinary HTTP requests.

## Source And Connector Boundaries

Source and connection support crosses contracts, security, and execution.

- `analytics/model` owns authored source and connection contracts.
- `analytics/connectors` owns supported connection/source kinds, formats, option schemas, and capability metadata.
- Credential and environment resolution belongs to infrastructure adapters.
- Path-scope and object-scope validation belongs at the compiler/runtime boundary before execution.
- DuckDB scan, secret, attach, and extension statements belong in `analytics/duckdb`.

Authored YAML describes what source to read and which governed connection to use. It must not expose DuckDB secret plumbing, internal `raw.*` relations, or scan implementation details.

## Storage Ownership

- SQLite is the control-plane store for workspaces, serving states, immutable asset graph snapshots, roles, sessions, agent conversations, and audit data.
- SQLite asset tables are indexed read models of compiled code assets, not authoring storage.
- DuckDB is the analytical data plane for imported/cache data, semantic query execution, dashboard data, and materializations.
- Generated sqlc code is private to SQLite adapter packages or narrow platform infrastructure.
- `platform.Store` must not expose raw `Queries()` or direct SQL access to handlers, use cases, runtime managers, or domain packages.
- `platform` may own migrations and DB setup. It must not accumulate workspace, serving state, access, session, asset, or agent business workflows.

Capability repositories wrap control-plane persistence:

```text
servingstate.Repository
servingstate.ArtifactRepository
workspace.AssetRepository
access.RoleBindingRepository
agent.ConversationRepository
```

SQLite implementations live under adapter packages:

```text
servingstate/sqlite.Repository
workspace/sqlite.AssetRepository
access/sqlite.RoleBindingRepository
agent/sqlite.ConversationRepository
```

## Domain And Use Cases

Domain code defines capability language:

- business types
- value objects
- statuses and state transitions
- validation rules
- business errors
- shared business-shaped ports

Domain and use-case packages must not contain:

- `http.Request` or `http.ResponseWriter`
- `chi`, Datastar, or gomponents details
- sqlc row types
- `sql.NullString`
- DuckDB connection details
- OpenAI request/response payloads
- filesystem layout assumptions, unless the capability is explicitly a filesystem adapter

Use-case services orchestrate one workflow. They may load domain objects, call domain methods, coordinate repositories, call ports, and return capability-level results. They must not decode HTTP, render HTML, emit Datastar patches, return sqlc structs, or construct infrastructure clients.

When a workflow needs atomic writes across repositories, define a capability-level unit-of-work port. Do not expose `*sql.Tx` or sqlc transaction types to use-case code.

## Ports And Interfaces

Prefer small interfaces defined where they are consumed.

Use-case-specific dependency:

```go
package activate

type Repository interface {
    ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error)
    Activate(ctx context.Context, workspaceID servingstate.WorkspaceID, servingStateID servingstate.ID) error
}
```

Shared business concept:

```text
servingstate.State
servingstate.Status
servingstate.Artifact
servingstate.Repository
```

Avoid generic infrastructure interfaces in domain or use-case packages:

```go
type Store interface {
    Queries() *db.Queries
}
```

Interface ownership:

- Shared business language lives in the capability root.
- Single-use dependencies live beside the consuming use case.
- Adapter implementation details stay inside adapters.
- Adapters implement ports; they do not own business-facing port definitions.

Broad interfaces must split when consumers diverge. Dashboard streaming, semantic query APIs, workspace asset views, and refresh orchestration should not share one cross-cutting runtime interface unless they truly need the same capability set.

## Product Interfaces

LeapView has peer product interfaces:

```text
REST API / APIGen
CLI
built-in agent and MCP tools
UI / HTML / Datastar
```

None of these owns product behavior. They translate transport contracts into capability use cases.

Rules:

- TypeSpec/APIGen owns the canonical headless REST contract and generator metadata.
- API DTOs live in `internal/api` as framework-neutral wire contracts only.
- CLI commands should use generated APIGen operation metadata where possible, with small UX wrappers only when needed.
- The built-in agent and MCP should consume one governed tool catalog derived from APIGen operation metadata, with shared risk, permission, explicit workspace argument, credential, execution, projection, audit, and error behavior.
- UI routes may render HTML and Datastar patches, but must call the same capability use cases as API, CLI, and agent interfaces for the same behavior.
- Datastar signal shapes are UI-private adapter contracts. They must not become headless API DTOs.

Avoid a single cross-capability `internal/api/http` package.

## UI And Datastar

HTTP handlers are adapters. They may parse route parameters, query strings, forms, JSON bodies, and Datastar signals; call one use case; translate results; and map errors to status codes.

Handlers must not own business workflows such as serving state activation, workspace access mutation, artifact validation, or dashboard query orchestration.

Datastar-specific logic belongs in adapter packages near the owning capability:

- signal decoding
- patch keys
- SSE serialization
- compatibility with client-side signal shape

Domain and analytics packages speak in typed commands, snapshots, events, query intents, and result structs.

Gomponents renderers are edge adapters. A shared `internal/ui` package may exist, but it must stay render-only:

- no workflow orchestration
- no storage access
- no semantic query planning
- no cross-contract validation
- no domain mutation

REST JSON handlers and UI/Datastar handlers may live beside the same capability, but their transport contracts must stay separate.

## Dashboard Runtime

Dashboard owns report-page behavior:

- `PageState`
- `PageSnapshot`
- `FilterState`
- `InteractionSelection`
- table window command intents
- chart selection command intents
- typed analytics query intents

Dashboard streaming services must:

- accept `context.Context` and stop promptly on cancellation
- treat repeated requests and stale client updates as safe to ignore or replace
- produce immutable page snapshots or typed patch intents
- keep cache invalidation and refresh behavior explicit
- treat Datastar as serialization and transport, not business state

Dashboard may describe what data a page needs. Analytics owns semantic query planning and execution. Dashboard queries analytics through typed semantic query ports.

Visual renderer plugins adapt renderer-neutral visual intent to concrete libraries such as ECharts. Renderer plugins must not own semantic query planning, dashboard filter behavior, or backend data contracts.

## Analytics Runtime

`analytics` owns:

- semantic model validation
- source and connection resolution
- relationship validation
- semantic query planning
- path safety
- SQL plan generation
- DuckDB execution adapters
- materialization and refresh behavior

DuckDB runtime construction belongs in analytics adapters. Workspace, dashboard, serving state, API, CLI, and agent code use typed analytics ports rather than constructing DuckDB runtimes directly.

## Serving State And Runtime Host

Serving state owns published workspace artifacts:

- bundle envelope and manifest
- artifact identity and digest
- serving state status transitions
- upload, validation, activation, rollback, and failure marking
- persistence of compiler-produced asset snapshots

Serving state depends on ports for:

- workspace compilation
- artifact storage
- runtime preparation
- artifact persistence
- access policy reconciliation

Serving state must not walk semantic/dashboard internals directly, construct DuckDB services, or call sqlc queries directly.

`runtimehost` owns active runtime lifecycle:

- track the active serving state/runtime for each workspace
- prepare candidate runtimes before activation commits
- atomically swap the active runtime after activation succeeds
- close replaced runtimes safely
- expose typed runtime ports to dashboard and agent use cases

Boundary rules:

- Serving state requests activation through a runtime host port.
- Analytics prepares executable engines and query/materialization services.
- Dashboard and agent use active runtime ports.
- Runtime host must not own serving state status transitions, semantic query planning, dashboard patch construction, or sqlc persistence.

## Composition Root

`internal/app` is composition only.

It may:

- load configuration
- open infrastructure adapters
- construct repositories, services, handlers, and background workers
- register routes
- manage lifecycle, logging, shutdown, health checks, and shared middleware
- wire adapters into use cases
- mount generated APIGen routing and delegate operations to capability-owned adapters

It must not:

- own business workflows
- contain capability DTO mapping
- contain domain validation
- call sqlc or raw SQL directly
- become the long-term owner of REST, CLI, agent, or UI adapters
- pass `*app.Server`, `*platform.Store`, or broad runtime objects into capability adapters when narrow ports are enough

Target route ownership:

```text
internal/app
  -> workspace/http
  -> access/http
  -> servingstate/http
  -> analytics/query/http
  -> dashboard/http
  -> agent/http
  -> admin/http
```

## Package Splitting Rules

Split when cohesion breaks:

- a file mixes unrelated workflows
- tests for one behavior need unrelated setup
- a service has methods with different dependency sets
- a package imports several unrelated external systems
- one product change risks accidental edits in another
- domain language diverges

Split by use case before generic layer:

```text
servingstate/activate
servingstate/validate
servingstate/upload
```

Prefer this over:

```text
servingstate/services
```

Create adapter subpackages when code imports or exposes:

- sqlc generated packages
- `database/sql`
- DuckDB-specific SQL/runtime details
- `net/http`
- `chi`
- Datastar SSE/signal machinery
- filesystem artifact layout
- model-provider API payloads

Line count is only a hint. Cohesion, dependency direction, and test friction are the real split criteria.

## Naming Rules

Prefer capability names:

```text
servingstate
workspace
access
analytics
dashboard
agent
runtimehost
platform
```

Prefer use-case names:

```text
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

The target architecture should be enforceable with package boundary tests.

Use-case packages must not import adapter packages such as:

```text
/sqlite
/filesystem
/duckdb
/datastar
/http
/openai
```

Exceptions are allowed only when the package is itself an adapter or composition package.

Architecture tests should assert:

- use-case packages do not import adapter packages
- domain and use-case packages do not import `net/http`, `chi`, Datastar, gomponents, sqlc generated packages, DuckDB adapters, filesystem adapters, or model-provider adapters
- `internal/api` remains transport-contract only
- `internal/ui` remains render-only
- `platform.Store` and sqlc generated types do not leak into handlers, use cases, runtime managers, or domain packages

The architecture is successful when a developer can understand and test one capability without loading the whole application into their head.
