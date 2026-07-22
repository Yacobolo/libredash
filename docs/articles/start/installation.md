# Installation

LeapView ships as a public multi-architecture container image. Pulling that image is the primary onboarding path; no source checkout, registry login, or installer is required. One running container with one persistent state volume is one LeapView instance.

## Before you begin

Install Docker Engine. The quick start below is a localhost-only development instance for evaluation. A public production instance additionally needs Docker Compose, a DNS name, HTTPS, durable secret storage, and off-host backups.

## Try it from the public image

Pull the current stable release, create its persistent state volume, and initialize one local administrator:

```sh
docker pull ghcr.io/yacobolo/leapview:latest
docker volume create leapview-state
umask 077
docker run --rm \
  --volume leapview-state:/var/lib/leapview \
  --env LEAPVIEW_PRODUCTION=0 \
  --env LEAPVIEW_ENVIRONMENT=dev \
  --env LEAPVIEW_BOOTSTRAP_ADMIN_EMAIL=admin@localhost \
  ghcr.io/yacobolo/leapview:latest \
  admin initialize --format json > initial-credentials.json
docker run --rm \
  --volume leapview-state:/var/lib/leapview \
  --env LEAPVIEW_PRODUCTION=0 \
  --env LEAPVIEW_ENVIRONMENT=dev \
  ghcr.io/yacobolo/leapview:latest \
  admin initialize --acknowledge-credentials
```

Start the same instance on the loopback interface:

```sh
docker run --detach --name leapview --init \
  --publish 127.0.0.1:8080:8080 \
  --volume leapview-state:/var/lib/leapview \
  --env LEAPVIEW_PRODUCTION=0 \
  --env LEAPVIEW_ENVIRONMENT=dev \
  --env LEAPVIEW_LOCAL_AUTH=1 \
  ghcr.io/yacobolo/leapview:latest serve
```

Open <http://localhost:8080> and sign in with the temporary password in `initial-credentials.json`. Keep that owner-readable file private: it also contains a restricted publisher token that expires after 24 hours. The acknowledgement command removes LeapView's recovery copy only after the redirected file exists; delete your file when you no longer need either credential.

The state survives removal or replacement of the container because it lives in `leapview-state`. To stop and remove only the container, run `docker rm --force leapview`. Removing the named volume deletes the instance and is not part of normal shutdown.

Use `latest` for this disposable evaluation path. Pin a release version or digest anywhere repeatability matters.

## Run a durable production instance

The released Compose package is the recommended operations layer around the same public image. It is not a separate LeapView distribution. It supplies hardened container settings, generated production secrets, optional Caddy HTTPS, validated backup and restore, and paired image-and-state rollback.

1. Download the `leapview-compose-<version>-<os>-<arch>.tar.gz` asset and checksum matching the host from a LeapView release.
2. Verify the checksum and extract the archive into the host directory. The archive contains an immutable application image reference, the base Compose stack, an optional Caddy HTTPS overlay, and the native Go `leapviewctl` operations binary.
3. Copy the deployment template and initialize the instance:


```sh
cp deployment.env.example deployment.env
./leapviewctl init \
  --admin-email admin@example.com \
  --domain dash.example.com \
  --environment prod
```

4. Start the instance and consume the one-time credentials:

```sh
./leapviewctl start
./leapviewctl first-login
```

Initialization generates production secrets, creates the persistent volume, validates configuration, and atomically creates a forced-change local administrator plus a restricted publisher token. `first-login` prints and deletes that one-time credential file.

`leapviewctl` is an optional production operations controller, not a prerequisite for pulling or running LeapView. It invokes the installed Docker Compose CLI and does not require Bash or direct access to the Docker socket API. You may manage the image with your existing container platform if it preserves the same single-process, persistent-home, initialization, backup, and environment contracts.

The Caddy overlay is enabled by default. Pass `--no-https` only when an existing trusted HTTPS proxy fronts the localhost-bound application port. Keep secure cookies and the public allowed host configured for that proxy.

## Understand the instance boundary

All application-owned local state is under `/var/lib/leapview` in one named volume. External customer sources such as S3 remain external and are not included in instance backups. Local managed uploads are included; S3-backed managed uploads require bucket-native backup and versioning.

Use separate Compose project directories and names for development, staging, and production. Never scale one project to multiple application containers or point two processes at the same volume.

Common operations are:

```sh
./leapviewctl status
./leapviewctl logs
./leapviewctl backup
./leapviewctl restore backups/leapview-<timestamp>.tar.gz
./leapviewctl upgrade ghcr.io/yacobolo/leapview@sha256:<digest>
./leapviewctl rollback --confirm
```

Upgrades create a state checkpoint. A failed health check restores both the previous image and state; manual rollback requires confirmation because it discards state created after the checkpoint.

## Contributor installation

Source checkout is the contributor workflow, not the production packaging path. Install the Go version from `go.mod`, Bun, and Task, then run:

```sh
task node:deps
task generate
task dev
```

Use `task dev:status`, `task dev:logs`, and `task dev:stop` for the worktree-local server. Run `task ci` before handing off substantial changes.

## Validate

For the local image path, run `docker inspect --format '{{.State.Health.Status}}' leapview` and expect `healthy`. For Compose, run `docker compose config --quiet` and `./leapviewctl status`. A production application container must report healthy, and its resolved image must include a `sha256` digest.

## Verify

Open the configured HTTPS URL, sign in with the temporary administrator credentials, and change the password when prompted. Then create a backup with `./leapviewctl backup` and confirm that both the archive and its checksum exist in `backups/`.

## Troubleshooting

Use `./leapviewctl logs` when startup or health checks fail. A second process cannot open the same state volume, and an instance initialized for one environment cannot be started as another; use a separate Compose project and volume instead of changing `LEAPVIEW_ENVIRONMENT`.

## Next steps

Continue with [Self-hosting](/docs/guides/operate/self-hosting), [Connect a data source](/docs/guides/build/connect-data), and [Build your first dashboard](/docs/first-dashboard).

The commands above illustrate the installation workflow. Use the generated [`admin` CLI reference](/docs/cli/admin), [`serve` CLI reference](/docs/cli/serve), and [environment variable reference](/docs/configuration) for the exact current command and runtime contracts.
