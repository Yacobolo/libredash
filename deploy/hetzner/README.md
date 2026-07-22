# Hetzner single-node deployment

This Terraform deployment runs LeapView and Caddy on one Hetzner Cloud server.
It provides automatic HTTPS, generated production secrets, restricted SSH,
daily provider backups, consistent application backups, and healthchecked
upgrades with rollback. It is the supported small-instance topology, not a
high-availability deployment. Managed data uses the local backend by default and
is stored on the persistent application-state volume.

## Deploy

Prerequisites:

- Terraform 1.7 or newer
- A Hetzner Cloud API token
- An SSH public key
- The immutable image reference from a LeapView release's
  `image-reference.txt` asset

```sh
cd deploy/hetzner
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars
export HCLOUD_TOKEN=...
terraform init
terraform apply
```

Set three values in `terraform.tfvars`: `admin_email`, `leapview_image`, and
`ssh_allowed_cidrs`. Use your public address with a `/32` suffix for SSH. The
module deliberately rejects world-open SSH and mutable image tags.

Provisioning extracts the matching Go `leapviewctl` binary from the immutable
multi-architecture application image. The provider then uses the same Compose
lifecycle controller and files as the generic self-hosting package.

When `domain` is empty, the deployment uses an HTTPS `sslip.io` hostname. That
is useful for evaluation. Set a domain you control for a durable installation.

## First Login

Provisioning creates a local platform administrator, a forced-change temporary
password, and a privilege-restricted publisher token that expires after 24
hours. Retrieve them once:

```sh
terraform output -raw initial_local_user_command | sh
```

The command removes the root-only credential file after printing it. Sign in at
`terraform output -raw url`, change the temporary password, and store the
publisher token with the CLI before it expires:

```sh
leapview login \
  --target "$(terraform output -raw url)" \
  --token '<publisherToken>'
```

Initialization is offline; no unrestricted bootstrap token is created or sent
over HTTP.

## Deploy Project

```sh
leapview deploy \
  --project ../../dashboards/leapview.yaml \
  --revision "olist=sha256:<64-lowercase-hex>" \
  --target "$(terraform output -raw url)" \
  --auto-approve
```

For project-global file ingestion, follow the [managed data ingestion
guide](../../docs/data-ingestion.md). `data sync` stages a revision; the
project-level deploy pins that revision and atomically activates every project
workspace. Omit `--revision` for projects without managed connections, or
repeat it exactly once per managed connection.

## Operations

Terraform exposes an SSH prefix for the server-side lifecycle command:

```sh
$(terraform output -raw operations_command) status
$(terraform output -raw operations_command) logs
```

Important paths:

- Docker volume `leapview_leapview-state`: application state, analytical data, and local managed data
- `/opt/leapview/backups`: consistent local archives and checksums
- `/opt/leapview/leapview.env`: generated application configuration
- `/opt/leapview/deployment.env`: pinned images and deployment metadata

The server creates a consistent backup every day and retains seven local
archives. Hetzner's daily server backups are also enabled. The application is
stopped while the archive is created, and the complete `/var/lib/leapview`
tree is captured, including the local managed-data object store.

Create and restore an archive manually:

```sh
ARCHIVE="$($(terraform output -raw operations_command) backup)"
$(terraform output -raw operations_command) restore "$ARCHIVE"
```

Restore verifies the checksum and archive paths. If the restored instance fails
its healthcheck, the previous state is reinstated automatically.

If you reconfigure managed data to use S3, these archives retain LeapView
metadata and the local runtime cache but do not contain authoritative S3 object
data. Enable bucket versioning and a bucket-native backup or replication policy;
disaster recovery requires both the metadata archive and the bucket objects.

For independent encrypted backups, place a root-only Restic environment file at
`/etc/leapview/restic.env`. Subsequent scheduled backups initialize the
repository if needed and retain 7 daily, 4 weekly, and 12 monthly snapshots.
Standard Restic variables such as `RESTIC_REPOSITORY`, `RESTIC_PASSWORD`, and
the selected object store's credentials are supported.

## Upgrade

Use the immutable digest published with the target release:

```sh
$(terraform output -raw operations_command) upgrade \
  ghcr.io/yacobolo/leapview@sha256:<digest>
```

The command creates a pre-upgrade state checkpoint, pulls and validates the
image, starts it, and waits for health. A failed healthcheck automatically
restores both the previous image and its paired state. Explicitly switch back
after a successful upgrade, discarding post-upgrade state, with:

```sh
$(terraform output -raw operations_command) rollback --confirm
```

## Destroy

Export an independent backup first. Hetzner server backups are deleted with the
server.

```sh
terraform destroy
```

The deployment stores no application secrets in Terraform state or outputs.
See the generated [configuration reference](../../docs/configuration.md) for the
complete process-global LeapView environment contract.
