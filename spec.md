# LibreDash Architecture Spec

This document describes the target architecture for LibreDash as it grows from a compact monolith into a modular Go application. The goal is not to add ceremony. The goal is to keep business capabilities cohesive, keep adapters honest, and avoid global `service.go` files becoming the new monolith.

## Architecture Style

LibreDash should evolve toward a feature-oriented modular monolith with hexagonal boundaries at the edges.

In practical Go terms:

- Package by business capability first.
- Keep each capability cohesive and understandable.
- Use ports and adapters where the capability talks to the outside world.
- Define small interfaces at the consumer boundary.
- Keep domain and application code free of transport and persistence details.
- Split into subpackages only when cohesion starts to break.

This is sometimes called:

- Modular monolith
- Feature-based architecture
- Vertical slice architecture
- Hexagonal architecture / ports and adapters
- Clean architecture, applied locally rather than globally

For LibreDash, the preferred label is:

> Feature-oriented modular monolith with ports and adapters.

## Target Dependency Direction

Dependencies should point inward:

```text
HTTP / Datastar / SQLite / DuckDB / filesystem / OpenAI
        -> capability application code
        -> capability domain types and ports
```

Business code should not import transport or persistence implementation packages.

Package import rules:

- Capability root packages contain shared domain language.
- Use-case packages may import the capability root package.
- Adapter packages may import the capability root package and use-case packages.
- Capability root packages and use-case packages must not import adapter packages.
- Composition code is the only place that wires adapters into use cases.

Allowed inward dependencies:

```text
internal/deployment/http      -> internal/deployment
internal/deployment/http      -> internal/deployment/activate
internal/deployment/sqlite    -> internal/deployment
internal/deployment/filesystem -> internal/deployment

internal/dashboard/datastar   -> internal/dashboard
internal/analytics/duckdb     -> internal/analytics
```

Avoid outward dependencies:

```text
internal/deployment -> chi
internal/deployment -> sqlc generated rows
internal/deployment -> datastar
internal/deployment -> http.Request
```

## Top-Level Capabilities

The long-term internal package map should be organized around product capabilities:

```text
internal/
  workspace/
  deployment/
  access/
  analytics/
  dashboard/
  agent/
```

Suggested ownership:

- `workspace`: workspace identity, catalog surface, asset discovery, workspace-level views.
- `deployment`: bundle lifecycle, upload, validation, artifact storage, activation, rollback.
- `access`: principals, roles, permissions, authorization decisions.
- `analytics`: semantic model loading, source/model resolution, semantic relationship validation, query planning, DuckDB execution, materialization.
- `dashboard`: report pages, filters, visuals, BI tables, interaction commands, page state, and typed query intents for analytics.
- `agent`: conversations, runs, tools, transcripts, model interaction.

Existing packages such as `semantic`, `query`, `dashboard`, and `data` already contain useful concepts. Refactors should preserve good domain language while moving responsibilities toward the capability map above.

Product contract:

```text
sources -> models -> semantic model -> dashboards
```

Metric views, datasets, cache tables, and generated serving tables are not product/schema concepts in the v1 contract. If they appear in code, they should be internal runtime implementation details, legacy rejection paths, or tests proving old vocabulary does not leak into user-facing surfaces.

YAML contract ownership:

- `workspace` owns catalog discovery and workspace asset surfacing.
- `analytics` owns source contracts, model table contracts, semantic model contracts, fields, relationships, measures, query-facing validation, and materialization definitions.
- `dashboard` owns dashboard/page/filter/visual/table contracts and runtime signal shapes.
- `deployment` owns bundle-level validation, artifact identity, activation, rollback, and artifact storage.

Storage ownership:

- SQLite is the control-plane store for application metadata such as workspaces, deployments, assets, roles, sessions, and agent conversations.
- DuckDB is the analytical engine for imported/cache data, semantic query execution, dashboard data, and materializations.

Capability sub-boundaries should make the semantic-model-first core explicit:

