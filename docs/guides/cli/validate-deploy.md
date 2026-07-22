# Validate, plan, and deploy

Validation proves that the candidate project is structurally coherent. Planning compares that candidate with a target without activating it. Deployment publishes the reviewed candidate and switches serving state only after validation succeeds.

## Before you begin

Start from a clean branch with a project that resolves locally. Authenticate to the intended target with the narrowest publisher credential that can deploy the workspace. For managed connections, stage each input revision and retain its immutable digest.

Use the same project path, environment, target, and managed-data revisions throughout the sequence:

1. Validate the complete project locally.
2. Plan the candidate against the exact target and environment.
3. Review additions, changes, removals, and revision pins.
4. Deploy the unchanged candidate.
5. Verify the active workspace and representative queries.

Store target and credential values outside shell history:

```sh
export LEAPVIEW_TARGET=https://dash.example.com
export LEAPVIEW_API_TOKEN=<publisher-token>
```

## Validate the project

Run validation before contacting a target:

```sh
leapview validate --project dashboards/leapview.yaml
```

Resolve every configuration, discovery, reference, and policy diagnostic. Validation covers the complete resource graph, so a failure in an unchanged file can still block a candidate whose combined state is invalid.

Use `--json` in CI and fail the job on a non-zero exit status. Keep human-readable output for local review.

## Review the plan

Compare the candidate with the intended environment:

```sh
leapview plan \
  --project dashboards/leapview.yaml \
  --environment prod \
  --target "$LEAPVIEW_TARGET" \
  --token "$LEAPVIEW_API_TOKEN" \
  --revision "olist=sha256:<64-lowercase-hex>"
```

Review the resource identities, removals, access changes, and managed revision digests. Stop if the plan includes a workspace or source outside the change request. Generate the plan again after any edit; a reviewed plan does not authorize a later candidate.

## Deploy the reviewed candidate

Run deployment with the same arguments:

```sh
leapview deploy --project dashboards/leapview.yaml \
  --environment prod \
  --target "$LEAPVIEW_TARGET" \
  --token "$LEAPVIEW_API_TOKEN" \
  --revision "olist=sha256:<64-lowercase-hex>"
```

Do not use a mutable data location in place of a revision digest. Record the source revision, Git revision, target environment, deployment result, and operator or automation identity together.

## Verify the deployment

Open the target workspace and confirm the expected project revision is active. Exercise one catalog page, one representative semantic query, one dashboard with a filter, and one model refresh or table window affected by the change. Check application and audit logs for rejected candidates or background failures.

A failed candidate must leave the last valid serving state active. Treat that protection as recovery behavior, not as a reason to skip pre-deployment checks.

## Troubleshooting

If validation succeeds but planning fails, verify target authentication, environment identity, and server/client contract compatibility. If the plan shows unexpected removals, inspect workspace discovery patterns and stable resource IDs. If deployment rejects a managed revision, confirm that the digest was staged on the same target and connection named by the project. For authorization failures, inspect the publisher's effective grants rather than broadening the token immediately.

## Next steps

Automate this sequence with protected environments, immutable build artifacts, and retained JSON plan output. Continue with [Deploy and operate](/docs/guides/operate), [Production configuration](/docs/guides/operate/production-configuration), and the generated [`deploy`](/docs/cli/deploy), [`plan`](/docs/cli/plan), and [`validate`](/docs/cli/validate) references.
