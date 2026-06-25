using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model AgentConversationCreateRequest {
  title?: string;
}

model AgentConversationPatchRequest {
  title?: string;
}

model AgentConversationResponse {
  id: string;
  workspaceId: string;
  principalId: string;
  title: string;
  status: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
  archivedAt?: utcDateTime;
  messageCount?: int32;
  lastMessageText?: string;
  titlePending?: boolean;
}

model AgentConversationListResponse {
  items: AgentConversationResponse[];
  page: PageInfo;
}

model AgentRunResponse {
  id: string;
  conversationId: string;
  workspaceId: string;
  principalId: string;
  status: string;
  "model"?: string;
  stopReason?: string;
  createdAt: utcDateTime;
  startedAt?: utcDateTime;
  completedAt?: utcDateTime;
  error?: string;
}

model AgentRunListResponse {
  items: AgentRunResponse[];
  page: PageInfo;
}

model AgentMessageResponse {
  id: string;
  runId?: string;
  seq: int64;
  role: string;
  contentText?: string;
  content?: Record<string>;
  toolCallId?: string;
  toolName?: string;
  isError?: boolean;
  createdAt: utcDateTime;
}

model AgentMessageListResponse {
  items: AgentMessageResponse[];
  page: PageInfo;
}

model AgentTurnRequest {
  input: string;
  correlationId?: string;
}

model AgentTurnResponse {
  conversationId: string;
  runId: string;
  stopReason: string;
  content: string;
}

model AgentEventResponse {
  id: string;
  runId: string;
  seq: int64;
  eventType: string;
  severity: string;
  payload: Record<string>;
  createdAt: utcDateTime;
}

model AgentEventListResponse {
  items: AgentEventResponse[];
  page: PageInfo;
}

model ListAgentConversationsOK {
  @statusCode statusCode: 200;
  @body body: AgentConversationListResponse;
}

@tag("Agent")
@summary("List agent conversations")
@route("/api/v1/workspaces/{workspace}/agent/conversations")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
@apigen.cli(#{ command: #["agent", "conversations"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listAgentConversations(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListAgentConversationsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateAgentConversationCreated {
  @statusCode statusCode: 201;
  @body body: AgentConversationResponse;
}

@tag("Agent")
@summary("Create an agent conversation")
@route("/api/v1/workspaces/{workspace}/agent/conversations")
@post
@apigen.authz(#{ mode: "permission", permission: "agent:use" })
op createAgentConversation(@path workspace: string, @body body: AgentConversationCreateRequest): CreateAgentConversationCreated | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ArchiveAgentConversationOK {
  @statusCode statusCode: 200;
  @body body: AgentConversationResponse;
}

@tag("Agent")
@summary("Archive an agent conversation")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}")
@delete
@apigen.authz(#{ mode: "permission", permission: "agent:use" })
op archiveAgentConversation(@path workspace: string, @path conversation: string): ArchiveAgentConversationOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetAgentConversationOK {
  @statusCode statusCode: 200;
  @body body: AgentConversationResponse;
}

@tag("Agent")
@summary("Get an agent conversation")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
op getAgentConversation(@path workspace: string, @path conversation: string): GetAgentConversationOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model UpdateAgentConversationOK {
  @statusCode statusCode: 200;
  @body body: AgentConversationResponse;
}

@tag("Agent")
@summary("Update an agent conversation")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}")
@patch
@apigen.authz(#{ mode: "permission", permission: "agent:use" })
op updateAgentConversation(@path workspace: string, @path conversation: string, @body body: AgentConversationPatchRequest): UpdateAgentConversationOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListAgentMessagesOK {
  @statusCode statusCode: 200;
  @body body: AgentMessageListResponse;
}

@tag("Agent")
@summary("List agent messages")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/messages")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
op listAgentMessages(@path workspace: string, @path conversation: string, @query limit?: int32, @query pageToken?: string): ListAgentMessagesOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListAgentRunsOK {
  @statusCode statusCode: 200;
  @body body: AgentRunListResponse;
}

@tag("Agent")
@summary("List agent runs")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
op listAgentRuns(@path workspace: string, @path conversation: string, @query limit?: int32, @query pageToken?: string): ListAgentRunsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetAgentRunOK {
  @statusCode statusCode: 200;
  @body body: AgentRunResponse;
}

@tag("Agent")
@summary("Get an agent run")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
op getAgentRun(@path workspace: string, @path conversation: string, @path run: string): GetAgentRunOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListAgentEventsOK {
  @statusCode statusCode: 200;
  @body body: AgentEventListResponse;
}

@tag("Agent")
@summary("List agent run events")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events")
@get
@apigen.authz(#{ mode: "permission", permission: "agent:read" })
op listAgentEvents(@path workspace: string, @path conversation: string, @path run: string, @query limit?: int32, @query pageToken?: string): ListAgentEventsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateAgentTurnOK {
  @statusCode statusCode: 200;
  @body body: AgentTurnResponse;
}

@tag("Agent")
@summary("Create an agent turn")
@route("/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/turns")
@post
@apigen.authz(#{ mode: "permission", permission: "agent:use" })
op createAgentTurn(@path workspace: string, @path conversation: string, @body body: AgentTurnRequest): CreateAgentTurnOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