```text
analytics/
  model/        semantic model contracts, fields, relationships, measures
  query/        semantic query requests, planning, path safety, SQL plans
  materialize/  refresh and cache/materialization behavior
  duckdb/       DuckDB execution adapter

dashboard/
  report/       dashboard/page/filter/visual/table contracts
  stream/       page snapshots and SSE/update flow
  command/      filter, selection, table-window, and refresh command handling
  datastar/     signal decoding, patch keys, SSE serialization
  http/         route handlers

workspace/
  catalog/      catalog discovery and workspace identity
  compiler/     cross-contract loading, normalization, and validation
```

`analytics` is the only owner of semantic query planning, semantic model validation, materialization, and DuckDB execution. `dashboard` may describe what data a page needs, but it should call analytics through typed semantic query ports instead of planning or executing semantic queries itself.

## Workspace Compiler

LibreDash is dashboards-as-code, so authored YAML contracts need a clear compilation boundary.

The long-term target is:

```text
workspace/catalog + analytics/model + dashboard/report
        -> workspace/compiler
        -> normalized runtime workspace
```

The compiler owns cross-contract validation and normalization:

- Catalog entries resolve to semantic models and dashboards.
- Dashboard `semantic_model` references resolve to loaded semantic models.
- Dashboard fields, measures, filters, tables, and visuals resolve against the semantic model.
- Legacy product vocabulary such as metric views is rejected at the boundary.
- The compiler produces a runtime workspace that dashboard, analytics, deployment, workspace UI, and agent tools can consume without re-parsing YAML.

Capability packages own their local contracts and validation. The compiler owns validation that spans multiple contracts. Deployment validation should call the compiler instead of importing semantic/dashboard internals directly.

The compiler also owns workspace lineage in workspace terms. Deployment can persist a versioned asset graph produced by the compiler, the control-plane store can store it, and workspace UI/API can present it. Deployment should not build lineage by walking semantic/dashboard internals itself.

## Source and Connector Boundaries

Source and connection support crosses product contracts, security, and runtime execution, so the boundary must stay explicit:

- `analytics/model` owns authored source and connection contracts.
- A connector registry owns supported connection/source kinds, formats, option schemas, and capability metadata.
- Credential and environment resolution belongs to infrastructure adapters, not authored domain structs.
- Path-scope and object-scope validation belongs at the compiler/runtime boundary before execution.
- DuckDB scan, secret, attach, and extension statements belong in `analytics/duckdb`.

Authored YAML should describe what source to read and under which governed connection. It should not expose DuckDB secret plumbing, internal `raw.*` relations, or runtime scan implementation details.

## Package Shape

Start with a flat capability package:

```text
internal/deployment/
  deployment.go
  activate.go
  validate.go
  repository.go
  errors.go
```

Use clear files before creating subpackages. A file should usually represent one concept or use case, not a generic layer.

When the package grows, split by workflow or adapter. Do this because dependencies, tests, or workflows diverge, not because every use case needs a subpackage on day one:

```text
internal/deployment/
  deployment.go
  repository.go
  errors.go
  activate/
    service.go
    planner.go
  validate/
    service.go
    manifest.go
  sqlite/
    repository.go
  filesystem/
    artifact_store.go
  http/
    handlers.go
```

This keeps the default Go experience simple while still giving large areas room to breathe. A small capability can keep `deployment.Activate` or `deployment.Activator` in the root package until there is real pressure to move to `deployment/activate`.

## Domain Code

Domain code defines the language of a capability:

- Business types
- Value objects
- Statuses and state transitions
- Validation rules
- Business errors
- Business-shaped ports when they are part of the capability's language

Domain code should not contain:

- `http.Request` or `http.ResponseWriter`
- `chi`, Datastar, or gomponents details
- sqlc row types
- `sql.NullString`
- DuckDB connection details
- OpenAI request/response payloads
- Filesystem layout assumptions unless the capability is explicitly about filesystem storage

Example:

