# Targets and environments

The target tells the CLI which LeapView instance to contact. The environment is the instance identity that the CLI expects to find there. Use both for production-like work so a valid credential or copied command cannot silently reach the wrong instance.

## Choose explicit boundaries

Give development, staging, and production separate URLs and credentials. A workspace name may exist in more than one instance, so it is not a sufficient deployment boundary by itself.

For a remote plan, name the target, expected environment, and workspace together:

```sh
leapview plan --project dashboards/leapview.yaml \
  --target https://dash.staging.example.com \
  --environment staging \
  --workspace retail
```

LeapView reads the target's environment before planning or deploying. If `--environment staging` reaches an instance that identifies itself as `production`, the command stops instead of continuing.

## Supply a target

For an occasional command, pass `--target` explicitly. For one shell or CI job, set `LEAPVIEW_TARGET`. For repeated local use, [`leapview login`](/docs/cli/login) stores a token under its exact target URL:

```sh
leapview login \
  --target https://dash.staging.example.com \
  --token "$LEAPVIEW_API_TOKEN"
```

Avoid aliases that can be repointed between environments. Use the same normalized URL when logging in and running later commands so the saved credential can be found.

## Keep local validation separate

[`leapview validate`](/docs/cli/validate) compiles the project locally and does not require a target. Run it before contacting any environment:

```sh
leapview validate --project dashboards/leapview.yaml
```

Then use an explicit remote target for the plan and deployment. Keep the project path, environment, target, workspace, and managed-data revision pins unchanged between review and activation.

## Verify before deployment

Check the target URL and asserted environment in reviewable CI configuration, not only in an operator's shell history. Use separate protected environments and secret scopes so a staging job cannot read the production token.

Continue with [Validate, plan, and deploy](/docs/cli/validate-deploy) for the full promotion workflow and [`leapview plan`](/docs/cli/plan) for every targeting option.
