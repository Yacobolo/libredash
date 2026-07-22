# LeapView Public API Target Architecture

## Status

This document defines the pre-release target for LeapView's public API. Breaking changes are allowed until the product is released. The target favors correctness, simplicity, robustness, scalability, and long-term maintainability over compatibility or implementation cost.

The API is complete only when its runtime behavior, generated contract, authorization model, persistence semantics, and integration tests agree. Route presence alone is not completion.

## Design Principles

1. A release deterministically identifies everything that can be deployed.
2. Reads and pagination use a server-owned consistency version appropriate to the resource.
3. Retriable mutations are durable and cannot duplicate effects.
4. Authorization is enforced consistently on collections, exact resources, and streams.
5. TypeSpec and runtime behavior cannot silently diverge.
6. Shared infrastructure standardizes transport concerns without erasing domain-specific state machines.
7. Dynamic data is allowed only behind explicit extension boundaries.

## Scope and Runtime Model

- Expose one coherent headless surface under `/api/v1`.
- Keep Datastar page streams, browser commands, sessions, and CSRF-protected administration private.
- Each LeapView instance serves one configured environment. Runtime product paths do not contain an environment segment.
- Authored YAML remains the only project, dashboard, and semantic-model authoring mechanism.
- Workspace IDs are globally unique within an instance.
- Synchronous queries are bounded. Bulk export jobs are outside this version.
- No compatibility aliases, legacy persistence backfills, or historical deployment migration are required before release.

## Resource Model

LeapView uses three principal hierarchies:

- Project → immutable release → deployment
- Project → managed connection → immutable revision or upload session
- Workspace → dashboards, semantic models, queries, access, refreshes, and agent conversations

Projects are materialized from releases and managed-data activity. They are discoverable but cannot be authored through the public API.

## Canonical Surface

### Discovery and system

- `GET /api/openapi.json`
- `GET /api/docs`
- `GET /api/v1/capabilities`
- `/healthz`, `/readyz`, and `/metrics` remain outside the product API.

Capabilities report the API version, build version, configured authentication modes, query formats, enabled upload protocols, and registered visual shapes. Capabilities must be derived from active configuration and registries rather than hard-coded assumptions.

Every `/api/v1` request requires bearer authentication, including local development. Development may use a configured static bearer credential, but it must not bypass the public authentication path.

### Projects, releases, and deployments

Project discovery is read-only:

- `GET /projects`
- `GET /projects/{project}`
- `GET /projects/{project}/workspaces`
- `GET /projects/{project}/connections`
- `GET /projects/{project}/connections/{connection}`

Release lifecycle:

- `POST/GET /projects/{project}/releases`
- `GET /projects/{project}/releases/{release}`
- `PUT /projects/{project}/releases/{release}/workspaces/{workspace}/artifact`
- `POST /projects/{project}/releases/{release}/finalize`
- `GET /projects/{project}/releases/{release}/events`

A release manifest contains:

- The project digest.
- The exact expected workspace artifact digest for every workspace.
- The exact managed-data revision digest for every referenced connection.

Artifact upload requires `application/octet-stream` and `Content-Digest`. Finalization validates project identity, artifact completeness and digests, globally unique workspace identities, and exact managed-data pins for every artifact. The finalization operation is safe to retry.

Release states are `draft`, `validating`, `ready`, and `failed`. Once finalization starts, release contents are immutable. A failed release is retained for audit; corrections require a new release.

Deployment lifecycle:

- `POST/GET /projects/{project}/deployments`
- `GET /projects/{project}/deployments/{deployment}`
- `GET /projects/{project}/deployments/{deployment}/events`
- `POST /projects/{project}/deployments/{deployment}/cancel`
- `POST /projects/{project}/deployments/{deployment}/rollback`

A deployment accepts only a ready release. Cutover atomically switches all workspace serving states and managed connection revisions. Cancellation is allowed only before cutover and is safe to retry. Rollback creates a new audited deployment targeting a selected prior release. There is no separate activation endpoint.

### Managed data

Managed-data discovery and lifecycle are nested under a project connection:

