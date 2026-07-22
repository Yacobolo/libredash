# Automation and CI

Treat validation, planning, approval, and deployment as separate gates. Build one candidate from an immutable Git revision, review its plan, and deploy that unchanged candidate only from an approved branch or environment.

## Provide bounded credentials

Inject `LEAPVIEW_TARGET` and `LEAPVIEW_API_TOKEN` from the CI secret manager. Use a dedicated service principal, protect production secrets with environment approval, and prevent pull requests from untrusted forks from reading them. The validation job does not need either secret.

Keep the expected environment and workspace in reviewable pipeline configuration:

```sh
export TARGET_ENVIRONMENT=production
export TARGET_WORKSPACE=retail
```

## Validate without network access

Compile the complete project first and retain structured diagnostics as a job artifact:

```sh
leapview validate --project dashboards/leapview.yaml --json
```

Stop the pipeline on any non-zero exit status. Do not allow a later deployment job to replace or edit the project after validation.

## Generate a reviewable plan

Plan against the exact target identity and workspace that will receive the deployment:

```sh
leapview plan \
  --project dashboards/leapview.yaml \
  --target "$LEAPVIEW_TARGET" \
  --environment "$TARGET_ENVIRONMENT" \
  --workspace "$TARGET_WORKSPACE" \
  --json > leapview-plan.json
```

Publish `leapview-plan.json` as a protected artifact. Review removals, access-policy changes, resource identity changes, and managed-data revision pins. Regenerate the plan after any source change.

## Deploy after approval

Run deployment from a protected job using the same project, target, environment, and revision pins that produced the reviewed plan:

```sh
leapview deploy \
  --project dashboards/leapview.yaml \
  --target "$LEAPVIEW_TARGET" \
  --environment "$TARGET_ENVIRONMENT" \
  --auto-approve
```

`--auto-approve` is appropriate only because the pipeline provides the approval gate. Do not use it to bypass review in an unprotected job.

## Preserve evidence and verify

Record the Git revision, target environment, actor, plan artifact, deployment result, and managed-data digests together. After deployment, probe readiness and exercise a representative workspace query or dashboard. A transport retry must reuse the same immutable candidate; never rebuild from a moving branch between attempts.

See [Validate, plan, and deploy](/docs/cli/validate-deploy) for the operational sequence, [Targets and environments](/docs/cli/targets) for environment safeguards, and the generated [`validate`](/docs/cli/validate), [`plan`](/docs/cli/plan), and [`deploy`](/docs/cli/deploy) references for all flags.
