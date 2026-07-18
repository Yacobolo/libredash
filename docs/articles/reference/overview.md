# Reference

Reference pages describe exact accepted syntax and supported machine contracts. They are generated from the same source definitions used by the application wherever possible. Use guides for intent and workflow; use reference when writing a field, flag, request, or visual shape.

## Configuration

YAML resource pages are generated from exported schemas and include a representative example, field table, nested definitions, and downloadable JSON Schema. The catalog covers projects, connections, sources, workspaces, model tables, semantic models, dashboards, access resources, and agent policies.

When a guide and reference appear to disagree on exact syntax, follow the generated resource page and report the guide drift. Do not hand-edit generated pages.

Process-global environment variables are generated from the runtime configuration specification. The reference includes type/default, scope, lifecycle, secret classification, description, and cross-field production relationships.

## CLI

Command pages are generated from the Cobra command tree, so usage, positional arguments, flags, defaults, and inherited options match the current source. Use the command page associated with the deployed application version.

Human-readable output is not automatically a stable parsing format. Prefer `--json` where supported, or use the headless API for a maintained machine schema.

## API

Endpoint pages and the downloadable OpenAPI document are generated from the TypeSpec API contract. Operation groups cover current user, workspaces, search, BI, deployments, refresh runs, agent, access, audit, and managed data.

OpenAPI is the client-generation source. Generated prose summarizes endpoints but may not expand every nested request/response type inline; inspect the downloadable document or generated client types for the exact schema.

## Visuals

Visual pages combine a live production component with dashboard configuration and expected query shape. The visual overview helps choose a renderer-neutral type. ECharts-specific adaptation remains behind LibreDash visual contracts.

Validate a new visual against the live documentation route because it exercises the site bundle, lazy renderer registration, theme behavior, and example payload together.

## Generated-artifact discipline

Run:

```sh
task generate
task docs:check
```

Generation composes configuration, CLI, API, visual, authored navigation, runtime catalog, and search artifacts. `docs:check` fails when committed catalog/search output no longer matches its sources and also detects duplicate routes, missing or orphaned Markdown, broken internal documentation links, and invalid YAML fences.

Reference tells you what is accepted. Start from [Supported capabilities](/docs/reference/capabilities) when choosing a surface, then return to the relevant task-oriented guide for design and operational advice.
