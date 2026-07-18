# LibreDash Project Overview

LibreDash is a dashboards-as-code BI monolith. Go owns configuration compilation, security, deployments, managed data, DuckDB/DuckLake execution, and the Datastar SSE command loop. Gomponents renders page shells; Lit components render typed signal payloads in the browser.

## Architecture

- `dashboards/libredash.yaml` is the project entrypoint. It references global connections and sources plus workspace-scoped models, semantic models, dashboards, access policy, and agent policy.
- `internal/workspace/compiler/` loads, validates, and compiles the project into deployable serving-state artifacts.
- `internal/deployment/`, `internal/servingstate/`, and `internal/runtimehost/` prepare immutable serving-state generations, activate them per workspace/environment, lease DuckLake snapshots, and drain readers safely during cutover.
- `internal/manageddata/` implements local and S3-backed ingestion, revisions, upload protocols, runtime views, retention, and binding resolution.
- `internal/analytics/model/` defines semantic models. `internal/analytics/query/` plans governed single- and multi-fact queries. `internal/analytics/materialize/` and `internal/analytics/duckdb/` execute and cache them.
- `internal/access/` owns principals, authentication credentials, RBAC, grants, data policies, groups, SCIM, sessions, service principals, and access auditing.
- `internal/app/` is the composition root and top-level HTTP router. Feature handlers live beside their domains under packages such as `internal/dashboard/http`, `internal/workspace/http`, and `internal/agent/http`.
- `pkg/pagestream/` owns the shared page/SSE transport, signal history, broker, tracing, and escaped Datastar action construction.
- `api/signals/main.tsp` is the source of truth for browser signal contracts. Generation produces Go models in `internal/ui/signals/models.gen.go` and TypeScript types in `web/generated/signals/index.ts`.
- `internal/ui/` and `internal/dashboard/ui/` render gomponents document shells. `web/components/` contains Lit route and visual components.
- ECharts is the built-in chart renderer. TanStack powers table state and virtualization behind LibreDash-owned signal/query contracts.

## Runtime Flow

1. `GET /workspaces` or `GET /workspaces/{workspace}` renders a pagestream document shell.
2. Dashboard routes are `GET /workspaces/{workspace}/dashboards/{dashboard}` and `/pages/{page}`.
3. Each page opens the canonical `GET /updates?...` Datastar SSE stream from `data-init`.
4. Browser components emit small domain events. Gomponents attributes translate them into CSRF-protected Datastar commands.
5. Domain handlers authorize the request, update stream state, execute governed DuckDB queries where needed, and publish typed signal patches through `pkg/pagestream`.
6. Lit components subscribe to signal paths and render without ad hoc data-fetch APIs.

## Important Files

- `cmd/libredash/main.go` and `internal/cli/serve.go`: process startup and lifecycle.
- `cmd/libredash-site/main.go` and `internal/site/http/`: independently deployable public site startup and HTTP adapter.
- `internal/app/router.go`: canonical page, command, auth, admin, and API routes.
- `internal/workspace/compiler/compiler.go`: project compilation entrypoint.
- `internal/runtimehost/manager.go`: serving-generation and snapshot-lease lifecycle.
- `internal/analytics/materialize/runtime.go`: query execution, coalescing, and cache integration.
- `internal/analytics/query/planner.go`: semantic query planning.
- `internal/dashboard/runtime/`: dashboard query orchestration and signal payload construction.
- `internal/ui/page.go` and `internal/dashboard/ui/page.go`: gomponents page shells.
- `web/components/dashboard/dashboard-page.ts`: interactive report surface.
- `web/components/dashboard/table/report-table.ts`: BI table component.
- `docs/`: authored and generated public documentation; `site/`: site-specific browser source and static assets.
- `.github/workflows/ci.yml`: canonical parallel CI workflow.

## Development

- `task dev` builds, bootstraps, deploys, and starts the managed development server.
- `task test` generates required sources/assets and runs the complete Go and browser test suite.
- `task ci` adds generated-artifact checks, static/race analysis, route QA, and deployment validation.
- `task generate` regenerates sqlc, configuration, API, signal, and JSON Schema artifacts.
- `task generated:check` verifies intentional public contract snapshots are current.
- `task dev:status`, `task dev:logs`, and `task dev:stop` manage the worktree-local server.

Use `task ci` before handing off substantial changes. Follow red-green-refactor for features and fixes. Prefer long-term correctness, simplicity, robustness, and scalability over minimizing implementation cost.
