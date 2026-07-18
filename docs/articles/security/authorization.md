# Roles, grants, and policies

LibreDash authorization assigns privileges on securable resources to principals, service principals, and groups. Authentication and provisioning establish identities; the grant engine determines what those identities may do.

## Securable hierarchy

Securable objects include workspaces, dashboards, semantic models, sources, model tables, agent policies, datasets, tables, and columns. Objects participate in a parent hierarchy, so effective access may include inherited privileges as well as direct grants.

Review the effective privilege result rather than assuming a direct binding is the only source of access. The Current User and Access APIs expose effective-privilege views for this purpose.

## Workspace roles

Workspace role bindings apply reusable privilege sets such as viewer, member, editor, contributor, deployer, admin, or owner. Bind a stable group wherever access follows team membership:

```yaml
apiVersion: libredash.dev/v1
kind: WorkspaceRoleBinding
metadata:
  workspace: sales
  name: analysts-viewer
spec:
  role: viewer
  subject:
    kind: group
    group: analysts
```

Roles express common responsibilities. Owners and grant managers should be rare; routine project deployment should use a dedicated deployer identity rather than an owner token.

## Explicit grants

Use a Grant when one subject needs one privilege on a specific securable object outside the standard role shape:

```yaml
apiVersion: libredash.dev/v1
kind: Grant
metadata:
  workspace: sales
  name: analysts-audit-view
spec:
  object:
    type: workspace
    id: sales
  subject:
    kind: group
    group: analysts
  privilege: VIEW_AUDIT
```

Choose the narrowest object and privilege that supports the task. Avoid accumulating one-off direct user grants; they are harder to review and can survive team changes.

## Data policies

Data policies constrain analytical access beyond navigation or query permission. A row-filter policy limits eligible records; a column-mask policy changes exposure of a protected column. Policies target a securable object and may target a subject.

Policy expressions are part of the governed server query boundary. Apply them consistently to browser dashboards, headless API queries, agent tools, preview, and other data surfaces. Do not rely on hiding a dashboard component or browser column as a security control.

Test policies with representative users, including aggregates. Row filtering can change totals, distinct counts, relationship behavior, and whether an empty result is expected. Column masking must preserve safe types and must not leak the raw value through labels, tooltips, exports, or alternative datasets.

## Owners and administration

Ownership and platform administration are distinct from ordinary workspace use. Keep platform-wide `MANAGE_PLATFORM`, workspace `MANAGE_GRANTS`, deployment, refresh, query, and view privileges separated according to operational responsibility.

A service principal used by CI should receive only the target environments, workspaces, and deployment/data privileges required by that pipeline. A read-only integration should not inherit project activation or grant management.

## Review access

Use this periodic review:

1. List active principals, service principals, and groups.
2. Reconcile SCIM membership and local groups.
3. Inspect effective privileges for sensitive workspaces.
4. Find direct grants that duplicate or exceed role access.
5. Review owner, admin, deployer, and grant-manager assignments.
6. Review data policies against current semantic fields.
7. Remove or deactivate obsolete identities and revoke credentials.
8. Audit every binding, policy, and ownership change.

Validate project access resources before deployment and test with a non-owner principal afterward. See [Workspace Role Binding](/docs/config/workspace-role-binding), [Grant](/docs/config/grant), [Data Policy](/docs/config/data-policy), and the [Access API](/docs/api/access).
