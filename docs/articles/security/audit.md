# Audit events

Audit events record security-sensitive and administrative activity with acting principal and workspace or resource context. Query events separately record governed data operations and their execution context. Use both when an incident crosses authorization and analytical activity.

## What to investigate

Security and administrative audit history is useful for:

- principal, service-principal, group, and membership changes;
- role bindings, grants, ownership, and data-policy changes;
- local-user creation and password reset;
- token or service-principal secret issuance and revocation;
- project deployment and activation;
- refresh and managed-data operations;
- backup, restore, maintenance, and storage cleanup;
- agent and API actions that emit audited operations.

Not every request is an administrative audit event. Query events provide filters for principal, surface, operation, kind, semantic model, target, status, text search, and time range.

## Query the API

The generated [Audit API](/docs/api/audit) exposes workspace-scoped endpoints:

```text
GET /api/v1/workspaces/{workspace}/audit-events
GET /api/v1/workspaces/{workspace}/query-events
```

Use bounded time ranges and pagination. Filter by actor/action/target for administrative changes or principal/surface/operation/status for queries. Record the request time and page tokens when exporting an investigation set so the collection process can be repeated.

Audit access requires its own privilege. Restrict it to security and operational roles that need the relevant workspace visibility. Query text and target metadata can reveal sensitive business context even when row data is absent.

## Correlate sources

For authentication incidents, correlate LeapView audit/application logs with identity-provider sign-in events, SCIM provider logs, reverse-proxy request IDs, and secret-manager access history.

For data or deployment incidents, correlate project commit, deployment ID, environment, managed revision digest, refresh generation, active serving state, and query request identity. Preserve timestamps in a consistent timezone.

Audit records should not contain bearer tokens, passwords, raw OIDC tokens, or secret values. If an external log captured a secret, treat the logging system as part of the exposure.

## Retention

Use policy-driven bounded retention. The maintenance command defaults to separate windows for audit, query, auth-state, and archived agent-conversation history and runs as a dry-run unless `--apply` is supplied:

```sh
leapview admin maintenance \
  --audit-days 365 \
  --query-days 90
```

Review the dry-run output, preservation requirements, and external archive before applying deletion. A value of zero disables pruning for that category; it does not automatically satisfy storage or compliance needs.

Export or forward events to an approved security system when organizational retention exceeds the operational database window. Protect integrity and access to the export.

## Incident workflow

1. Define actor, resource, action, and time window.
2. Preserve relevant audit, query, application, proxy, and provider records.
3. Build a chronological timeline using stable IDs.
4. Determine effective privileges at the time where possible.
5. Contain credentials or access without deleting evidence.
6. Correct grants, policy, project, or runtime state through normal workflows.
7. Record findings and validate that new detection covers recurrence.

Audit history supports accountability but does not replace least privilege, secure credential handling, or external monitoring.

## Verify audit coverage

Exercise a representative administrative change and a governed query in a non-production workspace. Confirm that the audit and query APIs record the expected principal, action, resource, status, and correlation identifiers, and verify that secret values are absent from every emitted record.
