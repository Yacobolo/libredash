# LeapView Configuration-as-Code North Star

This spec defines the target configuration-as-code model for LeapView. It is not
bound by the current implementation. Prefer quality, simplicity, robustness,
scalability, and long-term maintainability over development cost.

## Goal

LeapView configuration is a typed, versioned resource graph. Global data access
resources are defined once. Workspaces reuse those resources and own their BI
layer.

The model must answer:

- What data can LeapView access?
- What stable source contracts exist?
- Which workspaces may use each source?
- Which BI assets belong to each workspace?
- What changed before deployment?
- Which exact graph is active?

## Essential Principles

1. Use explicit scopes. A workspace is a boundary, not a default selection.
2. Keep connections and sources global.
3. Keep model tables, semantic models, dashboards, and workspace access scoped to one workspace.
4. Give every resource a stable, fully qualified ID.
5. Compile deterministic input into an immutable graph snapshot.
6. Validate before runtime. Reject ambiguity.
7. Reference credentials; never store secrets in config.
8. Separate environments from workspaces. `dev`, `staging`, and `prod` are deployment targets, not workspaces.

## Scopes

### Platform

Platform resources are global to one LeapView installation.

Platform resources:

- `Connection`
- `Source`
- Source schema contracts
- Credential references
- Global principals
- Identity-provider groups
- Platform admin roles

Platform resources must not depend on workspace resources.

### Workspace

A workspace is a BI asset container and the authorization parent for the assets
inside it. It is not a conversation or agent-identity boundary.

Workspace resources:

- `Workspace`
- Workspace role bindings
- Workspace-local groups, if LeapView manages groups directly
- `ModelTable`
- `SemanticModel`
- `Dashboard`
- Materialization settings

Workspace resources may depend on platform resources and resources in the same
workspace. They must not depend on another workspace unless an explicit sharing
contract exists.

### Deployment

A deployment is an immutable compiled snapshot for one workspace in one
environment.

Deployment snapshots include:

- Workspace resources
- Referenced global connections and sources
- Content hashes
- Dependency edges
- Validation result
- Runtime payloads
- Materialization targets

Activation changes only the active deployment pointer.

## Resource Shape

Every authored resource uses the same envelope:

```yaml
apiVersion: leapview.dev/v1
kind: Source
metadata:
  name: olist.orders
  title: Orders
spec:
  connection: olist
  path: olist_orders_dataset.csv
  fields:
    order_id:
      type: string
```

Required fields:

- `apiVersion`
- `kind`
- `metadata.name`
- `spec`

Optional common fields:

- `metadata.title`
- `metadata.description`
- `metadata.owner`
- `metadata.tags`

Resource IDs are stable. Display labels are not IDs. Renames are breaking graph
changes unless declared as migrations.

## Repository Layout

Use many focused files, not one mega-file.

```text
leapview.yaml
connections/
  olist.yaml
sources/
  olist.orders.yaml
  olist.customers.yaml
workspaces/
  sales/
    workspace.yaml
    models/
      orders.yaml
    semantic-models/
      sales.yaml
    dashboards/
      executive-sales.yaml
  finance/
    workspace.yaml
    models/
      invoices.yaml
```

Top-level project file:

```yaml
apiVersion: leapview.dev/v1
kind: Project
metadata:
  name: company-bi
spec:
  connections:
    include:
      - connections/*.yaml
  sources:
    include:
      - sources/*.yaml
  workspaces:
    include:
      - workspaces/*/workspace.yaml
```

Includes must expand deterministically. The compiler must reject duplicate IDs,
ambiguous ownership, and hidden imports.

## Connections

Connections define how LeapView reaches data systems. They are global because
many workspaces may use the same data system.

```yaml
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: olist
spec:
  kind: managed
  credentials:
    provider: none
```

Production credentials are references:

```yaml
apiVersion: leapview.dev/v1
kind: Connection
metadata:
  name: warehouse
spec:
  kind: postgres
  host: analytics-db.internal
  database: warehouse
  credentials:
    provider: env
    secret: LEAPVIEW_WAREHOUSE_DSN
```

Connection validation must check required fields, supported options, credential
references, and scope boundaries.

## Sources

Sources are global contracts over physical data. They describe data access, not
business meaning.

```yaml
apiVersion: leapview.dev/v1
kind: Source
metadata:
  name: olist.orders
spec:
  connection: olist
  path: olist_orders_dataset.csv
  format: csv
  fields:
    order_id:
      type: string
      description: Raw order identifier.
    order_status:
      type: string
```

Source contracts may include:

- Physical location or object name
- Format and read options
- Field names
- Field types
- Physical field descriptions
- Schema expectations
- Data classification metadata

Source contracts must not include:

- BI measures
- Business dimensions
- Dashboard labels
- Workspace-specific meaning
- Report-specific transformations

## Workspaces

Workspaces own BI assets and access policy. They read only allowed global
sources.

```yaml
apiVersion: leapview.dev/v1
kind: Workspace
metadata:
  name: sales
  title: Sales
spec:
  uses:
    sources:
      - olist.orders
      - olist.customers
  models:
    include:
      - models/*.yaml
  semanticModels:
    include:
      - semantic-models/*.yaml
  dashboards:
    include:
      - dashboards/*.yaml
  access:
    include:
      - access/*.yaml
```