- `GET .../revisions`
- `GET .../revisions/{revision}`
- `GET .../active-revision`
- `POST/GET .../upload-sessions`
- `GET .../upload-sessions/{session}`
- `POST .../{session}/finalize`
- `POST .../{session}/cancel`
- `GET .../{session}/events`

Revisions are immutable and content-addressed. Upload sessions negotiate an enabled transport without exposing credentials or resolved secrets.

TUS and S3 multipart are separate protocol modules in TypeSpec and public documentation:

- TUS remains a protocol endpoint under `/upload-protocols/tus`.
- Authenticated S3 multipart commands may remain nested beneath the upload session in `/api/v1`.
- Upload-session negotiation links to the applicable protocol contract and endpoints.

The route location is less important than keeping product lifecycle semantics separate from transport-protocol semantics.

### Workspace and BI APIs

- `GET /workspaces`
- `GET /workspaces/{workspace}`
- Keep workspace assets, asset lineage, search, audit, and query-history collections.

Dashboard metadata is normalized:

- Dashboard descriptions contain page summaries.
- `GET .../pages/{page}` returns the complete typed layout.
- Page components use a discriminated union for visual, table, and filter placements.
- Visual, table, and filter resources have symmetric individual `GET` descriptions.
- There is no separate component-list endpoint.

Canonical page query routes:

- `POST .../pages/{page}/query`
- `POST .../pages/{page}/visuals/{visual}/query`
- `POST .../pages/{page}/tables/{table}/query`
- `POST .../pages/{page}/filters/{filter}/values`

Semantic-model discovery includes datasets, fields, sources, and relationships. Retain model-level query/explain and dataset preview/explain. Do not expose redundant dataset query operations.

Visual responses are renderer-neutral discriminated unions. Every built-in shape has an explicit schema. Plugin-specific payloads and options are allowed only within a namespaced `extensions` object. Renderer-specific records must not leak into unrestricted top-level response fields.

### Refreshes and agent runs

Refresh routes provide list, create, get, events, and cancel. Retrying a refresh creates a new run with `retryOf` referencing the prior run.

Agent execution is asynchronous:

- `POST .../conversations/{conversation}/runs` returns `202` immediately.
- Runs expose status, messages, paginated events, resumable SSE, and cancellation.
- Synchronous public turn endpoints do not exist.

Releases, deployments, upload finalization, refreshes, and agent runs share durable job leasing and a common outer event envelope. They retain domain-specific resources, states, progress payloads, errors, and cancellation rules.

The common event envelope contains:

- A resource-scoped ordered event ID.
- Event type.
- Resource type and resource ID.
- Creation timestamp.
- Optional progress and structured error.
- Domain-specific `data`.

Agent events may have agent-specific data, but they use the same outer envelope and persistence abstraction as other asynchronous resources.

### Access administration

Access resources expose lifecycle-complete operations rather than artificial CRUD symmetry:

- Principals, service principals, groups, role bindings, grants, and data policies support the mutable operations appropriate to their lifecycle.
- Roles are read-only.
- Service-principal secrets support create, list/get metadata, and revoke. Secret values cannot be read or updated after creation.
- Workspace role assignments remain distinct from object-level privilege grants.
- Mutable `PATCH` operations require `If-Match`.
- Batch authorization checks are available for explicit client decisions.
- One-time secret responses use `Cache-Control: no-store`.

List operations remove inaccessible entries before pagination. Exact inaccessible resource IDs return `404`. Collection-level authorization failures return `403`. Graph and lineage responses exclude inaccessible nodes and any edges connected to them.

## Contract and Protocol Rules

### TypeSpec and generation

- TypeSpec defines every public `/api/v1` operation.
- Generate OpenAPI, Go dispatch and models, CLI commands, and agent tools from annotated TypeSpec operations.
- Manually registered `/api/v1` routes are forbidden by architecture tests.
- Generated artifacts must be reproducible and clean after generation.

Strictness applies where it protects interoperability:

- Mutation and query request bodies reject unknown fields.
- IDs, pagination fields, protocol fields, and security-sensitive collections have meaningful bounds.
- Core response shapes use explicit models, enums, and discriminated unions.
- Arbitrary records are allowed only for documented metadata or namespaced extension payloads.
- Do not add arbitrary limits or closed enums to naturally extensible descriptive text.

