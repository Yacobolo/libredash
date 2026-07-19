# LibreDash

LibreDash is a dashboards-as-code BI monolith using Go, Datastar, Lit, DuckDB, and one global DuckLake catalog per instance.

## Install

The public container image is the quickest way to try LibreDash. This starts a
single development instance on localhost with persistent state:

```sh
docker pull ghcr.io/yacobolo/libredash:latest
docker volume create libredash-state
umask 077
docker run --rm \
  --volume libredash-state:/var/lib/libredash \
  --env LIBREDASH_PRODUCTION=0 \
  --env LIBREDASH_ENVIRONMENT=dev \
  --env LIBREDASH_BOOTSTRAP_ADMIN_EMAIL=admin@localhost \
  ghcr.io/yacobolo/libredash:latest \
  admin initialize --format json > initial-credentials.json
docker run --rm \
  --volume libredash-state:/var/lib/libredash \
  --env LIBREDASH_PRODUCTION=0 \
  --env LIBREDASH_ENVIRONMENT=dev \
  ghcr.io/yacobolo/libredash:latest \
  admin initialize --acknowledge-credentials
docker run --detach --name libredash --init \
  --publish 127.0.0.1:8080:8080 \
  --volume libredash-state:/var/lib/libredash \
  --env LIBREDASH_PRODUCTION=0 \
  --env LIBREDASH_ENVIRONMENT=dev \
  --env LIBREDASH_LOCAL_AUTH=1 \
  ghcr.io/yacobolo/libredash:latest serve
```

Read `initial-credentials.json`, open <http://localhost:8080>, and sign in with
the temporary administrator password. This pull-and-run path is for local
evaluation only. For a durable production instance, use the released
[Compose operations package](deploy/compose/README.md), which pins an immutable
image digest and adds generated secrets, HTTPS, backups, and state-aware
upgrades. See the [installation guide](docs/articles/start/installation.md) and
[self-hosting guide](docs/articles/operate/self-hosting.md).

## Contributor development

```sh
task dev
```

`task dev` installs JavaScript dependencies with Bun, builds Tailwind CSS and Bun-bundled browser assets, syncs the demo data, starts or reuses a managed dev server, chooses a worktree-safe port, and prints the URL. Use `task dev:status`, `task dev:logs`, and `task dev:stop` for lifecycle checks. Set `LIBREDASH_DEV_RESTART=1` to force a restart.

Generated implementation code, documentation indexes/reference pages, CSS, route entrypoints, and bundled component assets are intentionally not checked in. `task dev`, `task test`, and the public-site tasks generate their prerequisites automatically. If you run individual commands directly, generate and build first:

```sh
bun install
task generate
bun run build
go run ./internal/tools/bootstrapolist --out .data/olist
go run ./cmd/libredash
```

Local files are staged through managed data sync before deployment:

```sh
go run ./cmd/libredash data sync --project dashboards/libredash.yaml --connection olist --from .data/olist
```

## Architecture

- `GET /` redirects to the configured local workspace or workspace index.
- `GET /workspaces/{workspace}/dashboards/{dashboard}` opens a dashboard, and `GET /workspaces/{workspace}/dashboards/{dashboard}/pages/{page}` renders a report page.
- `GET /workspaces` renders published BI workspaces, and `GET /workspaces/{workspace}` renders canonical dashboard and semantic model assets.
- `GET /connections` renders global connection administration and inspection.
- `GET /workspaces/{workspace}/assets/{asset}/details` renders canonical asset details, including semantic model, model table, field, measure, source, and dashboard definitions.
- `GET /workspaces/{workspace}/assets/{asset}/lineage` renders canonical asset lineage.
- `GET /updates?...` is the canonical long-running Datastar SSE transport; pages open it from `data-init`, and commands publish signal patches back to that stream.
- `GET /workspaces/{workspace}/chat` renders workspace-scoped agent chat when the workspace policy enables it.
- DuckDB registers local CSV files as views and materializes model-scoped import tables.
- `dashboards/libredash.yaml` is the CaC project entrypoint for global connections/sources and workspace-scoped models, semantic models, dashboards, access, and agent policy.
- Semantic model YAML follows `sources -> models -> semantic model`: sources are raw physical inputs, models are light DuckDB-backed preparation tables, and semantic models own tables, fields, relationships, and measures.
- Dashboard YAML owns pages, filters, KPIs, visuals, tables, and interactions over semantic model fields and measures.
- Lit route components consume typed Datastar-backed page signals; dashboard visuals bind to signal payloads such as `visuals.revenue`.
- `api/signals/main.tsp` is the source of truth for UI signal payloads. APIGen generates the shared Go models and TypeScript types with `task ui-signals:generate`; handwritten adapters only translate internal dashboard domain values into those transport contracts.
- The bundled `datastar-inspector` web component shows live Datastar signals in the browser.

