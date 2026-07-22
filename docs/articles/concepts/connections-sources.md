# Connections and sources

Connections and sources deliberately answer different questions: **how can LeapView reach data?** and **which logical input should a workspace consume?** Keeping those answers separate prevents credentials and physical locations from leaking into analytical models.

## Connections

A connection describes a physical access method and its defaults. Supported connection kinds are defined by the generated schema and currently include managed data, object storage, HTTP, relational databases, SQLite, and DuckLake.

```yaml
apiVersion: leapview.dev/v1
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

Object storage is the recommended external-file boundary. LeapView supports these v1 credential modes:

| Connection | Explicit `env` | Public `none` | Ambient identity |
| --- | --- | --- | --- |
| S3 | Access-key JSON | Yes, when explicitly declared | AWS default credential chain |
| Azure Blob | Connection string or service-principal JSON | No | Azure default credential chain with `accountName` |
| R2 and GCS | Provider-specific JSON | No | Not in v1 |
| HTTP(S) | Connector-specific | Yes | Not applicable |

Ambient S3 credentials may declare a non-secret `region` and `endpoint`. Ambient Azure credentials require the storage `accountName`. LeapView compiles these declarations into temporary, path-scoped DuckDB secrets; resolved credentials are not written to deployment artifacts.

The v1 object-storage contract is:

| Source boundary | Formats | Credential modes | Path boundary | Read consistency | LeapView-owned alternative | Backup owner |
| --- | --- | --- | --- | --- | --- | --- |
| S3 | CSV, JSON, Parquet, Excel, text, blob, Vortex, Delta, Iceberg, and Lance where the corresponding extension supports the object | `env`, `none`, `ambient` | Required connection `scope`; compiled secrets use the same scope | Direct read at discovery or refresh time | Managed data with a pinned revision | Source owner |
| Azure Blob | Same path-backed formats | `env`, `ambient` | Required connection `scope`; ambient also requires `accountName` | Direct read at discovery or refresh time | Managed data with a pinned revision | Source owner |
| R2 and GCS | Same path-backed formats supported by their S3-compatible access | `env` | Required connection `scope` | Direct read at discovery or refresh time | Managed data with a pinned revision | Source owner |
| Public HTTP(S) | Path-backed formats supported by the configured reader | `none` | URL scope constrains authored source paths | Direct read at discovery or refresh time | Download and publish as managed data | Source owner |
| Managed local or S3 uploads | CSV, JSON, Parquet, Excel, text, blob, Vortex, Delta, Iceberg, and Lance | LeapView-managed storage configuration | Immutable revision manifest | Explicit pinned revision | This is the managed alternative | LeapView operator; S3 objects also need bucket-native backup |

## Sources

A source gives one accessible object a stable project identity:

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

Managed connections participate in the plan, stage, revision, and activation lifecycle. The file content is identified by immutable revision state before deployment activates it. External connectors are direct reads: discovery or refresh observes whatever the configured object path exposes at that time. LeapView does not copy, pin, or version those objects in v1.

Use managed data when LeapView should own the uploaded object revision. Use an external connection when an existing system remains the source of truth and LeapView should read it in place.

For reproducible external refreshes, publish immutable object keys or versioned prefixes and change the source path through a reviewed project deployment. A mutable glob or overwritten key remains the source owner's consistency responsibility. A failed refresh does not replace the last successful serving snapshot.

## Change safely

Changing a connection endpoint or source path can affect every dependent workspace. Before deployment:

1. Search the dependency graph for consumers.
2. Validate the whole project.
3. Plan against the target instance.
4. Refresh affected model tables in a non-production environment.
5. Compare row counts, null behavior, types, and key uniqueness.

Continue with [Connect a data source](/docs/guides/build/connect-data) for an authoring procedure and [Managed data ingestion](/docs/data-ingestion) for immutable managed files.
