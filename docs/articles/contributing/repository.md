# Repository and development workflow

LibreDash is a monorepo containing the product application, browser components, resource examples, generators, deployment contracts, documentation, and independently deployable public site. Keeping these surfaces together lets one pull request update behavior and its contracts atomically.

## Important locations

- `cmd/libredash/` — application and CLI entry point.
- `internal/` — domain, transport, query, runtime, storage, access, agent, and generator packages.
- `api/typespec/` — headless API source contract.
- `api/signals/` — UI signal source contract.
- `dashboards/` — complete example configuration-as-code projects.
- `web/components/` — product Lit components and renderer adapters.
- `static/` — built product browser assets.
- `docs/articles/` — authored task and concept documentation.
- `docs/reference/`, `docs/api/`, and `docs/visuals/` — generated or catalogued reference inputs.
- `site/` and `internal/site/` — public site assets and Go HTTP server.
- `deploy/hetzner/` — supported single-node deployment contract.

Read the nearest `AGENTS.md` before editing. Preserve unrelated user changes in a dirty worktree.

## Development loop

Use red-green-refactor for behavior changes:

1. Add or update a focused test that demonstrates missing behavior.
2. Run it and confirm the expected failure.
3. Implement the smallest coherent change.
4. Run focused tests until green.
5. Refactor while keeping tests green.
6. Run generated checks and the full CI gate.

Prefer package-level Go tests and focused Bun/Playwright tests during iteration. Use `task ci` before handing off substantial work.

## Managed development server

Use the worktree-safe commands:

```sh
task dev
task dev:status
task dev:logs
task dev:stop
```

The workflow stores process state beneath `.tmp/` and selects a worktree-local port. Do not kill unrelated processes or reuse persistent state from another worktree implicitly.

## Generation

Run:

```sh
task generate
```

It produces database code, configuration surfaces, API and UI-signal contracts, JSON Schemas, CLI docs, and the unified documentation catalog/search index. Individual generator tasks exist for focused work.

Do not manually edit a file marked generated. Change TypeSpec, CUE/config contracts, Cobra commands, configuration specs, or the owning generator. Generated implementation code, reference prose, catalogs, and search indexes are build inputs and stay out of Git. Only intentional public snapshots—`.env.example`, JSON Schemas, and the OpenAPI contract—are committed so integrations and reviewers can consume their exact version.

Use `task docs:check` and `task config:check` to validate generated output. `task generated:check` detects drift in the public snapshots. CI generates build-only inputs once, verifies deterministic output, and shares them with downstream jobs.

## Browser assets

Product assets and public-site assets have separate builds. Component DOM tests use Playwright/Bun against focused bundles. Site tests exercise production lazy chunks and documentation routes. Design-token checks enforce the supported Primer-backed styling boundary.

When changing a component, test its cold/unupgraded layout, upgraded behavior, compact width, theme, and cleanup where relevant.

## Project and schema changes

Update the example dashboards when a contract changes. A feature is not complete if the code accepts it but schemas, generated references, examples, and docs disagree.

Validate example projects and generated YAML fences. Keep stable identifiers and provide migrations for intentional compatibility breaks.

## Final verification

The standard gate is:

```sh
task ci
task vuln
git diff --check
```

Review `git status` and the diff before committing. Keep product, tests, generated contracts, and documentation in the same pull request when they describe one behavior change.
