# Managed data ingestion

LibreDash managed data turns local files into immutable, project-global data
revisions. A connection belongs to the project, not to an individual workspace.
DuckDB reads the active revision through its native CSV, Parquet, and other file
scanners; ingestion does not insert rows one at a time.

The examples below assume the CLI is already authenticated with `libredash
login`. Replace the paths, connection name, project ID, target, and environment
with values from your project.

## Plan a revision

Planning reads the project catalog, discovers the files used by the managed
connection, hashes them, and prints the canonical manifest and diff. It does not
contact the server or modify data.

```sh
libredash data plan \
  --project dashboards/libredash.yaml \
  --connection olist \
  --from /srv/olist
```

Use `--previous-manifest <path>` when you want the plan output to classify files
as added, changed, removed, or unchanged relative to an earlier manifest.

## Stage the files

Sync plans the same source tree and transfers only objects the server does not
already have. Local deployments use resumable tus uploads; S3 deployments use
direct multipart uploads. The CLI streams files and verifies that they do not
change during transfer.

```sh
libredash data sync \
  --project dashboards/libredash.yaml \
  --connection olist \
  --from /srv/olist \
  --target https://libredash.example.com
```

`data sync` stages an immutable revision only. It does not activate the revision
or change any serving workspace. Activation happens through the project deploy
command so project configuration and managed data revisions move together.

Deploy the project with the staged digest printed by `data sync`:

```sh
libredash deploy \
  --project dashboards/libredash.yaml \
  --revision "olist=sha256:<64-lowercase-hex>" \
  --environment prod \
  --target https://libredash.example.com \
  --auto-approve
```

Supply exactly one repeatable `--revision
"<connection>=sha256:<64-lowercase-hex>"` flag for every managed connection in
the project. The CLI rejects missing, duplicate, and unknown connection pins
before deployment. It uploads and validates every project workspace candidate
first, then the server switches all project-global revision pointers and
workspace serving states in one atomic rollout. A failed candidate leaves every
active revision and workspace unchanged. Projects with no managed connections
use the same deploy command without revision flags.

## Inspect revisions

Revision inspection uses the server project ID from the project's
`metadata.name`, not the local project file path.

```sh
libredash data revisions list \
  --project libredash-showcase \
  --connection olist \
  --target https://libredash.example.com

libredash data revisions current \
  --project libredash-showcase \
  --connection olist \
  --environment prod \
  --target https://libredash.example.com
```

The list command also accepts `--limit` and `--page-token`. The current command
prints the active revision digest, or `none` when the environment has no active
revision.

## Storage and recovery

The Hetzner single-node deployment uses the local backend and stores managed
objects under `/var/lib/libredash/managed-data`. Its stopped-state application
backup contains the object store and LibreDash metadata, so backup and restore
recover a complete local deployment.

With the S3 backend, LibreDash backups still contain the control-plane metadata
and local runtime cache, but not the authoritative S3 objects. Enable bucket
versioning and a bucket-native backup or replication policy. A recoverable S3
deployment requires both the LibreDash metadata backup and the corresponding
bucket objects.
