using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
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
  content?: Record<unknown>;
  toolCallId?: string;
  toolName?: string;
  isError?: boolean;
  createdAt: utcDateTime;
}

model AgentMessageListResponse {
  items: AgentMessageResponse[];
  page: PageInfo;
}

model AgentRunCreateRequest {
  input: string;
  correlationId?: string;
}

alias ListAgentConversationsOK = OkJson<AgentConversationListResponse>;

@tag("Agent")
@summary("List agent conversations")
@route("/workspaces/{workspace}/agent/conversations")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
@apigen.cli(#{ command: #["agent", "conversations"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listAgentConversations(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListAgentConversationsOK | CommonErrors;

alias CreateAgentConversationCreated = CreatedJson<AgentConversationResponse>;

@tag("Agent")
@summary("Create an agent conversation")
@route("/workspaces/{workspace}/agent/conversations")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op createAgentConversation(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: AgentConversationCreateRequest): CreateAgentConversationCreated | CommonErrors;

@tag("Agent")
@summary("Archive an agent conversation")
@route("/workspaces/{workspace}/agent/conversations/{conversation}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op archiveAgentConversation(@path workspace: string, @path conversation: string): NoContentResponse | CommonErrors;

alias GetAgentConversationOK = OkJson<AgentConversationResponse>;

@tag("Agent")
@summary("Get an agent conversation")
@route("/workspaces/{workspace}/agent/conversations/{conversation}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op getAgentConversation(@path workspace: string, @path conversation: string): GetAgentConversationOK | CommonErrors;

alias UpdateAgentConversationOK = OkJson<AgentConversationResponse>;

@tag("Agent")
@summary("Update an agent conversation")
@route("/workspaces/{workspace}/agent/conversations/{conversation}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op updateAgentConversation(@path workspace: string, @path conversation: string, @header("If-Match") ifMatch: string, @body body: AgentConversationPatchRequest): UpdateAgentConversationOK | CommonErrors;

alias ListAgentMessagesOK = OkJson<AgentMessageListResponse>;

@tag("Agent")
@summary("List agent messages")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/messages")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentMessages(@path workspace: string, @path conversation: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListAgentMessagesOK | CommonErrors;

alias ListAgentRunsOK = OkJson<AgentRunListResponse>;

@tag("Agent")
@summary("List agent runs")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentRuns(@path workspace: string, @path conversation: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListAgentRunsOK | CommonErrors;

alias GetAgentRunOK = OkJson<AgentRunResponse>;

@tag("Agent")
@summary("Get an agent run")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op getAgentRun(@path workspace: string, @path conversation: string, @path run: string): GetAgentRunOK | CommonErrors;

alias ListAgentEventsOK = OkJson<AsyncEventListResponse>;

@tag("Agent")
@summary("List agent run events")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentEvents(@path workspace: string, @path conversation: string, @path run: string, @header accept?: string, @header("Last-Event-ID") lastEventId?: string, @query limit?: ListLimit, @query pageToken?: PageToken): EventHistoryResponse<AsyncEventListResponse> | CommonErrors;

@tag("Agent")
@summary("Create an asynchronous agent run")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op createAgentRun(@path workspace: string, @path conversation: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: AgentRunCreateRequest): AcceptedJson<AgentRunResponse> | CommonErrors;

@tag("Agent")
@summary("Cancel an agent run")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/cancel")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op cancelAgentRun(@path workspace: string, @path conversation: string, @path run: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<AgentRunResponse> | CommonErrors;
