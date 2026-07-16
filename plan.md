# LibreDash Ideal API Redesign

## Summary

Replace the current API with one coherent `/api/v1` headless surface while keeping Datastar page streams and browser commands private. Each LibreDash instance serves one configured environment (`dev` locally, `prod` when deployed), so runtime paths remain environment-free.

Use three principal resource hierarchies:

- Project → immutable release → deployment
- Project → managed connection → revision/upload session
- Workspace → dashboards, semantic models, queries, access, refreshes, and agent conversations

Implement every slice with red-green-refactor TDD. Remove legacy endpoints without compatibility aliases or data backfills.

## Canonical API Surface

### Discovery and system

- Serve `GET /api/openapi.json`, `GET /api/docs`, and `GET /api/v1/capabilities`.
- Keep `/healthz`, `/readyz`, and `/metrics` operational and outside the product API.
- Capabilities report API/build versions, enabled authentication modes, query formats, upload protocols, and plugin visual shapes.
- Public `/api/v1` accepts bearer credentials only; browser sessions and CSRF remain private UI concerns.

### Projects, releases, and deployments

- Expose read-only project discovery through `GET /projects`, `GET /projects/{project}`, and nested workspace/connection lists. Projects are materialized from uploaded releases and managed-data activity; they cannot be authored through the API.
- Replace deployment candidates with releases:
  - `POST/GET /projects/{project}/releases`
  - `GET /projects/{project}/releases/{release}`
  - `PUT /projects/{project}/releases/{release}/workspaces/{workspace}/artifact`
  - `POST /projects/{project}/releases/{release}/finalize`
  - `GET /projects/{project}/releases/{release}/events`
- A release manifest contains the project digest, expected workspace artifact digests, and pinned managed-data revisions. Artifact upload requires `application/octet-stream` and `Content-Digest`.
- Release states are `draft`, `validating`, `ready`, and `failed`. Finalization makes the release immutable; correcting a failed release requires creating another release.
- Deploy a ready release through:
  - `POST/GET /projects/{project}/deployments`
  - `GET /projects/{project}/deployments/{deployment}`
  - `GET /projects/{project}/deployments/{deployment}/events`
  - `POST .../{deployment}/cancel`
  - `POST .../{deployment}/rollback`
- Deployment automatically validates readiness and atomically switches all workspace serving states and connection revisions. There is no separate activation endpoint. Rollback creates a new audited deployment targeting the selected prior release.

### Managed data

- Use `/projects/{project}/connections` for connection discovery and description.
- Nest revisions and upload sessions below each connection:
  - `GET .../revisions`, `GET .../revisions/{revision}`, `GET .../active-revision`
  - `POST/GET .../upload-sessions`, `GET .../upload-sessions/{session}`
  - `POST .../{session}/finalize`, `POST .../{session}/cancel`, `GET .../{session}/events`
- Retain TUS and S3 multipart upload protocols, but document them as separate protocol contracts linked from upload-session negotiation.
- Never expose credentials or resolved secrets.

### Runtime workspace and BI APIs

- Add `GET /workspaces/{workspace}` and keep accessible-workspace, asset, lineage, search, audit, and query-history collections.
- Require workspace IDs to be globally unique within an instance.
- Normalize dashboard metadata:
  - Dashboard description returns page summaries.
  - `GET .../pages/{page}` returns the complete typed layout.
  - Provide symmetric `GET` descriptions for page visuals, tables, and filters.
  - Remove the separate component-list endpoint and both legacy table-query variants.
- Canonical page query routes are:
  - `POST .../pages/{page}/query`
  - `POST .../pages/{page}/visuals/{visual}/query`
  - `POST .../pages/{page}/tables/{table}/query`
  - `POST .../pages/{page}/filters/{filter}/values`
- Semantic-model discovery includes datasets, fields, sources, and relationships. Retain model-level query/explain and dataset preview/explain; remove redundant dataset query operations.
- Visual responses remain renderer-neutral tagged unions. Built-in shapes receive explicit schemas; plugin-specific data belongs in namespaced `extensions`, not unrestricted top-level records.

### Refresh and agent runs

- Refresh routes provide list/create/get/events/cancel. Retrying creates a new run with `retryOf`.
- Replace synchronous agent turns with `POST .../conversations/{conversation}/runs`, returning `202` immediately.
- Agent runs expose status, messages, paginated event history, resumable SSE events, and cancellation.
- Releases, deployments, upload finalization, refreshes, and agent runs share timestamps, progress, structured errors, cancellation rules, and event-envelope conventions while retaining domain-specific resources.

### Access administration

