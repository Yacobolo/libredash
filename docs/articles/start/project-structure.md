# Project structure

A LeapView project separates project-global data inputs from workspace-owned analytical and presentation resources. The boundary is deliberate: connections and sources describe shared inputs, while each workspace owns the models and user experiences built from the inputs it is allowed to use.

```text
dashboards/
  leapview.yaml
  connections/
    warehouse.yaml
  sources/
    warehouse.orders.yaml
  workspaces/
    sales/
      workspace.yaml
      models/
        orders.yaml
      semantic-models/
        sales.yaml
      dashboards/
        executive-sales.yaml
      access/
      agent/
```

The names are conventions rather than hard-coded discovery paths. Include lists in the project and workspace manifests decide which files belong to a deployment.

## Project entry point

The project manifest is the root of configuration discovery:

```yaml
apiVersion: leapview.dev/v1
kind: Project
metadata:
  name: commerce
spec:
  connections:
    include: [connections/*.yaml]
  sources:
    include: [sources/*.yaml]
  workspaces:
    include: [workspaces/*/workspace.yaml]
```

Paths are resolved relative to the containing resource. Keep include patterns narrow enough that ownership remains obvious; a resource should be discovered once by one containing project or workspace.

## Project-global resources

Connections define how LeapView reaches physical data. Sources use a connection and provide stable logical names, paths, and field definitions. They are project-global because several workspaces may safely reuse the same governed input without copying credentials or physical location details.

Managed-data planning and revision activation also operate at project scope. A deployment can therefore pin a consistent set of shared input revisions while changing several workspaces atomically.

## Workspace resources

Each `workspace.yaml` declares which project sources the workspace may consume and discovers its own resources:

- `models/` contains materialized analytical tables.
- `semantic-models/` contains reusable dimensions, measures, metrics, and relationships.
- `dashboards/` contains filters, visual queries, tables, pages, and layout.
- `access/` contains groups, role bindings, grants, and data policies.

Workspace metadata supplies the stable resource name used in routes and authorization. Renaming that identifier is not the same as changing its display title; treat identifier changes as migrations.

## Resource identity and metadata

Every resource uses the same envelope: `apiVersion`, `kind`, `metadata`, and `spec`. `metadata.name` is a stable machine identifier. `metadata.title`, `description`, `owner`, and `tags` communicate intent without changing identity. Workspace-owned resources also declare `metadata.workspace` where required by their schema.

Use lower-case, stable names and avoid encoding environment names in resource IDs. Deploy the same project source to separate dev, staging, and production instances; do not create parallel `sales-dev` and `sales-prod` resource trees.

## Validate discovery

Validate from the project root after moving files or changing include patterns:

```sh
go run ./cmd/leapview validate --project dashboards/leapview.yaml
```

Validation catches duplicate resources, missing includes, invalid references, unsupported fields, and other contract failures before deployment. The generated [Project configuration](/docs/config/project) and [Workspace configuration](/docs/config/workspace) pages remain the source of truth for exact fields.
