# Upgrades and migrations

Treat an upgrade as a coordinated change to application code, browser assets, persistent schemas, runtime configuration, and supported project contracts. An image rollback is useful but does not automatically reverse a persistent-state migration.

## Assess the release

Before scheduling an upgrade, review release notes for:

- minimum Go, browser, database, or infrastructure requirements;
- control-plane or DuckLake migrations;
- environment variables added, removed, or made mandatory;
- resource schema changes and project migration steps;
- API or CLI compatibility changes;
- backup format changes;
- known rollback limitations.

Build or pull an immutable artifact and verify its provenance. Do not upgrade production from a mutable tag.

## Rehearse against restored state

Create and validate a production backup, then restore it into an isolated environment. Run the target version with production-like configuration and apply any documented project migration.

The rehearsal should cover startup migration, authentication, active deployments, semantic queries, dashboard interactions, refresh execution, backup creation, and the intended rollback procedure. Measure migration and restart duration to set the maintenance window.

## Prepare production

1. Confirm recent backups of every authoritative storage boundary.
2. Record current image digest, configuration version, active projects, and revisions.
3. Validate the target configuration with `libredash config validate --production`.
4. Pause or drain conflicting deployments, refreshes, and maintenance jobs.
5. Confirm disk headroom for migrations, new images, and rollback artifacts.
6. Notify users of the expected availability impact.

Use `libredash admin maintenance` or the deployment's maintenance mechanism only as documented by the release. Dry-run retention maintenance is not itself a general traffic-draining switch.

## Apply the upgrade

For the supported Hetzner topology, use the generated operations command with the target image digest. It pulls and validates the image, starts it, waits for health, and restores the prior image if health fails.

For another topology, preserve the same invariants: one controlled writer for persistent migrations, immutable artifact selection, bounded health wait, and an explicit decision point before old artifacts are removed.

Do not run two application versions against shared writable state unless the release explicitly declares mixed-version compatibility.

## Verify after startup

Check more than readiness:

- browser assets and route shell load without cache/version mismatch;
- local or external authentication completes and sessions persist correctly;
- expected workspaces, grants, and active deployments are present;
- one semantic model can be described and queried;
- one representative dashboard and interaction works;
- a refresh can complete and activate;
- metrics, logs, audit events, and backups still function;
- configuration validation reports no deprecated or missing settings.

Keep the maintenance window open until these checks pass.

## Roll back carefully

If the failure is limited to application behavior and persistent state remains backward compatible, return to the previous immutable artifact. If a migration changed persistent state incompatibly, follow the release's restore procedure instead of starting old code against new state.

Preserve failure logs, migration output, the target artifact, and post-failure state for diagnosis. A rollback restores service; it does not remove the need to understand the failed upgrade.

Project YAML remains on its own delivery cadence unless the new application version requires a resource migration. In that case, version application and project changes together in the promotion record.
