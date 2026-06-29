# LibreDash

LibreDash is a small fullstack Go dashboard demo using gomponents, Datastar signals, Lit web components, and DuckDB over local Olist CSV files.

## Run

```sh
python3 -m pip install -r scripts/requirements.txt
npm install
task dev
```

`task dev` builds generated browser assets, syncs the demo data, starts a managed dev server, chooses a worktree-safe port, stops this worktree's stale server if one is already running, and prints the URL. Use `task dev:status`, `task dev:logs`, and `task dev:stop` for lifecycle checks.

Generated files such as `static/app.css`, `static/charts.js`, and other bundled component assets are intentionally not checked in. If you run the app without `task dev`, build assets first:

```sh
python3 -m pip install -r scripts/requirements.txt
npm install
npm run build
python3 scripts/bootstrap_olist.py
go run ./cmd/libredash
```

By default, the bootstrap script copies CSVs into `.data/olist`. To use a different path:

```sh
export LIBREDASH_DATA_DIR=/path/to/olist-csvs
npm run build
go run ./cmd/libredash
```

## Architecture

- `GET /` renders the file-backed dashboard catalog by mounting `ld-catalog-page` in the server document shell.
- `GET /dashboards/{dashboard}` opens a dashboard, and `GET /dashboards/{dashboard}/pages/{page}` renders a report page.
- `GET /workspaces` renders published BI workspaces, and `GET /workspaces/{workspace}` renders canonical dashboard and semantic model assets.
- `GET /connections` renders global connection administration and inspection.
- `GET /workspaces/{workspace}/assets/{asset}/details` renders canonical asset details, including semantic model, model table, field, measure, source, and dashboard definitions.
- `GET /workspaces/{workspace}/assets/{asset}/lineage` renders canonical asset lineage.
- `GET /updates?dashboard={dashboard}&page={page}` opens a long-running Datastar SSE stream and patches signals with `datastar.MarshalAndPatchSignals`.
- DuckDB registers local CSV files as views and materializes model-scoped import tables.
- `dashboards/catalog.yaml` discovers semantic models and dashboards.
- Semantic model YAML follows `sources -> models -> semantic model`: sources are raw physical inputs, models are light DuckDB-backed preparation tables, and semantic models own tables, fields, relationships, and measures.
- Dashboard YAML owns pages, filters, KPIs, visuals, tables, and interactions over semantic model fields and measures.
- Lit route components consume typed Datastar-backed page signals; dashboard visuals bind to signal payloads such as `visuals.revenue`.
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

Semantic models expose model tables, fields, safe relationships, and SQL aggregate measures. Dashboards query these directly.

```yaml
semantic_models:
  olist:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
          customer_id: {expr: customer_id}
          revenue: {expr: revenue, type: number}
      customers:
        model: customers
        primary_key: customer_id
        fields:
          state: {expr: customer_state}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults:
        table: orders
        grain: order_id
        time: orders.purchase_timestamp
        grains: [day, week, month, quarter, year]
      revenue:
        expression: SUM(orders.revenue)
        format: currency
```

`base_table` is the required semantic-model root; every table in the model must be reachable from it through one safe active relationship path.

Local CSV:

```yaml
default_connection: olist

connections:
  olist:
    kind: local
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
    auth:
      access_key_id: ${AWS_ACCESS_KEY_ID}
      secret_access_key: ${AWS_SECRET_ACCESS_KEY}
      region: ${AWS_REGION}

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
    auth:
      connection_string: ${AZURE_STORAGE_CONNECTION_STRING}

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
    auth:
      connection_string: ${CRM_DATABASE_URL}

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
    auth:
      access_key_id: ${AWS_ACCESS_KEY_ID}
      secret_access_key: ${AWS_SECRET_ACCESS_KEY}
      region: ${AWS_REGION}

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
    auth:
      access_key_id: ${AWS_ACCESS_KEY_ID}
      secret_access_key: ${AWS_SECRET_ACCESS_KEY}
      region: ${AWS_REGION}
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
    auth:
      token: ${LIBREDASH_QUACK_TOKEN}

sources:
  remote_schemata:
    connection: remote_quack
    object: information_schema.schemata
```

LibreDash owns the credential contract. Connection `auth` fields are resolved from `${ENV_VAR}` references or literal config values, validated by connection kind, and compiled into temporary DuckDB secrets internally. External secret managers such as Infisical should inject environment variables before `libredash serve` starts.

For file and table paths, LibreDash infers `format` from clear extensions such as `.csv`, `.csv.gz`, `.json`, `.jsonl`, `.ndjson`, `.parquet`, `.xlsx`, `.txt`, `.blob`, `.vortex`, and `.lance`. Set source-level `format` explicitly for ambiguous paths or table directories such as `events/*`, `format: delta`, and `format: iceberg`. Advanced DuckDB integrations should be modeled explicitly before being exposed in source YAML.

## Deploy

Production mode serves the active deployed BI-as-code bundle from `.libredash` by default:

```sh
export LIBREDASH_PRODUCTION=1
export LIBREDASH_API_TOKEN_ONLY_AUTH=1 # or configure Azure below
export LIBREDASH_CSRF_KEY=<32+ byte secret>
libredash serve --production
libredash admin bootstrap
libredash deploy --target http://localhost:8080 --token <token> --catalog dashboards/catalog.yaml
```

Useful env vars:

```sh
LIBREDASH_HOME=/var/lib/libredash
LIBREDASH_DATA_DIR=/path/to/data
LIBREDASH_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
LIBREDASH_AZURE_CLIENT_ID=...
LIBREDASH_AZURE_CLIENT_SECRET=...
LIBREDASH_AZURE_CALLBACK_URL=https://your-host/auth/azureadv2/callback
LIBREDASH_AZURE_TENANT=...
LIBREDASH_CSRF_KEY=<32+ byte secret>
LIBREDASH_COOKIE_SECURE=true
```

LibreDash reads production secrets from environment variables. Infisical is the recommended production workflow, but any env-based secret manager works:

```sh
infisical run --env=prod -- libredash serve --production
```

Use `.env.example` as the list of required/common variables; do not commit real `.env` files.

Production serve enables structured request logs, security headers, rate limits, and OAuth state cookies derived from `LIBREDASH_CSRF_KEY`.

## Test

```sh
task test
```