```go
type Deployment struct {
    ID          ID
    WorkspaceID WorkspaceID
    Status      Status
    Digest      Digest
}

func (d Deployment) CanActivate() bool {
    return d.Status == Validated || d.Status == Inactive || d.Status == Active
}
```

## Ports and Interfaces

Prefer small interfaces defined where they are consumed.

Good:

```go
type Repository interface {
    ByID(ctx context.Context, id ID) (Deployment, error)
    Save(ctx context.Context, deployment Deployment) error
}
```

Good when the use case needs a very specific view:

```go
package activate

type Repository interface {
    ByID(ctx context.Context, id deployment.ID) (deployment.Deployment, error)
    Activate(ctx context.Context, workspaceID deployment.WorkspaceID, deploymentID deployment.ID) error
}
```

Avoid generic interfaces that expose persistence mechanics:

```go
type Store interface {
    Queries() *db.Queries
}
```

Interface ownership rule:

- If the interface describes shared business language, keep it with the capability root package.
- If the interface exists only for one use case, keep it in the consuming use-case package.
- If the interface describes an adapter implementation detail, avoid exporting it from domain code.
- Adapters implement ports; they do not own the business-facing port definitions.

For example, activation-specific dependencies should live beside the activation workflow:

```text
deployment/activate.Repository
deployment/activate.RuntimeActivator
```

Shared concepts stay in the capability root:

```text
deployment.Deployment
deployment.Status
deployment.Artifact
deployment.Repository
```

## Application Services

Application services orchestrate use cases. They are not dumping grounds.

Prefer focused use-case services:

```text
deployment/activate.Service
deployment/validate.Service
access/grant.Service
dashboard/stream.Service
analytics/materialize.Service
```

Avoid one object that accumulates every workflow:

```text
deployment.Service
  Create
  Upload
  Validate
  Activate
  Rollback
  List
  Delete
  Refresh
```

A service should generally have one reason to change. If a service is changing for multiple workflows, split it.

Application services may:

- Load domain objects from repositories.
- Call domain methods.
- Coordinate transactions through repositories.
- Call adapter ports such as artifact stores, runtime activators, model clients, or query engines.
- Return capability-level results or DTOs designed for callers.

Application services should not:

- Decode HTTP requests.
- Write HTTP responses.
- Render gomponents pages.
- Emit Datastar patches directly unless the service belongs to a Datastar adapter package.
- Return sqlc generated structs.

When a use case spans multiple repositories or must make several writes atomically, define a capability-level transaction runner or unit-of-work port. Do not expose sqlc transaction types or `*sql.Tx` to use-case code.

```go
type UnitOfWork interface {
    Do(ctx context.Context, fn func(ctx context.Context, repos Repositories) error) error
}
```

## Adapters

Adapters translate between external systems and capability code.

Examples:

```text
deployment/http        HTTP request/response translation
deployment/sqlite      sqlc/SQLite persistence adapter
deployment/filesystem  artifact storage and bundle files
analytics/duckdb       DuckDB execution
dashboard/datastar     signal patch translation
agent/openai           model API adapter
```

Adapters may import external libraries and generated code. They should hide those details behind capability ports.

Gomponents renderers are also edge adapters. Prefer colocating renderers with the capability HTTP/UI adapter when they are capability-specific. A shared `internal/ui` package may exist, but it must stay render-only:

- No workflow orchestration.
- No storage access.
- No semantic query planning.
- No cross-contract validation.
- No mutation of domain state.

## Product Interfaces

LibreDash has four major product interfaces:

```text
REST API / APIGen
CLI
agent tools
UI / HTML / Datastar
        -> capability use cases
        -> capability domain types and ports
```

These interfaces are peers. None of them should own product behavior.

Long-term rules:

- TypeSpec/APIGen owns the canonical headless REST contract and generator metadata.
- Friendly CLI commands should use generated APIGen operation metadata where possible, with small UX wrappers only when they improve ergonomics.
- Agent tools should be derived from APIGen operation metadata, then filtered by LibreDash policy such as risk, permission, workspace scope, and credential constraints.
- UI routes may render HTML, gomponents, and Datastar patches, but they should call the same capability use cases as API, CLI, and agent interfaces.
- Datastar signal shapes are UI-private adapter contracts. They should not become headless API DTOs.
- API DTOs live in `internal/api` only as framework-neutral wire contracts. They should not contain HTTP routing, Datastar, repositories, gomponents, or use-case orchestration.

