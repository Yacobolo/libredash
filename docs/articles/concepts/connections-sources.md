# Connections and sources

Connections and sources deliberately answer different questions: **how can LibreDash reach data?** and **which logical input should a workspace consume?** Keeping those answers separate prevents credentials and physical locations from leaking into analytical models.

## Connections

A connection describes a physical access method and its defaults. Supported connection kinds are defined by the generated schema and currently include managed data, object storage, HTTP, relational databases, SQLite, DuckLake, and Quack-compatible access.

```yaml
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: olist
  title: Olist CSV files
spec:
  kind: managed
  credentials:
    provider: none
  defaults:
    options:
      header: true
```

Connection options are connector-specific. Put options shared by its sources under `defaults`; keep per-source format, path, object, or reader options on each source. The generated [Connection configuration](/docs/config/connection) page lists the accepted top-level fields, while the connector implementation defines the meaning of connector-specific options.

Do not store secret values directly in project YAML. Use the supported credential provider and runtime secret boundary. A connection can name an environment-backed secret without placing its value in Git.

## Sources

A source gives one accessible object a stable project identity:

```yaml
apiVersion: libredash.dev/v1
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
      description: Raw order identifier.
```

The source may identify a path, database object, format, options, and declared fields. Use a name that reflects the governed dataset rather than a temporary filename. Model tables and workspace permissions depend on that stable name.

Field declarations document expected input shape and improve validation and discovery. They do not replace defensive transformations: model-table SQL should still cast or reject malformed physical values where necessary.

## Workspace permission

A workspace lists the project sources it may use under `spec.uses.sources`. Listing a source establishes the configuration dependency; it does not copy the source or its credentials into the workspace.

```yaml
spec:
  uses:
    sources:
      - olist.orders
      - olist.customers
```

Validation should fail when a model table references an undiscovered source or a source the workspace has not declared. This keeps repository layout from becoming an accidental authorization mechanism.

## Managed and external data

Managed connections participate in the plan, stage, revision, and activation lifecycle. The file content is identified by immutable revision state before deployment activates it. External connectors resolve their configured systems according to the connector and refresh contract; their availability and consistency remain operational dependencies.

Use managed data when LibreDash should own the uploaded object revision. Use an external connection when an existing system remains the source of truth and LibreDash should read it in place.

## Change safely

Changing a connection endpoint or source path can affect every dependent workspace. Before deployment:

1. Search the dependency graph for consumers.
2. Validate the whole project.
3. Plan against the target environment.
4. Refresh affected model tables in a non-production environment.
5. Compare row counts, null behavior, types, and key uniqueness.

Continue with [Connect a data source](/docs/guides/build/connect-data) for an authoring procedure and [Managed data ingestion](/docs/data-ingestion) for immutable managed files.
