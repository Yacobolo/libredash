# Integrate with LibreDash

LibreDash exposes CLI, HTTP API, and workspace-agent surfaces backed by the same active projects, authorization, data policies, and semantic contracts. Choose the highest-level stable operation that fits the task instead of reconstructing internal workflows from low-level calls.

## CLI

Use the CLI for human-operated delivery, CI pipelines, managed-data synchronization, administrative maintenance, and quick headless queries. It provides opinionated workflows such as validate, plan, deploy, data sync, backup, and restore as well as generated access to API operations.

The CLI is preferable when:

- an operator needs readable diagnostics or an approval prompt;
- a pipeline should follow the supported atomic deployment sequence;
- local project files or managed-data directories are inputs;
- shell composition is sufficient and a maintained SDK would add little value.

Use `--json` where a command supports machine output. Do not parse human-readable tables as a compatibility contract.

## API

Use the versioned HTTP API for application integrations, custom portals, catalog discovery, BI queries, deployment automation, access administration, audit export, refresh control, and agent conversations.

The downloadable OpenAPI document describes paths and request/response schemas. Generate a client where that improves type safety, but retain explicit handling for authentication, pagination, rate limiting, conflicts, and server errors.

The API is the right boundary when an application needs long-lived programmatic integration or when direct HTTP status and payload control matter.

## Agent

Workspace agent conversations provide governed natural-language exploration through an explicitly allowed tool catalog. The agent works inside workspace authorization and semantic boundaries. It is not a way to bypass modeling or expose unrestricted SQL.

Use the agent when a user benefits from iterative question/answer and can validate results against delivered evidence. Use deterministic BI endpoints for scheduled reporting, financial controls, or other workflows where a natural-language interpretation would be an unnecessary source of variability.

## Identity and scope

Every integration should have an attributable principal and the narrowest useful privileges. Create separate service principals for deployment, data publishing, read-only BI, and agent workloads. Store credentials in the deployment or CI secret manager.

Token access is limited by principal privileges and any token scope or allowlist. A `403` should prompt an effective-privilege review, not replacement with an owner token.

## Reliability contract

Design integrations to:

- send bounded list and query requests;
- treat page tokens as opaque;
- distinguish retryable failures from invalid or unauthorized requests;
- use idempotent retries only where the operation allows them;
- persist deployment, revision, refresh, conversation, and run IDs needed for correlation;
- avoid logging tokens or sensitive analytical payloads;
- tolerate active deployment changes by rediscovering resource metadata when needed.

Prefer `libredash deploy` over manually creating and activating deployment candidates. Prefer dashboard or semantic query endpoints over building SQL from catalog metadata.

Start with [API quickstart](/docs/guides/integrate/api-quickstart), [Headless BI queries](/docs/guides/integrate/headless-bi), or [Agent integrations](/docs/guides/integrate/agent).