## Source Model

Semantic model YAML declares user-facing `sources` and named `connections`. LibreDash compiles these declarations into DuckDB `raw.*` and `source.*` views and keeps DuckDB extension, secret, and scan setup behind the source contract. Each source is an object with exactly one of `path` or `object`.

Model tables live under `models` and contain light preparation SQL or direct source references. They are not a general transformation framework; they are the place to align grain, clean fields, and prepare fact/dimension tables before the governed semantic model consumes them.

```yaml
models:
  orders:
    sources: [orders, payments]
    transform:
      sql: |
        SELECT o.order_id, o.customer_id, SUM(p.payment_value) AS revenue
        FROM source.orders o
        LEFT JOIN source.payments p ON p.order_id = o.order_id
        GROUP BY o.order_id, o.customer_id

  customers:
    source: customers
```

Semantic models expose model tables, safe relationships, conformed dimensions, typed atomic measures, and derived metrics. A measure's required `fact` identifies its owning table; a model may contain any number of facts. Dashboards query these directly.

```yaml
semantic_models:
  olist:
    tables:
      - orders
      - customers
    relationships:
      - id: orders_customers
        from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
    dimensions:
      customer_state:
        type: string
        bindings:
          orders:
            field: customers.state
            path: [orders_customers]
    measures:
      revenue:
        fact: orders
        aggregation: sum
        input: {field: orders.revenue}
        empty: zero
        format: currency
      order_count:
        fact: orders
        aggregation: count
        empty: zero
    metrics:
      revenue_per_order:
        expression: safe_divide(${revenue}, ${order_count})
        format: currency
```

Model-scoped aggregate queries independently aggregate each participating fact and stitch the results through semantic dimensions that bind to every fact. Table-scoped aggregate and row queries remain single-fact.

Managed CSV:

```yaml
default_connection: olist

connections:
  olist:
    kind: managed
    defaults:
      options:
        header: true

sources:
  orders:
    path: olist_orders_dataset.csv

  order_items:
    path: olist_order_items_dataset.csv
```

S3 Parquet with LibreDash-managed auth:

```yaml
connections:
  prod_lake:
    kind: s3
    scope: s3://analytics-prod/
    credentials:
      provider: ambient
      region: eu-west-1

sources:
  sales_events:
    connection: prod_lake
    path: events/*
    format: parquet
```

Azure Delta Lake:

```yaml
connections:
  azure_lake:
    kind: azure_blob
    scope: az://warehouse/
    credentials:
      provider: env
      secret: AZURE_STORAGE_CREDENTIALS

sources:
  delta_orders:
    connection: azure_lake
    path: tables/orders
    format: delta
```

Postgres table:

```yaml
connections:
  crm:
    kind: postgres
    credentials:
      provider: env
      secret: CRM_DATABASE_URL

sources:
  crm_accounts:
    connection: crm
    object: public.accounts
```

Lance source:

```yaml
connections:
  prod_lake:
    kind: s3
    scope: s3://analytics-prod/
    credentials:
      provider: ambient
      region: eu-west-1

sources:
  product_embeddings:
    connection: prod_lake
    path: vectors/products.lance
```

DuckLake catalog:

```yaml
connections:
  lakehouse:
    kind: ducklake
    scope: s3://analytics-prod/ducklake/
    path: metadata.ducklake
    credentials:
      provider: env
      secret: COMMERCE_DUCKLAKE_CREDENTIALS
    options:
      data_path: data

sources:
  lake_orders:
    connection: lakehouse
    object: main.orders
```

Experimental Quack remote query:

```yaml
connections:
  remote_quack:
    kind: quack
    path: quack:quack.example.com:443
    credentials:
      provider: env
      secret: LIBREDASH_QUACK_TOKEN

sources:
  remote_schemata:
    connection: remote_quack
    object: information_schema.schemata
```

LibreDash owns the credential contract. Use `credentials.provider: env` with an environment-variable name for explicit JSON credentials, `provider: ambient` for the S3 or Azure default credential chain, and `provider: none` only for deliberately public S3/HTTP data. LibreDash validates each mode and compiles temporary, path-scoped DuckDB secrets internally.

For file and table paths, LibreDash infers `format` from clear extensions such as `.csv`, `.csv.gz`, `.json`, `.jsonl`, `.ndjson`, `.parquet`, `.xlsx`, `.txt`, `.blob`, `.vortex`, and `.lance`. Set source-level `format` explicitly for ambiguous paths or table directories such as `events/*`, `format: delta`, and `format: iceberg`. Advanced DuckDB integrations should be modeled explicitly before being exposed in source YAML.

## Deploy

Start the development server; the default Olist project is synced and deployed
before the command reports readiness:

```sh
task dev
```

After data or YAML changes, run `task deploy:dev` and refresh or navigate the
UI. The server reads workspace assets, details, lineage, and versions from the
active deployment records.

For production, use the generic [Docker Compose package](deploy/compose/README.md). It provides pinned release images, optional Caddy HTTPS, generated secrets, complete instance backups, and health-checked image-and-state rollback. The [Hetzner recipe](deploy/hetzner/README.md) adds cloud provisioning and off-host operations.

Production mode serves the active deployed BI-as-code bundle from `.libredash` by default:

```sh
export LIBREDASH_PRODUCTION=1
export LIBREDASH_ENVIRONMENT=prod
export LIBREDASH_LOCAL_AUTH=1 # or configure OIDC/Azure below
export LIBREDASH_CSRF_KEY=<32+ byte secret>
export LIBREDASH_ALLOWED_HOSTS=localhost
export LIBREDASH_METRICS_BEARER_TOKEN=<32+ byte secret>
export LIBREDASH_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
libredash admin initialize --format json > initial-credentials.json
libredash admin initialize --acknowledge-credentials
libredash serve --production
SYNC_OUTPUT="$(libredash data sync --project dashboards/libredash.yaml --connection olist --from /srv/olist --target http://localhost:8080 --token <publisher-token-from-initialize>)"
REVISION="$(printf '%s\n' "$SYNC_OUTPUT" | awk '$1 == "staged" { print $2 }')"
libredash deploy --project dashboards/libredash.yaml --revision "olist=$REVISION" --target http://localhost:8080 --token <publisher-token-from-initialize> --environment prod --auto-approve
```

`deploy` validates the complete project, pins each supplied managed data
revision into its workspace artifacts, and activates all project workspaces in
one atomic rollout. Supply exactly one repeatable `--revision
"<connection>=sha256:<64-lowercase-hex>"` pin for every managed project
connection. Projects without managed connections omit the flag.
Create consistent instance backups with `libredash admin backup --out /backup/libredash-$(date +%Y%m%d%H%M%S).tar.gz`.
The archive includes the control-plane SQLite database, DuckLake catalog, deployed artifacts, DuckLake files, and other `LIBREDASH_HOME` state. Restore while the server is stopped; the command validates the archive and requires a backup path for the current instance before replacement:

```sh
libredash admin restore \
  --from /backup/libredash-20260706120000.tar.gz \
  --current-out /backup/libredash-before-restore-$(date +%Y%m%d%H%M%S).tar.gz \
  --confirm
```

`--database-only` is available for low-level SQLite maintenance, but it is not a full production recovery backup because artifacts and DuckLake data files live outside the database.
Full instance backup/restore is intentionally self-contained: keep `LIBREDASH_DUCKLAKE_CATALOG_PATH` inside `LIBREDASH_HOME` so the global DuckLake catalog is captured with the rest of the instance state.

Run operational history pruning from a scheduler after backups. It is a dry-run by default; pass `--apply` to delete rows older than the configured windows:

```sh
libredash admin maintenance \
  --audit-days 365 \
  --query-days 90 \
  --archived-agent-days 180 \
  --auth-state-days 30

libredash admin maintenance --apply
```

Build and run the production container with persistent control-plane storage mounted at `/var/lib/libredash`:

```sh
docker build -t libredash .
docker run --rm -p 8080:8080 \
  -v libredash-data:/var/lib/libredash \
  -e LIBREDASH_API_TOKEN_ONLY_AUTH=1 \
  -e LIBREDASH_CSRF_KEY=<32+ byte secret> \
  -e LIBREDASH_ALLOWED_HOSTS=localhost \
  -e LIBREDASH_METRICS_BEARER_TOKEN=<32+ byte secret> \
  -e LIBREDASH_BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
  libredash
```

The image runs as a non-root user, serves generated browser assets from `/app/static`, and keeps SQLite, DuckLake, artifacts, runtime files, and backups outside the image layer under `LIBREDASH_HOME`.

LibreDash uses one process-global environment contract. A minimal local-auth
production configuration is:

```sh
LIBREDASH_PRODUCTION=1
LIBREDASH_ENVIRONMENT=prod
LIBREDASH_HOME=/var/lib/libredash/home
LIBREDASH_LOCAL_AUTH=1
LIBREDASH_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
LIBREDASH_CSRF_KEY=<32+ byte secret>
LIBREDASH_ALLOWED_HOSTS=libredash.example.com
LIBREDASH_METRICS_BEARER_TOKEN=<32+ byte secret>
LIBREDASH_COOKIE_SECURE=true
```

See the generated [configuration reference](docs/configuration.md) for every
setting, default, lifecycle, and cross-setting production requirement. Run
`libredash config validate` in the deployment environment before starting the
server.

Local auth is admin-managed: users with grant-management access can create local
users from Admin / Principals and copy the one-time temporary password shown in
the response. Local users and local groups use the same grants and workspace
roles as OIDC/SCIM identities.

LibreDash reads production secrets from environment variables. Infisical is the recommended production workflow, but any env-based secret manager works:

```sh
infisical run --env=prod -- libredash serve --production
```

Use the generated `.env.example` as a valid local-auth production baseline; do not commit real `.env` files.

Production serve keeps the control-plane SQLite database and DuckLake catalog in separate files under `LIBREDASH_HOME`. It enables structured request logs, security headers, allowed-host validation, rate limits, a 128 MiB request body limit, bounded interactive query execution, and OAuth state cookies derived from `LIBREDASH_CSRF_KEY`.
`LIBREDASH_ALLOWED_HOSTS` accepts exact hosts and `*.example.com` wildcards. Browser auth deployments also allow the hosts from configured OIDC/Azure callback URLs; API-token-only production must set the allowlist explicitly.
Operational probes are exposed at `/healthz` and `/readyz`; Prometheus-compatible HTTP metrics are exposed at `/metrics`. Production requires `LIBREDASH_METRICS_BEARER_TOKEN`, and metrics scrapes must send `Authorization: Bearer <token>`.

## Test

```sh
task test
```
