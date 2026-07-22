# Agent integrations

LeapView conversations are global and owned by the authenticated principal. Workspaces are asset containers: a workspace-aware tool requires an explicit `workspace` argument, then enforces the principal's privileges, any REST credential restrictions, data policies, and the governed query layer.

## Configure the built-in model provider

The built-in chat surface uses an OpenAI-compatible provider configuration:

```sh
LEAPVIEW_AGENT_BASE_URL=https://api.openai.com/v1
LEAPVIEW_AGENT_MODEL=<model-id>
LEAPVIEW_AGENT_API_KEY=<secret>
```

Store the API key in the deployment secret manager. The global administrator-controlled system prompt is configured in the agent administration page. Provider prompts and responses may contain business context; review the provider's data handling, retention, regional, and contractual requirements before enabling it.

The MCP endpoint does not depend on this provider configuration. External MCP hosts can use LeapView tools when the built-in model is disabled.

## Ask through the CLI

```sh
leapview agent ask \
  --target "$LEAPVIEW_TARGET" \
  --token "$LEAPVIEW_API_TOKEN" \
  "Which categories contributed most to revenue in the sales workspace?"
```

Use `--conversation <id>` to continue an existing principal-owned conversation and `--json` for machine processing. List conversations with bounded pagination through `leapview agent conversations`. The CLI follows the asynchronous run to a terminal state.

## Integrate through REST

The generated [Agent API](/docs/api/agent) is rooted at `/api/v1/agent` and exposes global conversation creation, update, archive, messages, runs, and run events. The removed `/api/v1/workspaces/{workspace}/agent` routes have no compatibility aliases.

A typical client creates or selects a conversation, starts a run, records its identity, follows the run/event surface to a documented terminal state, renders the assistant message and tool evidence, and archives conversations according to retention policy. List endpoints use opaque pagination tokens.

## Integrate through MCP

Set `LEAPVIEW_PUBLIC_URL` to the deployment's canonical HTTPS origin, then give an MCP host such as Claude the deployment-specific URL `${LEAPVIEW_PUBLIC_URL}/mcp`. The host discovers authorization automatically and opens LeapView's sign-in and consent flow. LeapView implements Streamable HTTP 2025-11-25 with stateless JSON responses and exposes tools only—no resources, prompts, nested conversation tools, or stdio transport.

MCP and built-in chat consume the same catalog, schemas, handlers, authorization, projections, audit path, and execution errors. Successful tool calls return both `structuredContent` and equivalent JSON text. MCP access requires `USE_AGENT`; each tool additionally requires its generated workspace or resource privilege.

By default, LeapView is the MCP authorization server. It supports authorization code with S256 PKCE, refresh-token rotation, OAuth protected-resource and authorization-server discovery, Client ID Metadata Documents, and Dynamic Client Registration. The user approves the coarse `mcp:use` scope; live LeapView RBAC and data policies remain authoritative for every tool call. Access tokens last 15 minutes and refresh tokens last 30 days.

General LeapView API tokens and browser-session cookies are intentionally rejected at `/mcp`. A development bearer token remains available only with the local development bypass. Automated MCP clients use an existing LeapView service-principal ID and secret with the OAuth `client_credentials` grant, the `mcp:use` scope, and an exact `resource` of `${LEAPVIEW_PUBLIC_URL}/mcp`.

To delegate MCP authorization to an organization-wide provider, set `LEAPVIEW_MCP_OAUTH_ISSUER_URL`. The issuer must publish OpenID Connect discovery and sign JWT access tokens whose audience is exactly `${LEAPVIEW_PUBLIC_URL}/mcp`, whose subject identifies the user, and whose scope contains `mcp:use`. LeapView maps the external subject/email to a principal, then applies the same live authorization checks. In this mode clients use the external provider's advertised authorization endpoints; LeapView's embedded authorization endpoints are unavailable.

Cross-origin MCP requests are rejected. OAuth tokens are bearer credentials on the wire, so deploy only behind HTTPS and never place them in URLs or logs. Follow [Connect an MCP host](/docs/guides/integrate/mcp) for deployment configuration, Claude setup, OAuth discovery, service-principal automation, and troubleshooting.

## Validate answers and operate safely

Natural-language output is not a replacement for governed results. Present tool evidence, resource identity, filters, and relevant time or deployment context so a user can validate claims. Use deterministic semantic or dashboard queries for automated decisions that cannot tolerate interpretive variation.

Test empty results, authorization failures, workspace-bound credentials, ambiguous questions, provider timeouts, cancelled runs, and active deployment changes. Audit conversation and tool activity, apply bounded retention with `leapview admin maintenance`, and never log provider API keys or raw sensitive prompts into general diagnostics.

See [Service principals and API tokens](/docs/security/tokens) and the generated [`agent` CLI reference](/docs/cli/agent).
