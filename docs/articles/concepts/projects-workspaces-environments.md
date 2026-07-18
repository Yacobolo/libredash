# Projects, workspaces, and environments

Projects, workspaces, and environments are separate dimensions in LibreDash. Treating them as interchangeable leads to duplicated configuration and unclear access boundaries.

## Project

A project is the atomic configuration-as-code delivery unit. Its manifest discovers project-global connections and sources and one or more workspace manifests:

```yaml
apiVersion: libredash.dev/v1
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

A workspace is the primary product ownership and authorization boundary. It owns:

- model tables and their materialization lifecycle;
- semantic models and business definitions;
- dashboards and report pages;
- groups, role bindings, grants, and data policies;
- an optional agent policy.

Workspace resources include `metadata.workspace` where their schema requires it. References must remain inside the allowed workspace and project graph. A Sales dashboard should not silently reach an Operations model table merely because both files exist in the repository.

Use one workspace for a coherent audience and governed semantic surface. Do not create a workspace per dashboard; several related dashboards should normally reuse one workspace semantic layer. Split workspaces when ownership, source permissions, authorization, or operational lifecycle differs materially.

## Environment

An environment is a serving dimension, normally `dev`, `staging`, or `prod`. The environment selects which validated project deployment and managed-data revisions are active. It is not another resource directory.

Keep environment-specific secrets, service URLs, storage locations, and active state in targets and runtime configuration. Keep business definitions in the shared project tree. This avoids drift between copied `dashboards-dev/` and `dashboards-prod/` directories.

The standard progression is:

1. Validate the same project source locally.
2. Plan against the target environment's active deployment.
3. Review the resource and data-revision changes.
4. Deploy the candidate to that environment.
5. Verify the resulting active state before promoting the same revision onward.

## Atomic delivery

One project deployment may change global references and several workspaces together. LibreDash builds and validates the candidate before activation. Activation switches the serving pointers only after the candidate is acceptable, so a failed candidate does not partially update one workspace while leaving another on incompatible definitions.

Managed-data revisions follow the same principle. A plan identifies immutable revisions; deployment activates the reviewed combination. Avoid mutable file replacement in an active serving directory because it bypasses that boundary.

## Choosing boundaries

Ask these questions when organizing a repository:

- Is this input shared and governed across workspaces? Define it as a project connection/source.
- Do these dashboards share owners, semantic definitions, and access rules? Keep them in one workspace.
- Does only infrastructure or serving state differ? Use environments and targets, not copied YAML.
- Must several changes become visible together? Deliver them in one project deployment.

See [Project configuration](/docs/config/project), [Workspace configuration](/docs/config/workspace), and [Targets and environments](/docs/cli/targets) for the exact contracts and workflow.