The desired shape for a mature capability is:

```text
internal/workspace/
  search.go          capability use case / domain language
  http/              REST JSON adapter
  cli/               optional friendly CLI adapter
  agent/             optional tool adapter or policy mapping
  ui/                optional HTML adapter

internal/dashboard/
  visual_data.go
  http/
  datastar/
  agent/
  ui/

internal/analytics/query/
  service.go
  http/
  cli/
  agent/
```

Do not create every adapter subpackage upfront. Start flat inside a capability and split when workflows, dependencies, or tests diverge.

Avoid a single cross-capability `internal/api/http` package. It would become the new monolith. Prefer capability-owned adapters:

```text
workspace/http.Handler
dashboard/http.Handler
analytics/query/http.Handler
deployment/http.Handler
access/http.Handler
agent/http.Handler
```

The composition root wires these adapters together. It should not absorb their product behavior.

## Control-Plane Infrastructure

SQLite/sqlc is control-plane infrastructure, not a product capability.

Long-term rules:

- Generated sqlc code should be private to SQLite adapter packages or a narrow `platform/sqlite` infrastructure package.
- `platform.Store` should not expose raw `Queries()` to handlers, services, runtime managers, or domain code.
- `access` owns roles, permissions, and authorization decisions.
- `deployment`, `workspace`, `access`, and `agent` each get capability-shaped repositories over the SQLite control plane.
- Composition code may wire SQLite implementations into use cases, but business workflows should not depend on sqlc row types or transaction types.

`platform` can remain as low-level infrastructure, migrations, or shared SQLite setup. It should not be the place where workspace, deployment, access, sessions, assets, and agent business workflows accumulate.

## Active Runtime Host

The active workspace runtime lifecycle needs a small explicit owner. It should not be absorbed wholesale by deployment, analytics, dashboard, or composition code.

Long-term responsibilities:

- Track the active deployment/runtime for a workspace.
- Prepare a candidate runtime before activation is committed.
- Atomically swap the active runtime after deployment activation succeeds.
- Close replaced runtimes safely.
- Expose typed runtime ports to dashboard and agent use cases.

Boundary rules:

- Deployment requests activation through a runtime host port.
- Analytics prepares executable engines and query/materialization services.
- Dashboard queries through typed analytics ports exposed by the active runtime.
- Composition wires the runtime host, deployment repositories, artifact store, and analytics runtime factory.
- The runtime host must not own deployment status transitions, semantic query planning, dashboard patch construction, or sqlc persistence.

## Composition Root

`internal/app` or a future `internal/server` package should become the composition root.

The composition root may:

- Load configuration.
- Open SQLite and DuckDB-backed adapters.
- Construct repositories, services, handlers, and background workers.
- Register routes.
- Manage lifecycle, logging, shutdown, and health checks.
- Wire adapters into use cases.
- Mount generated APIGen routing and delegate operations to capability-owned adapters.

The composition root should not:

- Own business workflows.
- Contain DTO mapping that belongs to a capability.
- Contain domain validation.
- Reach around use cases by calling generated sqlc queries directly.
- Become the place where unrelated product behavior accumulates.
- Become the long-term owner of every REST, CLI, agent, and UI adapter.

For a small surface, `internal/app` may temporarily contain thin handlers. As an interface grows, move handlers and translation logic toward the owning capability adapter while keeping application bootstrapping and route mounting in `internal/app`.

## HTTP and Datastar

HTTP handlers should be thin:

- Parse route parameters, query strings, forms, JSON, and Datastar signals.
- Call one application use case.
- Translate the result into HTML, JSON, redirects, or signal patches.
- Map errors to status codes.

