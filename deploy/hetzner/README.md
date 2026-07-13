# Hetzner single-node deployment

This Terraform deployment runs LibreDash and Caddy on one Hetzner Cloud server.
It provides automatic HTTPS, generated production secrets, restricted SSH,
daily provider backups, consistent application backups, and healthchecked
upgrades with rollback. It is the supported small-instance topology, not a
high-availability deployment.

## Deploy

Prerequisites:

- Terraform 1.7 or newer
- A Hetzner Cloud API token
- An SSH public key
- The immutable image reference from a LibreDash release's
  `image-reference.txt` asset

```sh
cd deploy/hetzner
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars
export HCLOUD_TOKEN=...
terraform init
terraform apply
```

Set three values in `terraform.tfvars`: `admin_email`, `libredash_image`, and
`ssh_allowed_cidrs`. Use your public address with a `/32` suffix for SSH. The
module deliberately rejects world-open SSH and mutable image tags.

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
libredash login \
  --target "$(terraform output -raw url)" \
  --token '<publishToken>'
```

The unrestricted bootstrap token exists only in provisioning memory and is
rotated after these credentials are created.

## Publish

```sh
libredash publish \
  --project ../../dashboards/libredash.yaml \
  --target "$(terraform output -raw url)" \
  --environment prod \
  --auto-approve
```

## Operations

Terraform exposes an SSH prefix for the server-side lifecycle command:

```sh
$(terraform output -raw operations_command) status
$(terraform output -raw operations_command) logs
```

Important paths:

- `/var/lib/libredash`: application state and analytical data
- `/var/backups/libredash`: consistent local archives and checksums
- `/etc/libredash/libredash.env`: generated application configuration
- `/etc/libredash/deployment.env`: pinned images and deployment metadata

The server creates a consistent backup every day and retains seven local
archives. Hetzner's daily server backups are also enabled.

Create and restore an archive manually:

```sh
ARCHIVE="$($(terraform output -raw operations_command) backup)"
$(terraform output -raw operations_command) restore "$ARCHIVE"
```

Restore verifies the checksum and archive paths. If the restored instance fails
its healthcheck, the previous state is reinstated automatically.

For independent encrypted backups, place a root-only Restic environment file at
`/etc/libredash/restic.env`. Subsequent scheduled backups initialize the
repository if needed and retain 7 daily, 4 weekly, and 12 monthly snapshots.
Standard Restic variables such as `RESTIC_REPOSITORY`, `RESTIC_PASSWORD`, and
the selected object store's credentials are supported.

## Upgrade

Use the immutable digest published with the target release:

```sh
$(terraform output -raw operations_command) upgrade \
  ghcr.io/yacobolo/libredash@sha256:<digest>
```

The command pulls and validates the image, starts it, and waits for health. A
failed healthcheck automatically restores the previous image. Explicitly switch
back after a successful upgrade with:

```sh
$(terraform output -raw operations_command) rollback
```

## Destroy

Export an independent backup first. Hetzner server backups are deleted with the
server.

```sh
terraform destroy
```

The deployment stores no application secrets in Terraform state or outputs.
See the generated [configuration reference](../../docs/configuration.md) for the
complete process-global LibreDash environment contract.
