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
	version:   "3.0.0"
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

#secure: [{BearerAuth: []}]

schemas: {
	"Error": {
		type: "object"
		properties: {
			"code": {schema: {type: "integer", format: "int32"}}
			"message": {schema: {type: "string"}}
			"details": {schema: {type: "object", additional_properties: {any: true}}}
			"requestId": {schema: {type: "string"}}
		}
		required: ["code", "message"]
	}
	"PageInfo": {
		type: "object"
		properties: {
			"nextCursor": {schema: {type: "string"}}
		}
	}
	"StatusResponse": {
		type: "object"
		properties: {
			"status": {schema: {type: "string"}}
		}
		required: ["status"]
	}
	"WorkspaceResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"title": {schema: {type: "string"}}
			"description": {schema: {type: "string"}}
			"activeDeploymentId": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"updatedAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "title", "description", "createdAt", "updatedAt"]
	}
	"WorkspaceListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "WorkspaceResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AssetResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"deploymentId": {schema: {type: "string"}}
			"type": {schema: {type: "string"}}
			"key": {schema: {type: "string"}}
			"parentId": {schema: {type: "string"}}
			"title": {schema: {type: "string"}}
			"description": {schema: {type: "string"}}
			"meta": {schema: {type: "object", additional_properties: {any: true}}}
			"href": {schema: {type: "string"}}
		}
		required: ["id", "workspaceId", "deploymentId", "type", "key", "title", "description"]
	}
	"AssetListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AssetResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AssetEdgeResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"deploymentId": {schema: {type: "string"}}
			"fromAssetId": {schema: {type: "string"}}
			"toAssetId": {schema: {type: "string"}}
			"type": {schema: {type: "string"}}
		}
		required: ["id", "workspaceId", "deploymentId", "fromAssetId", "toAssetId", "type"]
	}
	"AssetEdgeListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AssetEdgeResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"PrincipalResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"email": {schema: {type: "string"}}
			"displayName": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"updatedAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "email", "displayName", "createdAt", "updatedAt"]
	}
	"PrincipalListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "PrincipalResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"PrincipalPatchRequest": {
		type: "object"
		properties: {
			"displayName": {schema: {type: "string"}}
		}
	}
	"PermissionListResponse": {
		type: "object"
		properties: {
			"workspaceId": {schema: {type: "string"}}
			"permissions": {schema: {type: "array", items: {type: "string"}}}
		}
		required: ["workspaceId", "permissions"]
	}
	"APITokenCreateRequest": {
		type: "object"
		properties: {
			"name": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"permissions": {schema: {type: "array", items: {type: "string"}}}
			"expiresAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["name"]
	}
	"APITokenResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"name": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"permissions": {schema: {type: "array", items: {type: "string"}}}
			"expiresAt": {schema: {type: "string", format: "date-time"}}
			"revokedAt": {schema: {type: "string", format: "date-time"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"lastUsedAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "name", "permissions", "createdAt"]
	}
	"APITokenCreateResponse": {
		type: "object"
		properties: {
			"token": {schema: {type: "string"}}
			"apiToken": {schema: {ref: "APITokenResponse"}}
		}
		required: ["token", "apiToken"]
	}
	"APITokenListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "APITokenResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"SessionResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"expiresAt": {schema: {type: "string", format: "date-time"}}
			"lastSeenAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "createdAt", "expiresAt"]
	}
	"SessionListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "SessionResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"RoleResponse": {
		type: "object"
		properties: {
			"name": {schema: {type: "string"}}
			"permissions": {schema: {type: "array", items: {type: "string"}}}
		}
		required: ["name", "permissions"]
	}
	"RoleListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "RoleResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"GroupCreateRequest": {
		type: "object"
		properties: {
			"name": {schema: {type: "string"}}
			"displayName": {schema: {type: "string"}}
		}
		required: ["name"]
	}
	"GroupPatchRequest": {
		type: "object"
		properties: {
			"displayName": {schema: {type: "string"}}
		}
	}
	"GroupResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"name": {schema: {type: "string"}}
			"displayName": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"updatedAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "name", "displayName", "createdAt", "updatedAt"]
	}
	"GroupListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "GroupResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"GroupMemberListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "PrincipalResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"RoleBindingRequest": {
		type: "object"
		properties: {
			"subjectType": {schema: {type: "string", enum: ["principal", "group"]}}
			"subjectId": {schema: {type: "string"}}
			"role": {schema: {type: "string"}}
		}
		required: ["subjectType", "subjectId", "role"]
	}
	"RoleBindingResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"subjectType": {schema: {type: "string", enum: ["principal", "group"]}}
			"subjectId": {schema: {type: "string"}}
			"email": {schema: {type: "string"}}
			"displayName": {schema: {type: "string"}}
			"role": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "workspaceId", "subjectType", "subjectId", "role", "createdAt"]
	}
	"RoleBindingListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "RoleBindingResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"DeploymentCreateRequest": {
		type: "object"
		properties: {
			"title": {schema: {type: "string"}}
			"description": {schema: {type: "string"}}
		}
	}
	"DeploymentArtifactResponse": {
		type: "object"
		properties: {
			"deploymentId": {schema: {type: "string"}}
			"sizeBytes": {schema: {type: "integer", format: "int64"}}
		}
		required: ["deploymentId", "sizeBytes"]
	}
	"DeploymentArtifactUploadRequest": {
		type:   "string"
		format: "binary"
	}
	"DeploymentResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"status": {schema: {type: "string", enum: ["draft", "validated", "active", "inactive", "failed"]}}
			"digest": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"activatedAt": {schema: {type: "string", format: "date-time"}}
			"error": {schema: {type: "string"}}
		}
		required: ["id", "workspaceId", "status", "digest", "createdAt"]
	}
	"DeploymentListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "DeploymentResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"MaterializationRunCreateRequest": {
		type: "object"
		properties: {
			"modelId": {schema: {type: "string"}}
			"deploymentId": {schema: {type: "string"}}
		}
		required: ["modelId"]
	}
	"MaterializationRunResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"modelId": {schema: {type: "string"}}
			"deploymentId": {schema: {type: "string"}}
			"status": {schema: {type: "string", enum: ["queued", "running", "succeeded", "failed"]}}
			"error": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"startedAt": {schema: {type: "string", format: "date-time"}}
			"finishedAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "workspaceId", "modelId", "status", "createdAt"]
	}
	"MaterializationRunListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "MaterializationRunResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AgentConversationCreateRequest": {
		type: "object"
		properties: {
			"title": {schema: {type: "string"}}
		}
	}
	"AgentConversationPatchRequest": {
		type: "object"
		properties: {
			"title": {schema: {type: "string"}}
		}
	}
	"AgentConversationResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"principalId": {schema: {type: "string"}}
			"title": {schema: {type: "string"}}
			"status": {schema: {type: "string", enum: ["active", "archived"]}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"updatedAt": {schema: {type: "string", format: "date-time"}}
			"archivedAt": {schema: {type: "string", format: "date-time"}}
			"messageCount": {schema: {type: "integer", format: "int32"}}
			"lastMessageText": {schema: {type: "string"}}
			"titlePending": {schema: {type: "boolean"}}
		}
		required: ["id", "workspaceId", "principalId", "title", "status", "createdAt", "updatedAt"]
	}
	"AgentConversationListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AgentConversationResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AgentRunResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"conversationId": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"principalId": {schema: {type: "string"}}
			"status": {schema: {type: "string"}}
			"model": {schema: {type: "string"}}
			"stopReason": {schema: {type: "string"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
			"startedAt": {schema: {type: "string", format: "date-time"}}
			"completedAt": {schema: {type: "string", format: "date-time"}}
			"error": {schema: {type: "string"}}
		}
		required: ["id", "conversationId", "workspaceId", "principalId", "status", "createdAt"]
	}
	"AgentRunListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AgentRunResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AgentMessageResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"runId": {schema: {type: "string"}}
			"seq": {schema: {type: "integer", format: "int64"}}
			"role": {schema: {type: "string"}}
			"contentText": {schema: {type: "string"}}
			"content": {schema: {type: "object", additional_properties: {any: true}}}
			"toolCallId": {schema: {type: "string"}}
			"toolName": {schema: {type: "string"}}
			"isError": {schema: {type: "boolean"}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "seq", "role", "createdAt"]
	}
	"AgentMessageListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AgentMessageResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AgentTurnRequest": {
		type: "object"
		properties: {
			"input": {schema: {type: "string"}}
			"correlationId": {schema: {type: "string"}}
		}
		required: ["input"]
	}
	"AgentTurnResponse": {
		type: "object"
		properties: {
			"conversationId": {schema: {type: "string"}}
			"runId": {schema: {type: "string"}}
			"stopReason": {schema: {type: "string"}}
			"content": {schema: {type: "string"}}
		}
		required: ["conversationId", "runId", "stopReason", "content"]
	}
	"AgentEventResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"runId": {schema: {type: "string"}}
			"seq": {schema: {type: "integer", format: "int64"}}
			"eventType": {schema: {type: "string"}}
			"severity": {schema: {type: "string", enum: ["debug", "info", "warning", "error"]}}
			"payload": {schema: {type: "object", additional_properties: {any: true}}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "runId", "seq", "eventType", "severity", "payload", "createdAt"]
	}
	"AgentEventListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AgentEventResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
	"AuditEventResponse": {
		type: "object"
		properties: {
			"id": {schema: {type: "string"}}
			"workspaceId": {schema: {type: "string"}}
			"principalId": {schema: {type: "string"}}
			"action": {schema: {type: "string"}}
			"targetType": {schema: {type: "string"}}
			"targetId": {schema: {type: "string"}}
			"metadata": {schema: {type: "object", additional_properties: {any: true}}}
			"createdAt": {schema: {type: "string", format: "date-time"}}
		}
		required: ["id", "workspaceId", "action", "targetType", "targetId", "metadata", "createdAt"]
	}
	"AuditEventListResponse": {
		type: "object"
		properties: {
			"items": {schema: {type: "array", items: {ref: "AuditEventResponse"}}}
			"page": {schema: {ref: "PageInfo"}}
		}
		required: ["items", "page"]
	}
}