Handlers should not own business workflows such as deployment activation, workspace access mutation, artifact validation, or dashboard query orchestration.

Datastar-specific logic should live in adapter code near dashboard/workspace capabilities rather than leaking across domain or analytics code.

REST JSON handlers and UI/Datastar handlers may live beside the same capability, but they should keep their transport contracts separate:

- REST handlers translate API DTOs and status codes.
- UI handlers translate HTML, forms, redirects, and Datastar signals.
- Both should call the same capability use cases when they represent the same product behavior.

Dashboard domain code should own:

- `PageState`
- `PageSnapshot`
- `FilterState`
- `InteractionSelection`
- Table window and chart selection command intents
- Typed analytics query intents

`dashboard/datastar` should own:

- JSON signal decoding.
- Datastar patch keys.
- SSE serialization.
- Compatibility with client-side signal shape.

Dashboard streaming services must:

- Accept `context.Context` and stop promptly on cancellation.
- Treat repeated requests and stale client updates as safe to ignore or replace.
- Produce immutable page snapshots or typed patch intents.
- Keep cache invalidation and refresh behavior explicit.
- Treat Datastar as serialization and transport, not as business state.
- Keep patch shape ownership in dashboard/datastar adapter code.

## Visual Renderer Plugins

Dashboard/Go code owns renderer-neutral visual intent:

- Visual kind and shape.
- Encodings and semantic fields.
- Safe core visual options.
- Validated renderer-specific option bags.
- Data payloads shaped by analytics results.

Web renderer plugins adapt those shapes to concrete libraries such as ECharts. Renderer plugins should not own semantic query planning, dashboard filter behavior, or backend data contracts. Future renderers should plug into the same renderer-neutral visual shape contracts rather than creating dashboard-specific query paths.

## Repositories

Repositories should be split by capability and by aggregate/use case when needed.

Good:

```text
deployment.Repository
deployment.ArtifactRepository
access.RoleBindingRepository
agent.ConversationRepository
workspace.AssetRepository
```

SQLite implementations can live under adapter subpackages:

```text
deployment/sqlite.Repository
access/sqlite.RoleBindingRepository
agent/sqlite.ConversationRepository
```

Repository implementations may use sqlc. Domain and application code should not.

## Deployment Boundaries

Deployment owns the lifecycle of published workspace artifacts:

- Bundle envelope and manifest.
- Artifact identity and digest.
- Deployment status transitions.
- Upload, validation, activation, rollback, and failure marking.

Deployment should not walk semantic/dashboard internals directly. It should depend on ports:

- A workspace compiler/validator for contract validation.
- An asset graph extractor for workspace lineage assets.
- An analytics runtime factory for preparing a runtime from a compiled artifact.
- Capability repositories for deployment and artifact persistence.

Runtime activation should prepare analytics runtime through a port and commit deployment state through deployment repositories. It should not construct DuckDB services or call sqlc queries directly.

## Scaling Laws

Use these rules to decide when to split files, packages, services, and interfaces.

### Start Flat

Begin with a single capability package when the area is small:

```text
internal/workspace/
  workspace.go
  assets.go
  repository.go
```

Do not create subpackages just to satisfy an architecture diagram.

### Split by Cohesion

Split when a file, service, or package has multiple reasons to change.

Signals:

- A file mixes unrelated workflows.
- Tests for one behavior need large unrelated setup.
- A service has methods that do not share dependencies.
- A package import list includes several unrelated external systems.
- A change to one product area risks accidental edits in another.

### Split by Use Case Before Layer

When `deployment/service.go` grows, prefer:

```text
deployment/activate/
deployment/validate/
deployment/upload/
```

over:

```text
deployment/services/
```

Use-case packages are easier to reason about than generic layer packages.

### Split Adapters When External Details Leak

Create an adapter subpackage when code imports or exposes:

- sqlc generated packages
- `database/sql`
- DuckDB-specific SQL/runtime details
- `net/http`
- `chi`
- Datastar SSE/signal machinery
- filesystem artifact layout
- OpenAI-compatible API payloads