### Errors and HTTP semantics

All public API and upload-protocol errors, including middleware failures, use RFC 9457 `application/problem+json` with:

- `type`, `title`, `status`, `detail`, and `instance`.
- Stable `code`.
- Generated or propagated `requestId`.
- Field violations where applicable.

Use HTTP semantics consistently:

- `201 + Location` for created resources.
- `202 + Location` for accepted asynchronous work.
- `204` for successful bodyless deletion or revocation.
- `304` for conditional reads.
- `404` for missing or concealed exact resources.
- `409` for state conflicts or unavailable consistency versions.
- `412` for failed preconditions.
- `413` for oversized bodies, including streamed bodies.
- `415` for media mismatch.
- `422` for semantically invalid well-formed input.

### Idempotency

Idempotency records are durable and scoped to the authenticated caller, operation, normalized request, and key. They store the original response and request digest.

- Require `Idempotency-Key` for resource creation and commands that can duplicate effects.
- Reuse with the same digest returns the original response.
- Reuse with another digest returns `409`.
- Resource state transitions such as finalize and cancel must also be inherently safe to retry.
- Async resource creation, its idempotency record, and its durable job must commit atomically.

### ETags and caching

- Mutable resources and serving-state-derived metadata use strong ETags.
- Mutable `PATCH` requires `If-Match`.
- Immutable releases, artifacts, and revisions support long-lived conditional caching.
- Queries, upload negotiation containing ephemeral URLs, and secret responses use `no-store`.

### Pagination and consistency

Use keyset pagination for collections that can grow independently:

- Projects, releases, deployments, connections, revisions, upload sessions, workspaces, assets, principals, groups, bindings, grants, policies, refresh runs, conversations, messages, runs, audit records, and events.

Small immutable or manifest-bounded child collections may be returned whole, including dashboard page summaries, page layout components, roles, and fixed capability enums.

Defaults and limits:

- Standard growing collections default to 50 and cap at 200.
- Query pages default to 100 and cap at 1,000.

Cursors are opaque, signed, bound to the normalized request, and expire explicitly. They bind to the consistency version appropriate to the collection:

- Serving-state-derived metadata and queries bind to the serving-state ID.
- Release children bind to the immutable release digest.
- Mutable administrative collections bind to an access/catalog revision when stable pagination requires it.
- Event cursors bind to resource identity and the last persisted event ID.

Cursor signing uses a configured, durable, rotatable key with a key identifier. Process restart must not invalidate otherwise valid cursors. Invalid or expired cursors return `400`; unavailable consistency versions return `409`.

Filtering for authorization happens before pagination.

### Query representations

- JSON rowsets use typed column descriptors and positional rows.
- `int64` and decimal values are strings in JSON to prevent precision loss.
- Row-oriented semantic and table queries support `application/json` and `application/vnd.apache.arrow.stream`.
- Visual queries remain JSON.
- Arrow responses include query ID, serving snapshot, and next cursor in response headers and schema metadata.
- JSON and Arrow representations of the same query are value-equivalent.

### Event history and SSE

`GET {async-resource}/events` returns repository-backed keyset-paginated JSON by default and resumable SSE for `Accept: text/event-stream`.

- Support `Last-Event-ID` and ordered resource-scoped IDs.
- Emit 15-second heartbeats.
- Replay persisted events after reconnect.
- Close after a terminal event.
- Authorize every reconnect.
- Re-evaluate authorization periodically for long-lived streams or enforce a bounded stream lifetime that requires reauthorization.
- Page and stream directly from persistent event storage; do not synthesize history from current resource state or load a fixed maximum into memory.

## Implementation Plan

### Phase 1: Contract and security invariants

1. Make bearer authentication unconditional for `/api/v1`, including development credentials.
2. Finish exact-object concealment and pre-pagination filtering for graph, lineage, and edge responses.
3. Complete typed dashboard page component unions and remove unrestricted top-level visual records.
4. Split upload protocol definitions into dedicated TypeSpec modules while retaining negotiated links.

