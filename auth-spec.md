# LibreDash Auth To-Be Architecture

LibreDash should use a proven hybrid model:

- **Product model:** Microsoft Fabric / Power BI.
- **Authorization mechanics:** Databricks Unity Catalog.

Fabric is the mental model users should see: workspaces, items, semantic models,
reports, dashboards, sharing, viewers, contributors, members, and admins.

Unity Catalog is the model LibreDash should implement internally: principals,
securable objects, ownership, grants, inherited privileges, service principals,
and data-level policies.

## Enterprise Auth Model

LibreDash should follow the standard enterprise split of responsibility:

```text
OIDC = interactive human login
SCIM = enterprise user and group provisioning
Grant engine = LibreDash authorization
Service principals = non-human workload identity
API tokens = scoped credentials, not identities
```

OIDC proves who the user is. SCIM keeps users, groups, and memberships in sync.
The LibreDash grant engine remains the source of truth for product access.
Service principals receive grants like any other principal. API tokens only
constrain an existing principal's access through workspace scope and privilege
allowlists.

## Goals

- Make authorization explicit, inspectable, and testable.
- Treat workspace boundaries as security boundaries.
- Use grants on securable objects as the source of truth.
- Treat roles as convenience bundles over grants, not the primitive authorization
  model.
- Support users, groups, and service principals uniformly.
- Keep authentication separate from authorization.
- Enforce access at every boundary: route, service, repository, query planner,
  agent tool, and data execution.

## Non-Goals

- Do not invent a LibreDash-specific RBAC theory.
- Do not rely on UI hiding as authorization.
- Do not treat API tokens as independent identities.
- Do not allow workspace-wide read access to imply unrestricted raw data access.

## Principal Model

Principals are actors that can receive grants.

- `user`: a human authenticated by an identity provider.
- `group`: an identity-provider or LibreDash-managed group.
- `service_principal`: an automation identity for API and publish workflows.

External identities map to stable LibreDash principal IDs. Email is display
metadata, not identity.

Groups should be synchronized from the identity provider when available. Local
groups are allowed for file-backed/demo projects but should use the same grant
model.

## Authentication

LibreDash should support:

- Browser SSO through OIDC/OAuth.
- API authentication through OAuth client credentials for service principals.
- Short-lived sessions for browser users.
- Scoped API tokens only as a compatibility/bootstrap mechanism.

Authentication returns a principal. It must not directly grant privileges.

## Securable Objects

All authorizable resources are securable objects.

```text
platform
  workspace
    item
      dashboard
      semantic_model
      source
      model_table
      agent_policy
    data_object
      dataset
      table
      column
```

Every securable object has:

- `id`
- `type`
- `parent_id`
- `owner_principal_id`
- `workspace_id`

Ownership grants full control over the object and delegated grant management
unless explicitly transferred.

## Privileges

Privileges are the primitive authorization units.

Core privileges:

- `USE_WORKSPACE`: enter and list a workspace.
- `VIEW_ITEM`: read item metadata and definitions.
- `EDIT_ITEM`: modify authored item definitions.
- `MANAGE_ITEM`: delete, move, rename, share, or transfer ownership.
- `QUERY_DATA`: execute semantic/data queries.
- `PREVIEW_DATA`: inspect row-level/raw data previews.
- `REFRESH_DATA`: run materialization or cache refresh jobs.
- `DEPLOY`: create publishes, upload artifacts, and validate serving-state
  candidates.
- `ACTIVATE_PUBLISH`: activate publishes or roll back active serving state.
- `USE_AGENT`: run agent turns.
- `VIEW_AGENT`: read agent conversations and runs.
- `MANAGE_GRANTS`: grant and revoke privileges.
- `VIEW_AUDIT`: read audit and query events.
- `MANAGE_WORKSPACE`: administer workspace settings.
- `MANAGE_PLATFORM`: administer platform-wide settings.

Privileges may be granted directly to users, groups, or service principals.

## Inheritance

Privileges inherit downward unless a privilege explicitly says otherwise.

```text
workspace grant -> items and data objects in that workspace
semantic_model grant -> datasets/tables/columns in that model
table grant -> columns in that table
```

Inheritance must be deterministic and inspectable. LibreDash should support a
`show grants`-style API that explains direct, inherited, and ownership-derived
access.

## Workspace Roles

Workspace roles are Fabric-style bundles over grants.

Default mapping:

