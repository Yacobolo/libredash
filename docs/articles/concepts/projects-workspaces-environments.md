# Projects, workspaces, and environments

Projects, workspaces, and instance environments are separate concepts. A LeapView instance serves exactly one environment; use separate instance targets for development, staging, and production.

## Project

A project is the atomic configuration-as-code delivery unit. Its manifest discovers project-global connections and sources and one or more workspace manifests:

```yaml
apiVersion: leapview.dev/v1
kind: Project
metadata:
  name: commerce
spec:
  connections:
    include: [connections/*.yaml]
  sources:
    include: [sources/*.yaml]
  workspaces:
    include: [workspaces/*/workspace.yaml]
```

The project name is used by project-scoped planning, deployment, and managed-data operations. Keep it stable. Changing a display title is harmless; changing `metadata.name` creates a different resource identity and should be planned as a migration.

Connections and sources live at project scope so several workspaces can depend on one governed input. This does not grant every workspace access automatically: each workspace declares the source names it uses.

## Workspace

A workspace is an asset container. It owns:

- model tables and their materialization lifecycle;
- semantic models and business definitions;
- dashboards and report pages;
- groups, role bindings, grants, and data policies that govern those assets.

Workspace resources include `metadata.workspace` where their schema requires it. References must remain inside the allowed workspace and project graph. A Sales dashboard should not silently reach an Operations model table merely because both files exist in the repository.

Use one workspace for a coherent collection of related assets. Do not create a workspace per dashboard; several related dashboards should normally reuse one workspace semantic layer. Agent conversations and identities remain global. A workspace-aware tool selects the container explicitly and then enforces its asset privileges and data policies.

## Environment

An environment is the immutable serving identity of an instance, normally `dev`, `staging`, or `prod`. It selects the validated project deployment and managed-data revisions active in that instance. It is neither another resource directory nor a request-time selector.

Keep environment-specific secrets, service URLs, storage locations, and active state in separate target instances. Keep business definitions in the shared project tree. This avoids drift between copied `dashboards-dev/` and `dashboards-prod/` directories.

The standard progression is:

1. Validate the same project source locally.
2. Plan against the target instance's active deployment.
3. Review the resource and data-revision changes.
4. Deploy the candidate to that instance; the CLI discovers and asserts its environment.
5. Verify the resulting active state before promoting the same revision onward.

## Atomic delivery

One project deployment may change global references and several workspaces together. LeapView builds and validates the candidate before activation. Activation switches the serving pointers only after the candidate is acceptable, so a failed candidate does not partially update one workspace while leaving another on incompatible definitions.

Managed-data revisions follow the same principle. A plan identifies immutable revisions; deployment activates the reviewed combination. Avoid mutable file replacement in an active serving directory because it bypasses that boundary.

## Choosing boundaries

Ask these questions when organizing a repository:

- Is this input shared and governed across workspaces? Define it as a project connection/source.
- Do these dashboards share owners, semantic definitions, and access rules? Keep them in one workspace.
- Does only infrastructure or serving state differ? Use separate instance targets, not copied YAML.
- Must several changes become visible together? Deliver them in one project deployment.

See [Project configuration](/docs/config/project), [Workspace configuration](/docs/config/workspace), and [Targets and environments](/docs/cli/targets) for the exact contracts and workflow.
