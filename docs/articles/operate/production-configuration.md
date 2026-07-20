# Production configuration

LeapView configuration is process-global. Production instances must make every external boundary explicit and pass the generated relationship checks before serving traffic.

## Start with validation

Validate the environment without printing configured values or secrets:

```sh
leapview config validate --production
```

Run this in artifact smoke tests and again in the deployment environment. `--production` applies production rules even if `LEAPVIEW_PRODUCTION` is not set; the serving process should also set `LEAPVIEW_PRODUCTION=true`.

## Server and public address

Set the listen address and explicit accepted hosts. Terminate TLS at a maintained reverse proxy or load balancer and preserve the public scheme/host required by authentication callbacks.

Set `LEAPVIEW_PUBLIC_URL` to the application deployment's canonical HTTPS origin. The MCP endpoint and OAuth resource are derived as `${LEAPVIEW_PUBLIC_URL}/mcp`, so this must be the BI application's address, not the separate `https://leapview.dev` documentation site. See [Connect an MCP host](/docs/guides/integrate/mcp) for the complete discovery and authentication flow.

`LEAPVIEW_TRUST_PROXY_HEADERS` must be enabled only when requests arrive through a trusted proxy that overwrites client-address headers. Never trust forwarding headers from an arbitrary public client.

Browser authentication in production requires secure cookies. Configure exact public OIDC or Azure callback URLs and register those same URLs with the identity provider.

## Authentication and security secrets

Production requires at least one supported authentication mode: local browser auth, generic OIDC, Azure/Entra, or API-token-only mode. Development auth bypass is forbidden.

Generate independent high-entropy values for:

- `LEAPVIEW_CSRF_KEY` for CSRF protection and OAuth state;
- `LEAPVIEW_TOKEN_HASH_KEY` when a dedicated API-token fingerprint key is desired;
- `LEAPVIEW_METRICS_BEARER_TOKEN` for the metrics endpoint;
- `LEAPVIEW_SCIM_BEARER_TOKEN` when SCIM provisioning is enabled;
- identity-provider client secrets and external storage credentials.

The production validator enforces minimum lengths and all-or-none provider settings where applicable. Store values in the deployment secret manager, not project YAML, image layers, Terraform outputs, shell history, or generated plans.

## Persistent storage

Configure a durable `LEAPVIEW_HOME` and the paths required for the control-plane database, global DuckLake catalog, analytical data, artifacts, and managed-data runtime. The service identity must own these private paths; they should not be served by the reverse proxy.

Choose `local` or `s3` for managed data. The S3 backend requires bucket and region, a private local staging/cache directory, and either ambient credentials or a complete key pair. Enable bucket versioning and native backup/replication because instance backups do not contain authoritative S3 objects.

Set upload size, file-count, free-space, session TTL, and garbage-collection limits according to actual capacity. The revision size limit must be at least the single-file limit.

## Query and refresh capacity

Configure separate read and write concurrency, queue lengths, and timeouts. Start conservatively:

- interactive reads should fail predictably rather than exhaust the host;
- refresh writes should remain limited because they consume memory, CPU, temporary space, and catalog write capacity;
- queue timeouts should be shorter than upstream request timeouts;
- abandoned-job lease timeout should be long enough for expected scheduler pauses but short enough for recovery.

Query cache entry and byte limits are per semantic-model runtime boundaries. Monitor hit rate and memory before increasing them.

## Operational endpoints

Configure the readiness URL used by `leapview healthcheck` and protect `/metrics` with the metrics bearer token. Restrict metrics network access as well as authenticating it. Logs should be collected from standard process output by the deployment platform.

## Final checklist

Before exposing traffic:

1. `leapview config validate --production` succeeds.
2. TLS, allowed hosts, secure cookies, and callback URLs match the public address.
3. Persistent paths and external stores are writable and backed up.
4. Authentication works without development bypass.
5. Metrics require the intended token and are not publicly browsable.
6. Readiness fails when required persistent dependencies are unavailable.
7. A backup and isolated restore have been tested.
8. Query and refresh limits fit host capacity.
9. MCP OAuth discovery uses the intended deployment origin and `/mcp` rejects general API tokens.

Use the generated [environment variable reference](/docs/configuration) as the source of truth; it is generated from the runtime configuration specification and includes every cross-field relationship.