`spec.uses.sources` is an allowlist. Workspace model tables must not read sources
outside the allowlist.

## Model Tables

Model tables are workspace-scoped transformations over allowed sources or other
model tables in the same workspace.

```yaml
apiVersion: leapview.dev/v1
kind: ModelTable
metadata:
  workspace: sales
  name: orders
spec:
  primaryKey: order_id
  sources:
    - olist.orders
  fields:
    order_id:
      type: string
    status:
      type: string
  transform:
    sql: |
      SELECT order_id, order_status AS status
      FROM source."olist.orders"
```

The compiler must verify declared source dependencies against SQL references.

## Semantic Models

Semantic models are workspace-scoped business query contracts over model tables.
They do not read connections or sources directly.

```yaml
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables:
    - orders
  measures:
    order_count:
      fact: orders
      aggregation: count
      empty: zero
```

Semantic models define relationships, conformed dimensions, typed atomic
measures, derived metrics, formatting, and query behavior. Facts are inferred
from atomic measure ownership.

## Dashboards

Dashboards are workspace-scoped presentation contracts over semantic models.

```yaml
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  workspace: sales
  name: executive-sales
spec:
  semanticModel: sales
  pages:
    - name: overview
```

Dashboards must not reference physical sources or connections.

## Access

Principals are global. Workspace permissions are scoped.

Workspace access is authored with workspace-local resources.

```yaml
apiVersion: leapview.dev/v1
kind: WorkspaceGroup
metadata:
  workspace: sales
  name: analysts
spec:
  members:
    - email: analyst@example.com
```

```yaml
apiVersion: leapview.dev/v1
kind: WorkspaceRoleBinding
metadata:
  workspace: sales
  name: analysts-viewer
spec:
  role: viewer
  subject:
    kind: group
    group: analysts
```

Rules:

- Users and service accounts are global principals.
- External identity-provider groups are global.
- LeapView-managed groups are workspace-local unless declared platform-wide.
- Role bindings grant a principal or group a role in one workspace.
- Platform roles are only for installation administration.

Global admin UI must not imply that users belong to one workspace. Workspace
access UI manages workspace role bindings.

## Agent configuration

Agent provider credentials, model choice, and the administrator-controlled
system prompt are global runtime configuration. Conversations are owned by a
principal. A workspace is selected only by an explicit argument to a
workspace-aware tool, and every tool call enforces credential and resource
authorization for that workspace.

## Environments

Environment is deployment context. Workspace is permission and BI asset scope.
Do not use workspaces to mean `dev`, `staging`, or `prod`.

Environment-specific values may include:

- Credential provider bindings
- Runtime endpoints
- Storage locations
- Deployment target
- Feature flags

Environment-specific values must not change resource identity.

## Validation

Validation must fail before deployment for:

- Duplicate resource IDs
- Unknown references
- Cyclic dependencies
- Cross-scope dependency violations
- Non-deterministic includes
- Ambiguous ownership
- Unsupported schema versions
- Invalid connection kinds
- Missing credential references
- Source paths outside connection scope
- Workspace reads outside `uses.sources`
- SQL references not declared as dependencies
- Semantic models referencing missing tables or fields
- Dashboards referencing missing semantic models or assets

Diagnostics must include resource ID, file path, and field path.

## Plan

Before deployment, LeapView must show a plan against the active deployment and
require explicit approval before activation.

The plan must report:

- Added resources
- Changed resources
- Removed resources
- Dependency changes, with `breaking` when a removed dependency changes an active BI contract
- Breaking changes
- Materialization impact
- Access policy changes, if access is managed from code

The plan must be deterministic for the same inputs.

Breaking changes include:

- Removing a source used by a workspace
- Removing or changing a source field used downstream
- Removing a model table field used by a semantic model
- Removing a semantic model field used by a dashboard
- Renaming a resource without a migration

## Compilation

Compilation turns authored resources into a normalized graph.

Each graph node includes:

- Resource ID
- Kind
- Scope
- Workspace ID, when scoped
- Source file
- Content hash
- Normalized payload

Each graph edge includes:

- From resource ID
- To resource ID
- Dependency type

Runtime code consumes the compiled graph, not raw YAML.

## Deployment

Deployments are immutable. A deployment is activated for one workspace in one
environment.

Deployment artifacts must include:

- Compiled graph
- Runtime payloads
- Referenced global resource snapshots
- Validation result
- Plan summary
- Content hashes

Rollback reactivates an older deployment artifact. It must not recompile from
current files.

## Naming

Use these terms consistently:

- `Connection`: data system access.
- `Source`: physical data contract.
- `Workspace`: BI asset and permission boundary.
- `ModelTable`: workspace transformation.
- `SemanticModel`: business query contract.
- `Dashboard`: presentation contract.
- `Environment`: runtime target.
- `Deployment`: immutable workspace snapshot.

Avoid:

- `default workspace` in production semantics.
- `workspace` as a synonym for platform or environment.
- Business metrics in global sources.
- Physical data access in dashboards.

## Non-Goals

Configuration-as-code is not:

- A secret store.
- A runtime mutation API.
- A substitute for source control review.
- A dashboard-only format.
- A multi-tenant mega-file.

## Summary

LeapView should define global data access once, let workspaces reference allowed
sources, and compile each workspace into immutable deployment snapshots. The
essential contract is scope, identity, deterministic input, validation, plan, and
reproducible deployment.
