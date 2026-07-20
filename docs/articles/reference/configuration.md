# Configuration reference

LeapView configuration resources use versioned YAML envelopes and generated JSON Schemas. Open the resource page that matches the `kind` you are authoring when you need exact field names, required values, nested structures, or validation rules.

## Choose the right resource

- Start with **Project** for repository-wide discovery of connections, sources, and workspaces.
- Use **Connection** and **Source** to separate physical access credentials from reusable data inputs.
- Use **Workspace**, **WorkspaceGroup**, **WorkspaceRoleBinding**, and **Grant** to define ownership and access boundaries.
- Use **ModelTable**, **SemanticModel**, and **Dashboard** for analytical modeling and presentation.
- Use **DataPolicy** for governed row and column access. Agent provider settings and the system prompt are installation-wide runtime configuration.

Each generated page includes a representative YAML resource, a field table, nested definitions, and a downloadable schema. Complete resources containing `apiVersion` and `kind` are validated against these same schemas when documentation tests run.

## Runtime settings

Resource YAML is separate from process-wide runtime configuration. Use the [environment variable reference](/docs/configuration) for server addresses, storage, authentication, secrets, and production validation requirements.

Use the task-oriented guides to decide what to model. Return here when you need the exact contract accepted by the current LeapView version.
