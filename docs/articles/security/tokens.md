# Service principals and API tokens

Non-human workloads should use a dedicated service principal and scoped credential. A token or service-principal secret authenticates an existing identity; it does not create identity or grant privileges by itself.

## Choose the principal type

Use a service principal for CI, deployment, data synchronization, scheduled integration, or another workload with its own lifecycle. Use a user API token only for a person's short-lived automation where individual ownership is intentional.

Never share one human token across several systems. Shared credentials remove attribution, make rotation disruptive, and allow one compromised workload to impersonate unrelated automation.

## Create and authorize a service principal

The [Access API](/docs/api/access) can create, update, list, and delete service principals and issue/revoke their secrets. Create one principal per workload boundary, then assign only the role or grants it requires.

Examples of separate identities include:

- a project deployer for one environment;
- a managed-data publisher for one project;
- a read-only semantic-query integration for one workspace;
- an MCP workload allowed to use the agent and only the required asset workspaces;
- a monitoring principal for bounded synthetic queries.

Do not grant platform administration because a narrower workflow initially returns `403`. Inspect effective privileges and add the missing specific privilege at the correct scope.

## Handle the credential

Creation responses return new secret material once. Send it directly into the CI or deployment secret manager. Do not write it to project YAML, committed `.env` files, container layers, Terraform state/output, command examples, build logs, or plan artifacts.

CLI automation can inject the target and token at runtime:

```sh
LEAPVIEW_TARGET=https://dash.example.com
LEAPVIEW_API_TOKEN=<secret>
```

Avoid command-line token flags where process listings or shell history may expose them. The CLI reference documents flags, but environment injection or protected CLI configuration is generally safer for automation.

## User API tokens

An authenticated user can list, create, and revoke their tokens through `/api/v1/me/api-tokens`. Token access is the intersection of the principal's effective privileges, any token workspace scope, and token privilege allowlist. A token can narrow the principal; it cannot elevate it.

The same user can inspect effective privileges and active sessions through the Current User API. Revoke unused sessions during credential or device incidents.

## Rotate and verify safely

Use overlapping rotation where the secret API and workload allow it:

1. Create a new secret with the same intended scope.
2. Store it as a new secret-manager version.
3. Roll out the workload and verify authenticated operations.
4. Revoke the old secret.
5. Confirm old-credential attempts fail.
6. Review audit events for unexpected use during the overlap.

Do not extend overlap indefinitely. Record owner, purpose, creation, last rotation, expiry where supported, and revocation date.

## Respond to exposure

Revoke the credential immediately, then inspect audit and query events for the principal, affected workspaces, operations, and time window. Rotate downstream secrets that the workload could access, correct excessive grants, and issue a replacement only after the cause is contained.

Deleting or disabling a service principal is appropriate when the workload is retired. Remove role bindings and ownership references as part of the same change.

See [Current User API](/docs/api/current-user), [Access API](/docs/api/access), and [Automation and CI](/docs/cli/automation).
