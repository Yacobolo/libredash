package api

import "list"

schema_version: "v1"

api: {
	base_path: "/api/v1"
}

info: {
	title:       "LibreDash Headless API"
	version:     "1.0.0"
	description: "Headless API for LibreDash workspaces, deployments, access control, materializations, and agent operations."
}

openapi: {
	version: "3.0.0"
	tag_order: ["Current User", "Workspaces", "Deployments", "Materializations", "Agent", "Access", "Audit"]
	security_schemes: {
		BearerAuth: {
			type:   "http"
			scheme: "bearer"
		}
	}
}

tags: [
	{name: "Current User", description: "Current principal, sessions, tokens, and permissions."},
	{name: "Workspaces", description: "Workspace and lineage discovery."},
	{name: "Deployments", description: "Dashboard-as-code deployment operations."},
	{name: "Materializations", description: "Headless materialization run operations."},
	{name: "Agent", description: "Headless agent conversation and run operations."},
	{name: "Access", description: "Principals, groups, roles, and role bindings."},
	{name: "Audit", description: "Workspace audit event discovery."},
]

endpoints: list.Concat([
	#currentUserEndpoints,
	#workspaceEndpoints,
	#deploymentEndpoints,
	#materializationEndpoints,
	#agentEndpoints,
	#accessEndpoints,
	#auditEndpoints,
])