The adapter should translate those details into capability language.

### Split Interfaces When Consumers Diverge

Do not create one broad repository interface for everyone.

If activation and listing need different data, define different ports:

```go
type ActivationRepository interface { ... }
type ListingRepository interface { ... }
```

Small interfaces keep tests focused and prevent use cases from depending on accidental persistence capabilities.

### Split Domain Types When Language Diverges

If a package starts using the same nouns to mean different things, split or rename.

Examples:

- Dashboard asset vs deployment artifact.
- Workspace role vs provider identity.
- Query filter vs UI filter signal.

Ambiguous domain language is an architectural smell.

### Split on Test Friction

Tests should be easy to write without booting the world.

Split when:

- A use case test needs HTTP setup.
- A domain rule test needs a database.
- A repository test needs Datastar signals.
- A dashboard command test needs OpenAI config.

The target is that domain and use-case tests run with plain Go fakes.

### Do Not Split Only by Line Count

Line count is a hint, not a rule.

A 500-line package can be healthy if it owns one cohesive idea. A 100-line package can be too large if it mixes transport, persistence, and business rules.

Use cohesion, dependency direction, and test friction as the real signals.

## Naming Rules

Prefer business names:

```text
deployment
workspace
access
analytics
dashboard
agent
```

Prefer use-case names:

```text
activate
validate
materialize
stream
grant
revoke
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

Avoid global horizontal names:

```text
handlers
services
repositories
models
utils
helpers
```

These names are acceptable inside a capability only when they stay small and specific.

## Example: Deployment Activation

Target shape:

```text
internal/deployment/
  deployment.go
  repository.go
  activate/
    service.go
  sqlite/
    repository.go
  filesystem/
    artifact_store.go
  http/
    handlers.go
```

Flow:

```text
deployment/http.Handler
    -> deployment/activate.Service
        -> activate.Repository
        -> activate.RuntimeActivator
    <- activate.Result
```

The handler knows HTTP. The service knows the activation workflow. The repository knows SQLite. The runtime activator knows how to prepare and commit the active DuckDB runtime. The domain knows what deployment states are valid.

## Example: Dashboard Updates

Target shape:

```text
internal/dashboard/
  dashboard.go
  filters.go
  table.go
  stream/
    service.go
  command/
    service.go
  datastar/
    patches.go
  http/
    handlers.go
```

Flow:

```text
dashboard/http.UpdatesHandler
    -> dashboard/stream.Service
        -> dashboard.QueryIntent
        -> analytics/query.Engine
    <- dashboard.PageSnapshot
    -> dashboard/datastar.Patch
```

Dashboard code owns report-page behavior, filter state, selections, table windows, and page snapshots. Analytics code owns semantic query planning and execution. Datastar code owns signal translation.

## Migration Guidance

Architecture should improve through focused moves:

1. Extract a cohesive use case from `internal/app`.
2. Define the smallest port needed by that use case.
3. Move sqlc/direct storage access behind an adapter.
4. Move HTTP/Datastar translation to an adapter package.
5. Add use-case tests with fakes.
6. Repeat for the next workflow.

Primary migration target:

- Retire `internal/data/DuckDBMetrics` as the cross-cutting runtime object. Split it into:
  - `analytics/duckdb` for DuckDB connections and execution.
  - `analytics/materialize` for model materialization refresh.
  - `analytics/query` for semantic query planning/execution ports.
  - `dashboard/stream` or `dashboard/runtime` for page snapshots, table windows, and command orchestration.
  - `workspace/catalog` and `workspace/compiler` for catalog/workspace loading and normalized runtime workspace construction.

Other good candidates:

- Deployment activation and validation.
- Workspace asset listing and access view shaping.
- RBAC grant/revoke/authorize workflows.
- Dashboard command handling and update streaming.
- Extracting sqlc access behind capability repositories and removing raw `Queries()` calls outside adapters/composition.

The architecture is successful when a developer can understand and test one capability without loading the whole application into their head.
