# Install and authenticate the CLI

Use a LeapView release for routine operation, or run the CLI from a checked-out repository when developing LeapView itself. Confirm that the command you invoke matches the server contract you intend to operate:

```sh
leapview --help
```

## Authenticate an interactive workstation

Create a narrowly scoped API token for the target, then store it with [`leapview login`](/docs/cli/login):

```sh
leapview login --target https://dash.example.com --token "$LEAPVIEW_API_TOKEN"
```

Login verifies the target before writing the credential. The saved token is associated with the exact target URL and the client configuration file is restricted to the current user. Do not commit that file, copy it into an image, or share it between operators.

After login, commands can use the saved token when the same target is supplied:

```sh
leapview plan \
  --project dashboards/leapview.yaml \
  --target https://dash.example.com \
  --workspace retail
```

## Authenticate automation

In CI, inject `LEAPVIEW_TARGET` and `LEAPVIEW_API_TOKEN` from the platform's secret manager. Do not run `leapview login`: ephemeral jobs should not persist credentials to a home directory.

```sh
export LEAPVIEW_TARGET=https://dash.example.com
export LEAPVIEW_API_TOKEN="$PUBLISHER_TOKEN"
leapview plan --project dashboards/leapview.yaml --workspace retail --json
```

Use a dedicated service identity with only the grants required by the job. Scope production credentials to protected branches or approved deployment environments, mask them in logs, and rotate them without changing the project source.

## Diagnose authentication failures

First confirm the target URL is exact and reachable. A token saved for one URL is not reused for a different hostname or scheme. Then check that the credential is present and has not expired or been revoked. An HTTP `401` usually means the credential could not be authenticated; `403` means the authenticated principal lacks permission for that operation.

Do not solve a `403` by immediately granting broad administrative access. Compare the failed operation with the service identity's effective grants and add the narrowest missing privilege.

See [Targets and environments](/docs/cli/targets) for safe target selection, [Automation and CI](/docs/cli/automation) for secret handling in pipelines, and the [CLI troubleshooting guide](/docs/cli/troubleshooting) for a diagnostic sequence.
