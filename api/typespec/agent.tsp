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

model AdminAgentToolResponse {
  name: string;
  description: string;
  inputSchema: Record<string>;
}

model AdminAgentResponse {
  enabled: boolean;
  "model"?: string;
  systemPrompt: string;
  tools: AdminAgentToolResponse[];
}

model AdminAgentConfigPatchRequest {
  systemPrompt: string;
}

alias GetAdminAgentConfigOK = OkJson<AdminAgentResponse>;

@tag("Agent")
@summary("Get admin agent configuration")
@route("/admin/agent/config")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getAdminAgentConfig(): GetAdminAgentConfigOK | CommonErrors;

alias UpdateAdminAgentConfigOK = OkJson<AdminAgentResponse>;

@tag("Agent")
@summary("Update admin agent configuration")
@route("/admin/agent/config")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updateAdminAgentConfig(@body body: AdminAgentConfigPatchRequest): UpdateAdminAgentConfigOK | CommonErrors;

alias ListAgentConversationsOK = OkJson<AgentConversationListResponse>;

@tag("Agent")
@summary("List agent conversations")
@route("/workspaces/{workspace}/agent/conversations")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
@apigen.cli(#{ command: #["agent", "conversations"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listAgentConversations(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListAgentConversationsOK | CommonErrors;

alias CreateAgentConversationCreated = CreatedJson<AgentConversationResponse>;

@tag("Agent")
@summary("Create an agent conversation")
@route("/workspaces/{workspace}/agent/conversations")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op createAgentConversation(@path workspace: string, @body body: AgentConversationCreateRequest): CreateAgentConversationCreated | CommonErrors;

alias ArchiveAgentConversationOK = OkJson<AgentConversationResponse>;

@tag("Agent")
@summary("Archive an agent conversation")
@route("/workspaces/{workspace}/agent/conversations/{conversation}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op archiveAgentConversation(@path workspace: string, @path conversation: string): ArchiveAgentConversationOK | CommonErrors;

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
op updateAgentConversation(@path workspace: string, @path conversation: string, @body body: AgentConversationPatchRequest): UpdateAgentConversationOK | CommonErrors;

alias ListAgentMessagesOK = OkJson<AgentMessageListResponse>;

@tag("Agent")
@summary("List agent messages")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/messages")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentMessages(@path workspace: string, @path conversation: string, @query limit?: int32, @query pageToken?: string): ListAgentMessagesOK | CommonErrors;

alias ListAgentRunsOK = OkJson<AgentRunListResponse>;

@tag("Agent")
@summary("List agent runs")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentRuns(@path workspace: string, @path conversation: string, @query limit?: int32, @query pageToken?: string): ListAgentRunsOK | CommonErrors;

alias GetAgentRunOK = OkJson<AgentRunResponse>;

@tag("Agent")
@summary("Get an agent run")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op getAgentRun(@path workspace: string, @path conversation: string, @path run: string): GetAgentRunOK | CommonErrors;

alias ListAgentEventsOK = OkJson<AgentEventListResponse>;

@tag("Agent")
@summary("List agent run events")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_AGENT" })
op listAgentEvents(@path workspace: string, @path conversation: string, @path run: string, @query limit?: int32, @query pageToken?: string): ListAgentEventsOK | CommonErrors;

alias CreateAgentTurnOK = OkJson<AgentTurnResponse>;

@tag("Agent")
@summary("Create an agent turn")
@route("/workspaces/{workspace}/agent/conversations/{conversation}/turns")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_AGENT" })
op createAgentTurn(@path workspace: string, @path conversation: string, @body body: AgentTurnRequest): CreateAgentTurnOK | CommonErrors;
