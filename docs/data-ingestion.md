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
or change any serving workspace. A project release pins the revision digest
alongside every workspace artifact digest. Deploying that ready release moves
project configuration and managed data revisions together.

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
before creating the release. It uploads and validates every release artifact
first, then the server deploys the ready release by switching all project-global
revision pointers and workspace serving states in one atomic cutover. A failed
release validation or deployment leaves every active revision and workspace
unchanged. Projects with no managed connections use the same deploy command
without revision flags.

## Upload protocol contracts

Upload-session negotiation is part of the JSON product API under
`/api/v1/projects/{project}/connections/{connection}/upload-sessions`. It
selects an enabled transport and returns transport endpoints, expiry, and
required headers; it never returns storage credentials or resolved secrets.

For `tus`, clients follow the TUS resumable-upload protocol at
`/upload-protocols/tus`. Every request still requires the same bearer
authentication as `/api/v1`. The negotiated upload ID binds the transport
object to its product upload session, while TUS offsets and completion remain
transport concerns.

For `s3_multipart`, clients use the authenticated multipart commands nested
beneath the upload session to create an upload, sign each part, complete it, or
abort it. The server returns short-lived signed part URLs and the exact headers
to send, rather than AWS credentials. Completing either transport does not make
the revision active: clients must finalize the upload session and then pin the
resulting immutable revision in a release.

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
objects under `/var/lib/libredash/home/managed-data`. Its stopped-state application
backup contains the object store and LibreDash metadata, so backup and restore
recover a complete local deployment.

With the S3 backend, LibreDash backups still contain the control-plane metadata
and local runtime cache, but not the authoritative S3 objects. Enable bucket
versioning and a bucket-native backup or replication policy. A recoverable S3
deployment requires both the LibreDash metadata backup and the corresponding
bucket objects.