### Phase 2: Durable retry and consistency infrastructure

1. Persist idempotency records and atomically couple them to resource/job creation.
2. Replace process-random cursor keys with configured rotatable signing keys.
3. Bind each cursor to its resource-appropriate consistency version.
4. Map streamed body-limit errors to `413` consistently.

### Phase 3: Unified durable events

1. Use one persistent outer event envelope for all asynchronous resources.
2. Migrate agent run events to the shared envelope and persistence abstraction.
3. Page and stream events directly from storage beyond 200 records.
4. Remove synthesized event-history helpers.
5. Add periodic authorization enforcement or bounded lifetimes for live SSE streams.

### Phase 4: Contract/runtime alignment

1. Audit every TypeSpec operation against runtime status codes, media types, headers, bounds, and authentication.
2. Enforce unknown-field rejection for every public mutation and query input.
3. Ensure generated CLI and agent tools expose only explicitly annotated public operations.
4. Verify capabilities against actual configured services and registries.
5. Remove any remaining public legacy or private browser routes.

### Phase 5: Verification and bootstrap

1. Add semantic contract checks across every operation.
2. Add selected golden schemas for critical release, query, visual, event, and access contracts.
3. Complete behavioral integration coverage described below.
4. Rebuild pre-release deployment and managed-data state through bootstrap and redeployment.
5. Run `task ci` and require a clean generated-artifact and worktree check.

## Verification Plan

### Contract verification

- Every public operation is generated from TypeSpec.
- Route inventory proves required routes exist and legacy/private routes do not.
- Semantic checks cover documented responses, media types, authentication, authorization annotations, required headers, enums, and bounds.
- Critical schemas use focused golden tests rather than one monolithic byte-for-byte OpenAPI snapshot.
- Generated CLI and agent-tool registries are checked for drift.

### Middleware verification

- Request-ID generation and propagation.
- RFC 9457 envelopes for authentication, authorization, host rejection, rate limiting, panic recovery, body limits, content negotiation, and validation.
- Durable idempotency replay, concurrent replay, digest conflict, and restart recovery.
- Conditional reads and `If-Match` preconditions.
- Rate-limit headers and secret redaction.

### Release and deployment verification

- Project and artifact digest verification.
- Missing, duplicate, and unexpected artifacts.
- Manifest/artifact managed-revision disagreement.
- Immutable finalization, concurrent finalization, validation failure, and correction through a new release.
- Atomic multi-workspace cutover and connection-pointer consistency.
- Cancellation before cutover, failure rollback, explicit rollback, and concurrent deployment serialization.

### Query and pagination verification

- JSON/Arrow value parity and type encoding.
- Stable pagination across the same consistency version.
- `409` after the relevant serving or catalog version becomes unavailable.
- Cursor tampering, expiry, request mismatch, signing-key rotation, and restart survival.
- Authorization, row limits, and renderer-neutral visual contracts.

### Event and async verification

- Durable claim, lease renewal, reclaim after restart, completion, failure, and cancellation.
- Event history beyond 200 records.
- Ordered SSE replay and `Last-Event-ID` reconnect.
- Observable 15-second heartbeat behavior.
- Terminal closure and cancellation events.
- Authorization revocation and reconnect behavior.

### Access verification

- Filtering before pagination for every user-visible collection, graph, and lineage response.
- Exact-object `404` concealment and collection-level `403` behavior.
- One-time secret delivery and `no-store` caching.
- Optimistic concurrency and audit event completeness.
- Batch authorization decisions and inherited grant explanations.

## Definition of Done

The target is complete when:

- The public route inventory matches this document with no compatibility aliases.
- Release identity fully determines workspace artifacts and managed-data revisions.
- Deployment cutover is atomic and auditable.
- Public authentication, authorization, idempotency, cursor signing, and async work survive process restart without weakening guarantees.
- TypeSpec, generated artifacts, and runtime behavior agree.
- All growing collections have scalable, consistency-aware pagination and authorization filtering.
- All asynchronous resources expose durable, replayable event history through the common envelope.
- The verification plan passes under `task ci` with no generated or worktree drift.
