# Self-hosting

LibreDash provides a supported small-instance topology for one independently managed node. The repository's Hetzner Terraform deployment runs LibreDash and Caddy together with automatic HTTPS, restricted SSH, generated production secrets, daily backups, and health-checked upgrades. It is not a high-availability topology.

## Before you begin

Prepare a domain, immutable application image digest, Hetzner credentials, an SSH public key, off-host backup storage, and an operator email address. Decide whether the public documentation site will be deployed separately; it has no runtime dependency on the BI application.

Use this production sequence:

1. Build and publish immutable application and optional documentation-site images.
2. Validate Terraform and the repository deployment contracts.
3. Provision with narrow network access and HTTPS.
4. Replace bootstrap credentials and deploy a revision-pinned project.
5. Verify health, queries, backups, restore behavior, and upgrade rollback.

## Build immutable artifacts

For application development, build from a clean checkout with pinned dependencies:

```sh
task generate
task build
go build ./cmd/libredash
```

Production automation should publish an OCI image and record its digest. Deploy the digest, not a mutable tag. The public documentation site is a separate binary and can be built with `task site:binary` when it is part of the release.

Build the public site image with `task site:image` (which uses `Dockerfile.site`). Set `LIBREDASH_SITE_BASE_URL` to its external HTTPS origin so canonical URLs, the sitemap, robots discovery, and HSTS describe the deployment rather than the internal listener. Point orchestration probes at `/healthz` and `/readyz` on port 8081.

## Validate the deployment contract

Run the repository deployment checks before provisioning:

```sh
task deploy:check
```

They cover Terraform formatting and validation plus security/lifecycle contracts such as bounded SSH access, immutable image references, backup wiring, and upgrade behavior.

## Provision the Hetzner topology

The deployment requires Terraform, a Hetzner Cloud token, an SSH public key, and an immutable LibreDash image reference. Copy the example variables file, set `admin_email`, `libredash_image`, and narrow `ssh_allowed_cidrs`, then apply the module from `deploy/hetzner`.

Use a domain you control for a durable installation. The generated `sslip.io` address is intended for evaluation. World-open SSH and mutable image tags are rejected by the module.

## Complete first login

Provisioning creates a local platform administrator with a forced-change temporary password and a limited publisher token. Retrieve the one-time output using the Terraform output command documented by the module, change the password immediately, and store the short-lived publisher token with `libredash login` only long enough to establish normal administration.

The unrestricted bootstrap credential should never become a reusable operator token.

## Understand persistent paths

The topology keeps application and analytical state beneath `/var/lib/libredash`, local managed objects beneath `/var/lib/libredash/managed-data`, and local backup archives beneath `/var/backups/libredash`. Generated service configuration lives outside the data root under `/etc/libredash`.

Runtime extraction and temporary directories may be ephemeral, but the control-plane database, DuckLake catalog, analytical data, artifact bundles, managed source objects, and backup destination are durable boundaries.

## Deploy a project

Stage managed data first, then deploy the project with one revision pin per managed connection:

```sh
libredash deploy \
  --project dashboards/libredash.yaml \
  --revision "olist=sha256:<64-lowercase-hex>" \
  --target https://dash.example.com \
  --environment prod
```

Verify the workspace catalog, a representative semantic query, and refresh behavior after activation.

## Operate and upgrade

Use the Terraform output `operations_command` as the SSH prefix for status, logs, backups, restores, upgrades, and rollback. The upgrade command accepts an immutable image digest, waits for health, and automatically restores the previous image on failed health. Keep an independent backup before upgrading even when image rollback is available.

The default scheduled application backup stops the application briefly to capture a consistent local state archive. Hetzner server backup is an additional layer, not a replacement for exportable recovery copies. Configure Restic or another independent destination for off-host retention.

## Verify the deployment

After provisioning, complete this acceptance check:

1. Confirm HTTPS, allowed-host enforcement, and application health.
2. Sign in as a non-administrator and verify only the intended workspace is visible.
3. Query a representative semantic measure and exercise a dashboard filter.
4. Trigger or observe a refresh and inspect its history and logs.
5. Create a backup, restore it in an isolated location, and verify its manifest.
6. Exercise the documented image rollback path before relying on it during an incident.

For the documentation site, request `/sitemap.xml`, confirm every URL uses the public origin, and verify that stable assets revalidate while fingerprinted chunks receive immutable caching.

## Know the limits

A single node has one failure and capacity domain. Schedule refreshes with query load, monitor disk and memory, and plan maintenance windows. If requirements demand horizontal serving, zero-downtime state migrations, or multi-region recovery, treat that as a different deployment architecture rather than stretching the single-node contract.

## Troubleshooting

If provisioning rejects the plan, fix mutable image references, world-open SSH ranges, or missing backup inputs instead of bypassing the contract. If the application is healthy internally but unreachable, inspect DNS, Caddy certificates, firewall rules, and allowed hosts in that order. If an upgrade fails health checks, preserve the failed logs and confirm automatic rollback restored the previous digest before attempting another change.

## Next steps

Complete [Production configuration](/docs/guides/operate/production-configuration), [Backup and restore](/docs/guides/operate/backup-restore), and [Health and observability](/docs/guides/operate/observability) before exposing the instance.