| Role | Grants |
| --- | --- |
| `viewer` | `USE_WORKSPACE`, `VIEW_ITEM`, `QUERY_DATA`, `USE_AGENT`, `VIEW_AGENT` |
| `contributor` | `viewer` + `EDIT_ITEM`, `REFRESH_DATA`, `DEPLOY` |
| `member` | `contributor` + `MANAGE_ITEM` |
| `admin` | `member` + `MANAGE_GRANTS`, `ACTIVATE_PUBLISH`, `VIEW_AUDIT`, `MANAGE_WORKSPACE` |
| `platform_admin` | `MANAGE_PLATFORM` plus admin-equivalent grants across workspaces |

Roles must compile into grants. Runtime authorization should check privileges,
not role names.

## Item Access

Workspace access and item access are separate.

A user can receive access to a dashboard, semantic model, or agent policy
without becoming a workspace admin. This mirrors Fabric item sharing while using
the same grant engine internally.

Examples:

- Share a dashboard: grant `VIEW_ITEM` on the dashboard.
- Allow dashboard interaction: grant `QUERY_DATA` on the dashboard's semantic
  model or relevant datasets.
- Allow editing a model: grant `EDIT_ITEM` on the semantic model.
- Allow managing access: grant `MANAGE_GRANTS` on the item.

## Data Security

Data access is not equivalent to item read access.

LibreDash should enforce data privileges before query execution:

- `QUERY_DATA` for aggregate/report queries.
- `PREVIEW_DATA` for raw row previews.
- Column grants or column masks for sensitive fields.
- Row filters for user/group-specific data restrictions.

Semantic-model RLS/OLS is allowed, but durable data security should live in the
data authorization layer so that APIs, dashboards, agent tools, and future
compute surfaces evaluate the same rules.

## Service Principals

Service principals are first-class principals.

They can:

- Receive grants directly.
- Be members of groups.
- Own publishes, jobs, and API tokens.
- Use OAuth client credentials for automation.

Service principals should not bypass grants. Their access is evaluated exactly
like user access.

## API Tokens

API tokens are credentials for an existing principal, not principals.

Tokens must be:

- Hashed at rest.
- Revocable.
- Expiring by default.
- Workspace-scoped when possible.
- Privilege-scoped as a maximum allowlist.

Effective token access is:

```text
principal grants ∩ token scope ∩ token privilege allowlist
```

## Enforcement Rules

Authorization must be checked at the deepest practical layer, not only at HTTP
routes.

Required checks:

- HTTP routes check coarse privilege and workspace scope.
- Services check object-level privilege.
- Repositories include workspace/object ownership filters.
- Query planning checks model, table, column, row-policy, and preview grants.
- Agent tools check the same API privileges as direct API calls.
- Background jobs run as the requesting principal or an explicit service
  principal.

Workspace ID must always be part of scoped reads and writes.

## Audit

LibreDash should audit:

- Sign-in and sign-out.
- Session and token creation/revocation.
- Grant and role changes.
- Group membership changes.
- Publish create/activate/rollback.
- Data queries and previews.
- Agent tool calls that read or mutate workspace state.

Audit events should include:

- principal ID
- workspace ID
- action
- target object
- effective privilege
- request/correlation ID
- result status
- timestamp

## Authoring Model

File-backed access policy should compile into the same grant tables used by the
UI and API.

Preferred authored resources:

- `WorkspaceGroup`
- `WorkspaceRoleBinding`
- `Grant`
- `DataPolicy`

Role bindings are convenience syntax. The compiler should expand them into
explicit grants.

## Migration Direction

1. Keep existing principals, groups, sessions, API tokens, and role bindings.
2. Add securable objects and grants as the canonical authorization store.
3. Compile current roles into grants.
4. Update middleware to check privileges through the grant engine.
5. Add workspace/object filters to agent repositories.
6. Split `asset:read` into `VIEW_ITEM`, `QUERY_DATA`, and `PREVIEW_DATA`.
7. Add `show grants` and effective-privileges APIs.
8. Add data-level policies for row filters and column masks.
9. Move API automation toward service-principal OAuth.

## References

- Microsoft Fabric permission model:
  https://learn.microsoft.com/en-us/fabric/security/permission-model
- Microsoft Fabric workspace roles:
  https://learn.microsoft.com/en-us/fabric/fundamentals/roles-workspaces
- Databricks Unity Catalog access control:
  https://docs.databricks.com/aws/en/data-governance/unity-catalog/access-control/
- Databricks securable objects:
  https://docs.databricks.com/aws/en/data-governance/unity-catalog/securable-objects
