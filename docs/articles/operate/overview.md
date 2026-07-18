# Deploy and operate LibreDash

Production delivery has two distinct layers: application delivery and project delivery. Keeping them separate lets operators upgrade the service without silently changing dashboards and lets data teams deploy reviewed projects without rebuilding the application image.

## Application delivery

Application delivery changes the LibreDash binary or OCI image, browser assets, runtime configuration, and infrastructure. It also owns persistent storage attachment, TLS termination, health checks, backups, and upgrade rollback.

Build an immutable artifact in CI from a reviewed commit. Pin the artifact by digest or another immutable identifier in deployment configuration. Validate production environment requirements before the process is allowed to serve traffic:

```sh
libredash config validate --production
```

An application release should not modify project YAML as a side effect. When a release changes resource schemas, publish explicit migration instructions and update projects through their normal delivery workflow.

## Project delivery

Project delivery changes connections, sources, workspaces, model tables, semantic models, dashboards, access resources, agent policies, and managed-data revision pins. The normal flow is:

```sh
libredash validate --project dashboards/libredash.yaml

libredash plan \
  --project dashboards/libredash.yaml \
  --target "$LIBREDASH_TARGET" \
  --token "$LIBREDASH_API_TOKEN" \
  --environment prod

libredash deploy \
  --project dashboards/libredash.yaml \
  --target "$LIBREDASH_TARGET" \
  --token "$LIBREDASH_API_TOKEN" \
  --environment prod
```

Add one `--revision connection=sha256:<digest>` flag for each managed connection. Use `--auto-approve` only in automation that already has a reviewed plan and explicit promotion controls.

## Promotion

Promote the same reviewed project commit, application artifact, and managed revision digests through environments. Environment-specific targets provide URLs and credentials; they should not require duplicated dashboard trees.

A practical promotion record contains:

- application image digest and version;
- project commit and generated-plan artifact;
- environment and target identity;
- managed connection revision digests;
- deployment ID and acting service principal;
- validation and post-deployment evidence.

## Runtime operations

Operators are responsible for process readiness, storage capacity, backups, maintenance, refresh execution, and security configuration. Data teams remain responsible for project correctness, model invariants, and dashboard behavior. Both sides need a shared incident boundary because a failed query can originate in infrastructure, active data, semantic modeling, or the dashboard itself.

Monitor `/healthz` for liveness, `/readyz` for readiness, protected Prometheus metrics, structured logs, deployment state, refresh runs, and audit/query events. Keep readiness checks cheap; use separate synthetic tests for representative analytical queries.

## Recovery principles

- Preserve the last active serving state when a candidate fails.
- Back up authoritative control-plane, DuckLake, analytical, and managed-data boundaries together.
- Use dry-run maintenance and storage cleanup before destructive modes.
- Keep the previous immutable application artifact available during an upgrade.
- Diagnose with deployment, revision, refresh, and request identities rather than manually editing state.

Continue with [Production configuration](/docs/guides/operate/production-configuration), [Self-hosting](/docs/guides/operate/self-hosting), and [Health and observability](/docs/guides/operate/observability). Use the generated [CLI command reference](/docs/cli/reference) for exact flags.