#errorResponses: [
	{status_code: 400, description: "bad request", schema: {ref: "Error"}},
	{status_code: 401, description: "unauthorized", schema: {ref: "Error"}},
	{status_code: 403, description: "forbidden", schema: {ref: "Error"}},
	{status_code: 404, description: "not found", schema: {ref: "Error"}},
	{status_code: 409, description: "conflict", schema: {ref: "Error"}},
	{status_code: 429, description: "rate limited", schema: {ref: "Error"}},
	{status_code: 500, description: "internal server error", schema: {ref: "Error"}},
]

#workspaceParam: {name: "workspace", in: "path", required: true, description: "Workspace ID.", schema: {type: "string"}}
#deploymentParam: {name: "deployment", in: "path", required: true, description: "Deployment ID.", schema: {type: "string"}}
#conversationParam: {name: "conversation", in: "path", required: true, description: "Agent conversation ID.", schema: {type: "string"}}
#runParam: {name: "run", in: "path", required: true, description: "Agent run ID.", schema: {type: "string"}}
#principalParam: {name: "principal", in: "path", required: true, description: "Principal ID.", schema: {type: "string"}}
#groupParam: {name: "group", in: "path", required: true, description: "Group ID.", schema: {type: "string"}}
#bindingParam: {name: "binding", in: "path", required: true, description: "Role binding ID.", schema: {type: "string"}}
#tokenParam: {name: "token", in: "path", required: true, description: "API token ID.", schema: {type: "string"}}
#sessionParam: {name: "session", in: "path", required: true, description: "Session ID.", schema: {type: "string"}}
#limitParam: {name: "limit", in: "query", description: "Maximum items to return.", schema: {type: "integer", format: "int32"}}
#cursorParam: {name: "pageToken", in: "query", description: "Opaque pagination cursor.", schema: {type: "string"}}

