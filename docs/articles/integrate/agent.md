# Agent integrations

LibreDash agents operate inside a workspace and use a policy-controlled tool catalog. They reuse active dashboards and semantic models and do not bypass authorization, data policies, or the governed query layer.

## Configure the model provider

The server uses an OpenAI-compatible provider configuration:

```sh
LIBREDASH_AGENT_BASE_URL=https://api.openai.com/v1
LIBREDASH_AGENT_MODEL=<model-id>
LIBREDASH_AGENT_API_KEY=<secret>
```

Store the API key in the deployment secret manager. Provider prompts and responses may contain business context; review the provider's data handling, retention, regional, and contractual requirements before enabling the feature.

## Define workspace policy

Create a `WorkspaceAgentPolicy` discovered by the workspace:

```yaml
apiVersion: libredash.dev/v1
kind: WorkspaceAgentPolicy
metadata:
  workspace: sales
  name: default
spec:
  enabled: true
  tools:
    allow:
      - list_dashboards
      - describe_dashboard
      - query_dashboard_page
      - list_semantic_models
      - describe_model
      - query_semantic_dataset
      - explain_semantic_query
    deny: []
  instructions: Use the sales workspace semantic models for revenue and order questions. Cite the dashboard or semantic result used.
```

Allow only tools needed for the workspace use case. The current built-in surface is analytical and discovery-oriented; keep any future write-capable or administrative tools in a separately reviewed privilege boundary.

Instructions guide behavior but are not an authorization control. The server must still enforce tool allowlists, principal privileges, data policy, and resource identity.

## Ask through the CLI

```sh
libredash agent ask \
  --workspace sales \
  --target "$LIBREDASH_TARGET" \
  --token "$LIBREDASH_API_TOKEN" \
  "Which categories contributed most to revenue?"
```

The command returns conversation/run output. Use `--conversation <id>` to continue an existing conversation and `--json` for machine processing. List conversations with bounded pagination through `libredash agent conversations`.

For automation, follow terminal run state rather than assuming the first response contains the complete answer. Preserve conversation and run IDs for correlation.

## Integrate through the API

The generated [Agent API](/docs/api/agent) exposes conversation creation/update/archive, messages, runs, run events, and turns. A typical client:

1. creates or selects a workspace conversation;
2. posts a turn;
3. records the returned run identity;
4. polls or consumes the supported run/event surface;
5. stops at a documented terminal state;
6. renders the assistant message and tool evidence;
7. archives conversations according to policy.

List endpoints use opaque pagination tokens. Do not assume every transient event is retained forever or that events arrive as a complete global ordering across separate runs.

## Validate answers

Natural-language output is not a replacement for governed results. Present tool evidence, resource identity, filters, and relevant time/deployment context so a user can validate the claim. Use deterministic semantic or dashboard queries for automated decisions that cannot tolerate interpretive variation.

Test refusal and empty-result behavior, policy-denied tools, unauthorized data, ambiguous questions, provider timeouts, cancelled runs, and active deployment changes.

## Security and operations

Give agent users only `USE_AGENT`/view and query privileges required for the workspace. Audit conversation and tool activity where events are emitted. Apply bounded retention with `libredash admin maintenance`; archived conversations should not remain indefinitely by accident.

Monitor provider latency, failures, token/cost usage where available, run duration, tool errors, and repeated policy denial. Never log provider API keys or raw sensitive prompts into general diagnostics.

See [Workspace Agent Policy configuration](/docs/config/workspace-agent-policy), [Service principals and API tokens](/docs/security/tokens), and the generated [`agent` CLI reference](/docs/cli/agent).
