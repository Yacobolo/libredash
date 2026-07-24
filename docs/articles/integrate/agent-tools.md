# Use the agent tool catalog

LeapView exposes one fixed read-only tool catalog through built-in chat and the deployment's MCP endpoint. Use catalog tools to identify governed resources before calling a query tool. Use documentation tools when the question is about LeapView itself.

## Choose a tool

| Need | Tool |
| --- | --- |
| Find a resource without knowing its workspace or parent | `catalog_search` |
| Browse the known catalog hierarchy one level at a time | `catalog_list` |
| Inspect one exact resource definition | `catalog_get` |
| Query a semantic model | `query_semantic_model` |
| Query an existing dashboard visual | `query_dashboard_visual` |
| Create a temporary visualization from semantic fields | `query_visual` |
| Find version-matched LeapView documentation | `docs_search` |
| Read a document returned by documentation search | `docs_read` |

The catalog does not expose connections, raw sources, model tables, refresh runs, lineage, raw SQL, previews, explanations, filter-value enumeration, page-wide queries, or mutation operations. Use the generated HTTP API or CLI when one of those separate product surfaces is appropriate.

## Identify resources with refs

Every catalog resource is identified by a closed `CatalogRef`:

```json
{
  "workspaceId": "sales",
  "type": "dashboard",
  "id": "executive-sales"
}
```

Supported `type` values are `workspace`, `dashboard`, `page`, `visual`, `filter`, `semantic_model`, `semantic_table`, `field`, and `measure`.

Treat the complete ref as the resource identity. IDs are meaningful only with their workspace and type, and some returned IDs encode their parent identity. Pass returned refs unchanged instead of constructing IDs from names.

Search and list results use the same compact item envelope. It contains the ref, display name, optional description, workspace, ancestors, known dashboard/page locations, browser URL, and the next tools that can act on the item.

## Find an unknown resource

`catalog_search` searches all workspaces the principal may access. It does not require a preceding workspace-list call.

```json
{
  "query": "monthly revenue",
  "types": ["dashboard", "visual", "measure"],
  "workspaceIds": ["sales"],
  "context": {
    "dashboardId": "executive-sales",
    "pageId": "overview"
  },
  "limit": 10
}
```

Only `query` is required. Type and workspace filters constrain the search; optional dashboard/page context influences ranking without changing authorization. The default limit is 10 and the maximum is 25.

Use `nextCursor` unchanged to continue the same search. Cursors are opaque and bound to the search, caller, and catalog snapshot. Restart from the first page if the catalog changed.

## Browse a known hierarchy

Call `catalog_list` without a parent to list accessible workspaces. Pass a returned ref as `parent` to browse exactly one level:

| Parent | Children |
| --- | --- |
| none | workspaces |
| workspace | dashboards and semantic models |
| dashboard | pages |
| page | visuals and filters |
| semantic model | semantic tables and measures |
| semantic table | fields |

```json
{
  "parent": {
    "workspaceId": "sales",
    "type": "dashboard",
    "id": "executive-sales"
  },
  "childTypes": ["page"],
  "limit": 25
}
```

`childTypes` is optional, but every requested type must be valid for the parent. Results are deterministically ordered. The default limit is 25 and the maximum is 50. Pass `nextCursor` back as `cursor` to continue; do not parse or edit it.

Listing an exact parent first resolves and authorizes that parent. Search and list silently omit inaccessible results.

## Inspect an exact definition

Pass a returned ref to `catalog_get`:

```json
{
  "ref": {
    "workspaceId": "sales",
    "type": "semantic_model",
    "id": "commerce"
  }
}
```

The result combines the normalized item envelope with type-specific `details`:

- workspaces include metadata and the active serving identity;
- dashboards include their semantic-model ref and page, visual, and filter counts;
- pages include their components;
- visuals include the compiled definition, query fields, columns, and placement;
- filters include their field, configuration, and placement;
- semantic models include counts and dashboard usage;
- semantic tables include source, grain, keys, and counts;
- fields and measures include their semantic metadata.

The result is a compact domain projection, not the raw deployed asset payload.

A visual or filter can appear on more than one page. If `catalog_get` returns `catalog_location_required`, choose one of the item's returned locations and retry:

```json
{
  "ref": {
    "workspaceId": "sales",
    "type": "visual",
    "id": "executive-sales.revenue-by-month"
  },
  "location": {
    "dashboardId": "executive-sales",
    "pageId": "overview"
  }
}
```

## Query governed data

Use the capabilities returned with a catalog item to choose the next tool.

- Use `query_semantic_model` with a semantic-model ref and the field and measure IDs discovered through catalog browsing. It returns governed row data and supports bounded pagination.
- Use `query_dashboard_visual` with the workspace, dashboard, page, and visual location of an existing visual. It preserves the dashboard definition, filters, authorization, and data-policy boundary.
- Use `query_visual` when no saved visual fits. Provide a workspace, semantic model, dataset, visual type, and semantic fields. It returns a renderer-independent visualization artifact and does not save or mutate the dashboard.

The exact contracts are versioned with the running LeapView release. Use the generated [Agent tool reference](/docs/agent-tools) for readable input and output schemas, or download the [machine-readable manifest](/docs/agent-tools/manifest.json). You can also inspect the matching local release with:

```sh
leapview agent tools
```

For MCP clients, the same schemas are returned by `tools/list`. Do not cache schemas across a LeapView upgrade without rediscovering the catalog.

## Read product documentation

Use `docs_search` for product behavior, configuration, CLI commands, API operations, and visual support:

```json
{
  "query": "semantic model relationships",
  "path": "concepts",
  "limit": 8
}
```

Pass a returned `doc:` ID unchanged to `docs_read`:

```json
{
  "id": "doc:concepts/semantic-models",
  "offset": 1,
  "limit": 200
}
```

Reads are line- and byte-bounded. Continue from `nextOffset` only when the current window is insufficient. These tools read the immutable documentation embedded in the running release; they cannot read arbitrary deployment files.

## Handle authorization and errors

All eight tools are read-only, idempotent, and non-destructive. MCP access requires `USE_AGENT`. Catalog operations require `VIEW_ITEM`; data-query tools require `QUERY_DATA` and continue to enforce resource grants and data policies.

Catalog lookup deliberately does not reveal inaccessible resources:

| Code | Meaning | Recovery |
| --- | --- | --- |
| `invalid_arguments` | The ref, child relationship, limit, filter, or cursor is invalid. | Correct the request using the discovered schema. |
| `catalog_not_found` | The resource is missing or inaccessible. | Search or list again with the current principal; do not infer which case occurred. |
| `catalog_location_required` | A shared visual or filter needs an exact dashboard/page location. | Retry with one of the returned locations. |
| `catalog_snapshot_changed` | The catalog changed during cursor pagination. | Restart the search or list from its first page. |

Expected tool failures are returned as tool errors. Transport and protocol failures are separate from resource and query errors.

## Verify the exposed surface

Run `leapview agent tools` against the same release used by a deployment. Built-in chat, deployment MCP `tools/list`, and the CLI catalog must expose exactly:

```text
catalog_search
catalog_list
catalog_get
query_semantic_model
query_dashboard_visual
query_visual
docs_search
docs_read
```

Legacy agent tool names are intentionally not exposed for new runs. Historical conversation entries remain renderable, but they are not callable aliases.