#workspaceRead: {"x-authz": {mode: "permission", permission: "workspace:read"}}
#assetRead: {"x-authz": {mode: "permission", permission: "asset:read"}}
#deploymentRead: {"x-authz": {mode: "permission", permission: "deployment:read"}}
#deploymentWrite: {"x-authz": {mode: "permission", permission: "deployment:write"}}
#deploymentActivate: {"x-authz": {mode: "permission", permission: "deployment:activate"}}
#rbacRead: {"x-authz": {mode: "permission", permission: "rbac:read"}}
#rbacWrite: {"x-authz": {mode: "permission", permission: "rbac:write"}}
#agentUse: {"x-authz": {mode: "permission", permission: "agent:use"}}
#agentRead: {"x-authz": {mode: "permission", permission: "agent:read"}}
#materializationRun: {"x-authz": {mode: "permission", permission: "materialization:run"}}
#auditRead: {"x-authz": {mode: "permission", permission: "audit:read"}}
#tokenManage: {"x-authz": {mode: "permission", permission: "token:manage"}}

endpoints: [
	{method: "get", path: "/me", operation_id: "getCurrentPrincipal", summary: "Get current principal", tags: ["Current User"], security: #secure, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "PrincipalResponse"}}], #errorResponses]), extensions: #workspaceRead},
	{method: "get", path: "/me/permissions", operation_id: "listCurrentPermissions", summary: "List current permissions", tags: ["Current User"], security: #secure, parameters: [{name: "workspace", in: "query", description: "Workspace ID.", schema: {type: "string"}}], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "PermissionListResponse"}}], #errorResponses]), extensions: #workspaceRead},
	{method: "get", path: "/me/api-tokens", operation_id: "listCurrentAPITokens", summary: "List current API tokens", tags: ["Current User"], security: #secure, parameters: [#limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "APITokenListResponse"}}], #errorResponses]), extensions: #tokenManage},
	{method: "post", path: "/me/api-tokens", operation_id: "createCurrentAPIToken", summary: "Create current API token", tags: ["Current User"], security: #secure, request_body: {required: true, schema: {ref: "APITokenCreateRequest"}}, responses: list.Concat([[{status_code: 201, description: "created", schema: {ref: "APITokenCreateResponse"}}], #errorResponses]), extensions: #tokenManage},
	{method: "delete", path: "/me/api-tokens/{token}", operation_id: "revokeCurrentAPIToken", summary: "Revoke current API token", tags: ["Current User"], security: #secure, parameters: [#tokenParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #tokenManage},
	{method: "get", path: "/me/sessions", operation_id: "listCurrentSessions", summary: "List current sessions", tags: ["Current User"], security: #secure, parameters: [#limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "SessionListResponse"}}], #errorResponses]), extensions: #workspaceRead},
	{method: "delete", path: "/me/sessions/{session}", operation_id: "revokeCurrentSession", summary: "Revoke current session", tags: ["Current User"], security: #secure, parameters: [#sessionParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #workspaceRead},

	{method: "get", path: "/workspaces", operation_id: "listWorkspaces", summary: "List workspaces", tags: ["Workspaces"], security: #secure, parameters: [#limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "WorkspaceListResponse"}}], #errorResponses]), cli: {command: ["workspaces", "list"], output: {mode: "raw"}}, extensions: #workspaceRead},
	{method: "get", path: "/workspaces/{workspace}/assets", operation_id: "listWorkspaceAssets", summary: "List workspace assets", tags: ["Workspaces"], security: #secure, parameters: [#workspaceParam, {name: "type", in: "query", description: "Filter by asset type.", schema: {type: "string"}}, {name: "q", in: "query", description: "Search query.", schema: {type: "string"}}, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AssetListResponse"}}], #errorResponses]), extensions: #assetRead},
	{method: "get", path: "/workspaces/{workspace}/asset-edges", operation_id: "listWorkspaceAssetEdges", summary: "List workspace asset edges", tags: ["Workspaces"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AssetEdgeListResponse"}}], #errorResponses]), extensions: #assetRead},

	{method: "post", path: "/workspaces/{workspace}/deployments", operation_id: "createDeployment", summary: "Create a deployment", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam], request_body: {required: false, schema: {ref: "DeploymentCreateRequest"}}, responses: list.Concat([[{status_code: 201, description: "created", schema: {ref: "DeploymentResponse"}}], #errorResponses]), extensions: #deploymentWrite},
	{method: "get", path: "/workspaces/{workspace}/deployments", operation_id: "listDeployments", summary: "List deployments", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "DeploymentListResponse"}}], #errorResponses]), cli: {command: ["deployments", "list"], output: {mode: "raw"}}, extensions: #deploymentRead},
	{method: "get", path: "/workspaces/{workspace}/deployments/{deployment}", operation_id: "getDeployment", summary: "Get a deployment", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam, #deploymentParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "DeploymentResponse"}}], #errorResponses]), extensions: #deploymentRead},
	{method: "put", path: "/workspaces/{workspace}/deployments/{deployment}/artifact", operation_id: "uploadDeploymentArtifact", summary: "Upload a deployment artifact", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam, #deploymentParam], request_body: {required: true, content_type: "application/octet-stream", schema: {ref: "DeploymentArtifactUploadRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "DeploymentArtifactResponse"}}], #errorResponses]), extensions: {"x-authz": {mode: "permission", permission: "deployment:write"}, "x-libredash-dispatch": "raw-body"}},
	{method: "post", path: "/workspaces/{workspace}/deployments/{deployment}/validate", operation_id: "validateDeployment", summary: "Validate a deployment", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam, #deploymentParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "DeploymentResponse"}}], #errorResponses]), extensions: #deploymentWrite},
	{method: "post", path: "/workspaces/{workspace}/deployments/{deployment}/activate", operation_id: "activateDeployment", summary: "Activate a deployment", tags: ["Deployments"], security: #secure, parameters: [#workspaceParam, #deploymentParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "DeploymentResponse"}}], #errorResponses]), extensions: #deploymentActivate},

	{method: "post", path: "/workspaces/{workspace}/materialization-runs", operation_id: "createMaterializationRun", summary: "Create a materialization run", tags: ["Materializations"], security: #secure, parameters: [#workspaceParam], request_body: {required: true, schema: {ref: "MaterializationRunCreateRequest"}}, responses: list.Concat([[{status_code: 202, description: "accepted", schema: {ref: "MaterializationRunResponse"}}], #errorResponses]), extensions: #materializationRun},
	{method: "get", path: "/workspaces/{workspace}/materialization-runs", operation_id: "listMaterializationRuns", summary: "List materialization runs", tags: ["Materializations"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "MaterializationRunListResponse"}}], #errorResponses]), extensions: #materializationRun},
	{method: "get", path: "/workspaces/{workspace}/materialization-runs/{run}", operation_id: "getMaterializationRun", summary: "Get a materialization run", tags: ["Materializations"], security: #secure, parameters: [#workspaceParam, #runParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "MaterializationRunResponse"}}], #errorResponses]), extensions: #materializationRun},

	{method: "post", path: "/workspaces/{workspace}/agent/conversations", operation_id: "createAgentConversation", summary: "Create an agent conversation", tags: ["Agent"], security: #secure, parameters: [#workspaceParam], request_body: {required: true, schema: {ref: "AgentConversationCreateRequest"}}, responses: list.Concat([[{status_code: 201, description: "created", schema: {ref: "AgentConversationResponse"}}], #errorResponses]), extensions: #agentUse},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations", operation_id: "listAgentConversations", summary: "List agent conversations", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentConversationListResponse"}}], #errorResponses]), cli: {command: ["agent", "conversations"], output: {mode: "raw"}}, extensions: #agentRead},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations/{conversation}", operation_id: "getAgentConversation", summary: "Get an agent conversation", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentConversationResponse"}}], #errorResponses]), extensions: #agentRead},
	{method: "patch", path: "/workspaces/{workspace}/agent/conversations/{conversation}", operation_id: "updateAgentConversation", summary: "Update an agent conversation", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam], request_body: {required: true, schema: {ref: "AgentConversationPatchRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentConversationResponse"}}], #errorResponses]), extensions: #agentUse},
	{method: "delete", path: "/workspaces/{workspace}/agent/conversations/{conversation}", operation_id: "archiveAgentConversation", summary: "Archive an agent conversation", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentConversationResponse"}}], #errorResponses]), extensions: #agentUse},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations/{conversation}/messages", operation_id: "listAgentMessages", summary: "List agent messages", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentMessageListResponse"}}], #errorResponses]), extensions: #agentRead},
	{method: "post", path: "/workspaces/{workspace}/agent/conversations/{conversation}/turns", operation_id: "createAgentTurn", summary: "Create an agent turn", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam], request_body: {required: true, schema: {ref: "AgentTurnRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentTurnResponse"}}], #errorResponses]), extensions: #agentUse},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations/{conversation}/runs", operation_id: "listAgentRuns", summary: "List agent runs", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentRunListResponse"}}], #errorResponses]), extensions: #agentRead},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}", operation_id: "getAgentRun", summary: "Get an agent run", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam, #runParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentRunResponse"}}], #errorResponses]), extensions: #agentRead},
	{method: "get", path: "/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events", operation_id: "listAgentEvents", summary: "List agent run events", tags: ["Agent"], security: #secure, parameters: [#workspaceParam, #conversationParam, #runParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AgentEventListResponse"}}], #errorResponses]), extensions: #agentRead},

	{method: "get", path: "/principals", operation_id: "listPrincipals", summary: "List principals", tags: ["Access"], security: #secure, parameters: [{name: "email", in: "query", description: "Filter by email.", schema: {type: "string"}}, {name: "q", in: "query", description: "Search query.", schema: {type: "string"}}, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "PrincipalListResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "get", path: "/principals/{principal}", operation_id: "getPrincipal", summary: "Get a principal", tags: ["Access"], security: #secure, parameters: [#principalParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "PrincipalResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "patch", path: "/principals/{principal}", operation_id: "updatePrincipal", summary: "Update a principal", tags: ["Access"], security: #secure, parameters: [#principalParam], request_body: {required: true, schema: {ref: "PrincipalPatchRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "PrincipalResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "get", path: "/workspaces/{workspace}/roles", operation_id: "listWorkspaceRoles", summary: "List workspace roles", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "RoleListResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "get", path: "/workspaces/{workspace}/groups", operation_id: "listGroups", summary: "List groups", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "GroupListResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "post", path: "/workspaces/{workspace}/groups", operation_id: "createGroup", summary: "Create a group", tags: ["Access"], security: #secure, parameters: [#workspaceParam], request_body: {required: true, schema: {ref: "GroupCreateRequest"}}, responses: list.Concat([[{status_code: 201, description: "created", schema: {ref: "GroupResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "get", path: "/workspaces/{workspace}/groups/{group}", operation_id: "getGroup", summary: "Get a group", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "GroupResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "patch", path: "/workspaces/{workspace}/groups/{group}", operation_id: "updateGroup", summary: "Update a group", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam], request_body: {required: true, schema: {ref: "GroupPatchRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "GroupResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "delete", path: "/workspaces/{workspace}/groups/{group}", operation_id: "deleteGroup", summary: "Delete a group", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "get", path: "/workspaces/{workspace}/groups/{group}/members", operation_id: "listGroupMembers", summary: "List group members", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "GroupMemberListResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "put", path: "/workspaces/{workspace}/groups/{group}/members/{principal}", operation_id: "addGroupMember", summary: "Add a group member", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam, #principalParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "delete", path: "/workspaces/{workspace}/groups/{group}/members/{principal}", operation_id: "removeGroupMember", summary: "Remove a group member", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #groupParam, #principalParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "get", path: "/workspaces/{workspace}/role-bindings", operation_id: "listRoleBindings", summary: "List role bindings", tags: ["Access"], security: #secure, parameters: [#workspaceParam, {name: "subjectType", in: "query", description: "Filter by subject type.", schema: {type: "string"}}, {name: "subjectId", in: "query", description: "Filter by subject ID.", schema: {type: "string"}}, {name: "role", in: "query", description: "Filter by role.", schema: {type: "string"}}, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "RoleBindingListResponse"}}], #errorResponses]), extensions: #rbacRead},
	{method: "post", path: "/workspaces/{workspace}/role-bindings", operation_id: "createRoleBinding", summary: "Create a role binding", tags: ["Access"], security: #secure, parameters: [#workspaceParam], request_body: {required: true, schema: {ref: "RoleBindingRequest"}}, responses: list.Concat([[{status_code: 201, description: "created", schema: {ref: "RoleBindingResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "patch", path: "/workspaces/{workspace}/role-bindings/{binding}", operation_id: "updateRoleBinding", summary: "Update a role binding", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #bindingParam], request_body: {required: true, schema: {ref: "RoleBindingRequest"}}, responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "RoleBindingResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "delete", path: "/workspaces/{workspace}/role-bindings/{binding}", operation_id: "deleteRoleBinding", summary: "Delete a role binding", tags: ["Access"], security: #secure, parameters: [#workspaceParam, #bindingParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "StatusResponse"}}], #errorResponses]), extensions: #rbacWrite},
	{method: "get", path: "/workspaces/{workspace}/audit-events", operation_id: "listAuditEvents", summary: "List workspace audit events", tags: ["Audit"], security: #secure, parameters: [#workspaceParam, {name: "actor", in: "query", description: "Filter by principal ID.", schema: {type: "string"}}, {name: "action", in: "query", description: "Filter by action.", schema: {type: "string"}}, {name: "targetType", in: "query", description: "Filter by target type.", schema: {type: "string"}}, {name: "targetId", in: "query", description: "Filter by target ID.", schema: {type: "string"}}, {name: "from", in: "query", description: "Filter from timestamp.", schema: {type: "string", format: "date-time"}}, {name: "to", in: "query", description: "Filter to timestamp.", schema: {type: "string", format: "date-time"}}, #limitParam, #cursorParam], responses: list.Concat([[{status_code: 200, description: "ok", schema: {ref: "AuditEventListResponse"}}], #errorResponses]), extensions: #auditRead},
]