- Complete CRUD symmetry for principals, service principals, service-principal secrets, groups, role bindings, grants, and data policies.
- Keep roles read-only and distinguish workspace role assignments from object-level privilege grants.
- Add individual `GET` endpoints and require `If-Match` for mutable `PATCH` operations.
- Provide a batch authorization-check endpoint for clients needing explicit decisions.
- Preserve one-time secret delivery with `Cache-Control: no-store`.

## Contract and Protocol Rules

- Define the entire public surface in TypeSpec and generate OpenAPI, Go dispatch/models, CLI commands, and agent tools from it. No manually registered `/api/v1` routes are permitted.
- Use RFC 9457 `application/problem+json` errors with `type`, `title`, `status`, `detail`, `instance`, stable `code`, generated `requestId`, and field violations. Middleware errors must use the same envelope.
- Use correct HTTP semantics: `201 + Location` for created resources, `202 + Location` for async work, `204` for bodyless deletion, `304` for conditional reads, `412` for failed preconditions, `415` for media mismatch, and `422` for semantic validation.
- Require `Idempotency-Key` for non-query POST mutations, including async creation and command endpoints. Reuse returns the original response; reuse with a different request digest returns `409`.
- Use strong ETags for mutable resources and serving-state-derived metadata. Queries and secret responses are `no-store`; immutable releases and revisions support conditional caching.
- Standard list pagination is keyset-based with default 50 and maximum 200. Query pages default to 100 and cap at 1,000.
- Cursors are opaque, signed, bound to the normalized request and serving snapshot, and expire explicitly. Invalid cursors return `400`; unavailable snapshots return `409`.
- JSON rowsets use typed column descriptors plus positional rows. `int64` and decimal cells are strings to avoid precision loss.
- Row-oriented semantic and table queries support `application/json` and `application/vnd.apache.arrow.stream`; visual queries remain JSON. Arrow responses carry query, snapshot, and next-cursor metadata in schema metadata and response headers.
- `GET {async-resource}/events` returns paginated JSON normally and resumable SSE for `Accept: text/event-stream`. Support `Last-Event-ID`, ordered event IDs, 15-second heartbeats, terminal events, and authorization on reconnect.
- Use explicit enums, bounded strings and collections, strict unknown-field rejection, and discriminated unions throughout the contract.

## Implementation Sequence

1. Write failing contract tests for the new route inventory, HTTP semantics, schemas, authentication declarations, and absence of private/legacy routes.
2. Replace candidate persistence with release, release-artifact, and revised deployment records; update atomic deployment coordination to activate a ready release in one operation.
3. Build centralized API middleware for request IDs, Problem Details, content negotiation, idempotency, ETags, rate-limit headers, and media/body validation.
4. Implement project/release/deployment and managed-data lifecycle handlers, followed by workspace metadata and BI query handlers.
5. Add shared asynchronous event infrastructure and migrate refresh, upload finalization, deployment, and agent execution to it.
6. Complete access-control resource symmetry and exact object authorization, filtering inaccessible list entries and returning `404` for inaccessible object IDs.
7. Regenerate CLI and agent-tool surfaces, update public documentation, then remove unscoped agent routes, candidate APIs, activation APIs, duplicate table queries, and all other legacy handlers.
8. Rebuild pre-release deployment/managed-data state through bootstrap and redeployment; do not add compatibility routes or historical backfills.

## Test Plan

- Contract snapshots verify every route/method, response, media type, auth rule, enum, bound, and generated client surface.
- Middleware tests cover every documented error status, request-ID propagation, Problem Details, idempotency replay/conflict, ETag preconditions, content negotiation, and rate-limit headers.
- Release tests cover digest verification, missing/duplicate artifacts, immutable finalization, validation failure, concurrent finalization, and retry through a new release.
- Deployment integration tests cover atomic multi-workspace cutover, revision-pointer consistency, cancellation before cutover, failure rollback, explicit rollback, and concurrent deployments.
- Query tests prove JSON/Arrow value parity, snapshot-stable pagination, cursor tampering/expiry, authorization, row limits, type encoding, and renderer-neutral visual contracts.
- SSE tests cover ordered replay, reconnect with `Last-Event-ID`, heartbeat behavior, terminal closure, cancellation, and permission revocation.
- Access tests cover list filtering, exact-object authorization, secret non-caching, optimistic concurrency, and audit events.
- Finish with `task ci` and a clean generated-artifact check.

## Assumptions

- One server instance exposes one configured serving environment.
- Authored YAML remains the only project/dashboard/model authoring mechanism.
- The first-party Datastar UI transport remains private and may call internal services directly.
- Synchronous queries are bounded; bulk export jobs are deferred to a later API addition.
- No API, route, or persisted deployment-history compatibility is required before release.

