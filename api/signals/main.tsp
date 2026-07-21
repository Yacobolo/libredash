import "@yacobolo/apigen";

@apigen.`package`(#{
  title: "LeapView UI Signal Contracts",
  version: "1.0.0",
  description: "Typed Datastar payload contracts shared by the LeapView Go backend and Lit web components.",
})
namespace LeapViewUISignals;

/** Route surfaces supported by the shared UI signal transport. */
enum RouteKind {
  dashboard: "dashboard",
  catalog: "catalog",
  chat: "chat",
  workspace: "workspace",
  workspaceAsset: "workspace_asset",
  connections: "connections",
  connectionAsset: "connection_asset",
  data: "data",
  admin: "admin",
  login: "login",
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminAgentSignal {
  `canWrite`: boolean;
  `enabled`: boolean;
  `model`?: string;
  `systemPrompt`: string;
  `tools`: AdminAgentToolSignal[];
  `updatePath`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminAgentToolSignal {
  `defaults`: Record<unknown>;
  `description`: string;
  `effect`: string;
  `inputSchema`: Record<unknown>;
  `name`: string;
  `outputSchema`: Record<unknown>;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminContentSectionSignal {
  `facts`?: DefinitionFactSignal[];
  `table`?: RecordTableSignal;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminMetricSignal {
  `detail`?: string;
  `label`: string;
  `value`: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["admin", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "admin" })
model AdminPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: AdminPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminPageSignal {
  `active`: string;
  `agent`?: AdminAgentSignal;
  `empty`?: string;
  `headerDetail`: string;
  `headerTitle`: string;
  `kind`: RouteKind;
  `metrics`?: AdminMetricSignal[];
  `sections`?: AdminContentSectionSignal[];
  `sidebar`: SubSidebarSignal;
  `storage`?: AdminStorageSignal;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminQueryDetailSignal {
  `connectionWaitMs`: int64;
  `correlationId`?: string;
  `createdAt`?: string;
  `durationMs`: int64;
  `error`?: string;
  `eventId`?: string;
  `loading`: boolean;
  `modelId`?: string;
  `databaseMs`: int64;
  `objectId`?: string;
  `objectType`?: string;
  `operation`?: string;
  `planText`?: string;
  `planningMs`: int64;
  `principalId`?: string;
  `queryError`?: string;
  `queryJson`?: string;
  `queryKind`?: string;
  `requestId`?: string;
  `rowsReturned`: int64;
  `sql`?: string;
  `status`?: string;
  `statusLabel`?: string;
  `surface`?: string;
  `target`?: string;
  `workspaceId`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminQueryEventSignal {
  `correlationId`: string;
  `connectionWaitMs`: int64;
  `createdAt`: string;
  `databaseMs`: int64;
  `durationMs`: int64;
  `error`: string;
  `id`: string;
  `modelId`: string;
  `objectId`: string;
  `objectType`: string;
  `operation`: string;
  `planText`: string;
  `planningMs`: int64;
  `principalId`: string;
  `queryJson`: string;
  `queryKind`: string;
  `requestId`: string;
  `rowsReturned`: int64;
  `sql`: string;
  `status`: string;
  `surface`: string;
  `target`: string;
  `workspaceId`: string;
}

@apigen.contract(#{ kind: "ui-command", tags: #["admin", "command"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "command", `x-leapview-surface`: "admin" })
model AdminQueryHistoryCommand {
  `action`: string;
  `eventId`?: string;
  `filterMenu`?: FilterMenuCommand;
  `filters`: AdminQueryHistoryFilters;
  `limit`?: int64;
  `pageToken`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminQueryHistoryFilters {
  `from`?: string;
  `kinds`?: string[];
  `principals`?: string[];
  `search`?: string;
  `statuses`?: string[];
  `surfaces`?: string[];
  `target`?: string;
  `to`?: string;
  `workspaces`?: string[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminQueryHistorySignal {
  `error`: string;
  `filterMenus`?: FilterMenuSignal[];
  `filters`: AdminQueryHistoryFilters;
  `hasMore`: boolean;
  `limit`: int64;
  `loadedCountLabel`: string;
  `loading`: boolean;
  `nextCursor`: string;
  `table`: RecordTableSignal;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminStorageColumnSignal {
  `beginSnapshot`: int64;
  `containsNan`: string;
  `containsNull`: string;
  `default`: string;
  `defaultValueDialect`: string;
  `defaultValueType`: string;
  `extraStats`: string;
  `id`: int64;
  `initialDefault`: string;
  `maxValue`: string;
  `minValue`: string;
  `name`: string;
  `nullable`: string;
  `ordinal`: int64;
  `type`: string;
}

@apigen.contract(#{ kind: "ui-command", tags: #["admin", "command"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "command", `x-leapview-surface`: "admin" })
model AdminStorageCommand {
  `databaseId`: string;
  `schema`: string;
  `table`: string;
}

model AdminStorageFileSignal {
  `beginSnapshot`: int64;
  `endSnapshot`: int64;
  `format`: string;
  `id`: int64;
  `path`: string;
  `recordCount`: int64;
  `recordCountLabel`: string;
  `sizeBytes`: int64;
  `sizeLabel`: string;
}

model AdminStorageServingStateSignal {
  `activatedAt`: string;
  `active`: boolean;
  `digest`: string;
  `environment`: string;
  `servingStateId`: string;
  `snapshotId`: int64;
  `status`: string;
  `workspaceId`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminStorageSignal {
  `selectedKey`: string;
  `selectedTable`?: AdminStorageTableSignal;
  `servingStates`: AdminStorageServingStateSignal[];
  `snapshots`: AdminStorageSnapshotSignal[];
  `status`: string;
  `summary`: AdminStorageSummary;
  `tables`: AdminStorageTableSignal[];
  `warnings`: string[];
}

model AdminStorageSnapshotSignal {
  `author`: string;
  `changes`: string;
  `extraInfo`: string;
  `id`: int64;
  `message`: string;
  `protected`: boolean;
  `schemaVersion`: int64;
  `servingStateCount`: int64;
  `time`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminStorageSummary {
  `catalogPath`: string;
  `catalogSizeLabel`: string;
  `dataFileCount`: int64;
  `dataPath`: string;
  `dataSizeLabel`: string;
  `databaseCount`: int64;
  `snapshotCount`: int64;
  `tableCount`: int64;
  `totalDataSizeLabel`: string;
  `totalSizeLabel`: string;
}

model AdminStorageTableHistorySignal {
  `author`: string;
  `changes`: string;
  `extraInfo`: string;
  `message`: string;
  `schemaVersion`: int64;
  `snapshotId`: int64;
  `source`: string;
  `time`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["admin", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "admin" })
model AdminStorageTableSignal {
  `beginSnapshot`: int64;
  `columnCount`: int64;
  `columns`?: AdminStorageColumnSignal[];
  `databaseId`: string;
  `databaseName`: string;
  `databasePath`: string;
  `duckLakePath`: string;
  `endSnapshot`: int64;
  `fileCount`: int64;
  `files`?: AdminStorageFileSignal[];
  `history`?: AdminStorageTableHistorySignal[];
  `key`: string;
  `modelId`: string;
  `modelName`: string;
  `name`: string;
  `rowCount`: int64;
  `rowCountLabel`: string;
  `schema`: string;
  `servingStates`?: AdminStorageServingStateSignal[];
  `sizeBytes`: int64;
  `sizeLabel`: string;
  `tableId`: int64;
  `tableUuid`: string;
  `type`: string;
}

model AssetLineageEdgeSignal {
  `id`: string;
  `kind`: "visual" | "filter" | "header";
  `label`?: string;
  `source`: string;
  `target`: string;
}

model AssetLineageGraphSignal {
  `edges`: AssetLineageEdgeSignal[];
  `nodes`: AssetLineageNodeSignal[];
}

model AssetLineageNodeSignal {
  `containedCount`?: int64;
  `containedSummary`?: string;
  `href`?: string;
  `id`: string;
  `kind`: string;
  `label`: string;
  `meta`?: string;
  `rank`: int64;
  `selected`?: boolean;
  `side`: string;
  `usedByCount`?: int64;
  `usesCount`?: int64;
  `visibleDownstreamCount`?: int64;
  `visibleUpstreamCount`?: int64;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["catalog", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "catalog" })
model CatalogDashboardSignal {
  `description`?: string;
  `href`: string;
  `id`: string;
  `pageCount`: int64;
  `semanticModel`?: string;
  `tags`?: string[];
  `title`: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["catalog", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "catalog" })
model CatalogPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: CatalogPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["catalog", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "catalog" })
model CatalogPageSignal {
  `dashboards`: CatalogDashboardSignal[];
  `description`: string;
  `kind`: RouteKind;
  `title`: string;
}

model ChatArtifactSignal {
  `id`: string;
  `type`: string;
  `summary`?: string;
}

model AgentReferenceKeySignal {
  `workspaceId`: string;
  `type`: string;
  `id`: string;
}

model AgentReferenceWorkspaceSignal {
  `id`: string;
  `name`: string;
}

model AgentReferenceLocationSignal {
  `dashboardId`?: string;
  `dashboardName`?: string;
  `pageId`?: string;
  `pageName`?: string;
  `href`: string;
}

model AgentReferenceSignal {
  `reference`: AgentReferenceKeySignal;
  `name`: string;
  `description`?: string;
	`visualType`?: string;
  `workspace`: AgentReferenceWorkspaceSignal;
	`hierarchy`: string[];
  `href`: string;
  `locations`: AgentReferenceLocationSignal[];
  `context`: string[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["agent", "search", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "agent" })
model AgentReferenceSearchSignal {
  `query`: string;
  `requestId`: int64;
  `results`: AgentReferenceSignal[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["agent", "dashboard", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "dashboard" })
model AgentContextSignal {
  `surface`: string;
  `workspaceId`: string;
  `dashboardId`: string;
  `dashboardTitle`: string;
  `pageId`: string;
  `pageTitle`: string;
  `modelId`: string;
  `generation`: int64;
  `filters`: DashboardFilters;
  `referenceLimit`: int32;
  `references`: AgentReferenceSignal[];
}

model ChatConversationSummary {
  `archivedAt`?: string;
  `createdAt`: string;
  `id`: string;
  `lastMessageText`?: string;
  `messageCount`: int64;
  `principalId`: string;
  `status`: string;
  `title`: string;
  `titlePending`?: boolean;
  `updatedAt`: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["chat", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "chat" })
model ChatEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agent" })
  `agent`: ChatSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agentContext" })
  `agentContext`: AgentContextSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agentReferenceSearch" })
  `agentReferenceSearch`: AgentReferenceSearchSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: ChatPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "visuals" })
  `visuals`: Record<DashboardVisual>;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["chat", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "chat" })
model ChatPageSignal {
  `description`: string;
  `kind`: RouteKind;
  `title`: string;
  `view`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["chat", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "chat" })
model ChatSignal {
  `activeConversationId`: string;
  `composer`: ComposerSignal;
  `conversations`: ChatConversationSummary[];
  `status`: ChatStatus;
  `transcript`: ChatTranscriptItemSignal[];
}

model ChatStatus {
  `enabled`: boolean;
  `error`?: string;
  `running`: boolean;
}

model ChatTranscriptItemSignal {
  `argumentsJson`?: string;
  `artifact`?: ChatArtifactSignal;
  `conversationId`?: string;
  `createdAt`?: string;
  `error`?: string;
  `id`: string;
  `inputFormat`?: string;
  `inputJson`?: string;
  `kind`: string;
  `markdown`?: string;
  `name`?: string;
  `references`?: AgentReferenceSignal[];
  `resultFormat`?: string;
  `resultJson`?: string;
  `resultSummary`?: string;
  `runId`?: string;
  `status`?: string;
  `summary`?: string;
  `text`?: string;
  `title`?: string;
  `toolCallId`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model ChromeSignal {
  `sidebar`: SidebarSignal;
}

model ComposerSignal {
  `disabled`: boolean;
  `placeholder`: string;
  `value`: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["connections", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "connections" })
model ConnectionsPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: ConnectionsPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["connections", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "connections" })
model ConnectionsPageSignal {
  `assetList`?: WorkspaceAssetListSignal;
  `description`?: string;
  `environment`?: string;
  `kind`: RouteKind;
  `title`: string;
  `workspaceId`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["dashboard", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "dashboard" })
model DashboardComponentSignal {
  `badges`?: string[];
  `description`?: string;
  `eyebrow`?: string;
  `filter`?: string;
  `height`: float64;
  `id`: string;
  `kind`: string;
  `placement`: DashboardPagePlacement;
  `subtitle`?: string;
  `title`?: string;
  `visual`?: string;
  `width`: float64;
  `x`: float64;
  `y`: float64;
}

/** Progressive refresh state for one visual target. */
model DashboardComponentStatus {
  `error`: string;
  `generation`: int64;
  `loading`: boolean;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["dashboard", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "dashboard" })
model DashboardEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agent" })
  `agent`: ChatSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agentContext" })
  `agentContext`: AgentContextSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "agentReferenceSearch" })
  `agentReferenceSearch`: AgentReferenceSearchSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "agentVisuals" })
  `agentVisuals`: Record<DashboardVisual>;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "componentStatus" })
  `componentStatus`: Record<DashboardComponentStatus>;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "filterConfig" })
  `filterConfig`: ReportFilterConfig[];
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "filterOptions" })
  `filterOptions`: Record<DashboardFilterOption[]>;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "filters" })
  `filters`: DashboardFilters;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "interactionCommand" })
  `interactionCommand`: DashboardInteractionCommand;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: DashboardPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "visualWindowCommand" })
  `visualWindowCommand`: DashboardVisualWindowRequest;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "urlParamShape" })
  `urlParamShape`: Record<unknown>;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "urlParams" })
  `urlParams`: Record<unknown>;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "merge", `x-leapview-signal-key`: "visuals" })
  `visuals`: Record<DashboardVisual>;
}

model DashboardFilterControl {
  `from`?: string;
  `operator`?: string;
  `preset`?: string;
  `to`?: string;
  `type`: string;
  `value`?: string;
  `values`?: string[];
}

model DashboardFilterOption {
  `label`: string;
  `value`: string;
}

model DashboardFilters {
  `controls`: Record<DashboardFilterControl>;
  `selections`: DashboardInteractionSelection[];
}

model DashboardInteractionCommand {
  `action`: string;
  `interactionKind`: string;
  `mappings`: DashboardInteractionCommandMapping[];
  `sourceId`: string;
  `sourceKind`: "visual";
  `toggle`: boolean;
}

model DashboardInteractionCommandMapping {
  `fact`?: string;
  `field`: string;
  `grain`?: string;
  `label`?: string;
  /** A typed JSON scalar. Runtime command validation rejects arrays and objects. */
  `value`: unknown;
}

model DashboardInteractionConfig {
  `kind`: string;
  `mappings`: DashboardInteractionConfigMapping[];
  `targets`?: string[];
  `toggle`: boolean;
}

model DashboardInteractionConfigMapping {
  `fact`?: string;
  `field`: string;
  `grain`?: string;
  `label`?: string;
  `value`: string;
}

model DashboardInteractionSelection {
  `entries`: DashboardInteractionSelectionEntry[];
  `id`: string;
  `interactionKind`: string;
  `label`: string;
  `order`: int64;
  `sourceId`: string;
  `sourceKind`: "visual";
}

model DashboardInteractionSelectionEntry {
  `label`?: string;
  `mappings`: DashboardInteractionSelectionMapping[];
}

model DashboardInteractionSelectionMapping {
  `fact`?: string;
  `field`: string;
  `grain`?: string;
  `label`?: string;
  /** A typed JSON scalar. Runtime selection validation rejects arrays and objects. */
  `value`: unknown;
}

model DashboardPageCanvas {
  `height`: int64;
  `width`: int64;
}

model DashboardPageGrid {
  `columns`: int64;
  `gap`: int64;
  `padding`: int64;
  `rowHeight`: int64;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["dashboard", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "dashboard" })
model DashboardPageNavSignal {
  `active`: boolean;
  `href`: string;
  `id`: string;
  `title`: string;
}

model DashboardPagePlacement {
  `col`: int64;
  `colSpan`: int64;
  `row`: int64;
  `rowSpan`: int64;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["dashboard", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "dashboard" })
model DashboardPageSignal {
  `canvas`: DashboardPageCanvas;
  `components`: DashboardComponentSignal[];
  `dashboardId`: string;
  `dashboardTitle`: string;
  `description`?: string;
  `grid`: DashboardPageGrid;
  `headerDetail`: string;
  `kind`: RouteKind;
  `modelId`: string;
  `modelTitle`: string;
  `pageId`: string;
  `pageTitle`: string;
  `pages`: DashboardPageNavSignal[];
  `title`: string;
}

model DashboardStatus {
  `error`: string;
  `generation`: int64;
  `lastUpdated`: string;
  `loading`: boolean;
  /** Backend-owned dashboard refresh completion percentage from 0 through 100. */
  `progressPercent`: float64;
  `refreshId`: string;
  `setupRequired`: boolean;
}

model DashboardVisualCardinality {
  `kind`: "unknown" | "lower_bound" | "estimated" | "exact";
  `value`: int64;
}

model DashboardVisualBlock {
  `requestSeq`: int64;
  `resetVersion`: int64;
  `rows`: Record<unknown>[];
  `sort`: DashboardVisualSort;
  `start`: int64;
}

model DashboardVisualColumn {
  `align`?: string;
  `columnValue`?: string;
  `format`?: string;
  `formatting`?: DashboardVisualFormattingRule[];
  `group`?: string;
  `key`: string;
  `label`: string;
  `measure`?: string;
  `role`?: string;
  `width`?: int64;
}

model DashboardVisualFormattingRule {
  `background`?: string;
  `color`?: string;
  `highColor`?: string;
  `kind`: string;
  `lowColor`?: string;
  `max`?: float64;
  `min`?: float64;
  `values`?: Record<string>;
}

model DashboardVisualWindowRequest {
  `block`: string;
  `count`: int64;
  `requestSeq`: int64;
  `resetVersion`: int64;
  `sort`: DashboardVisualSort;
  `start`: int64;
  `visual`: string;
}

model DashboardVisualSort {
  `direction`: string;
  `key`: string;
}

model DashboardVisualStyle {
  `density`: string;
  `grid`: string;
  `zebra`: boolean;
}

@discriminator("type")
model DashboardVisual {
  `availableRows`?: int64;
  `blocks`?: Record<DashboardVisualBlock>;
  `cardinality`?: DashboardVisualCardinality;
  `chunkSize`?: int64;
  `columns`?: DashboardVisualColumn[];
  `data`?: Record<unknown>[];
  `dimensions`?: string[];
  `error`?: string;
  `format`?: string;
  `id`: string;
  `interaction`: DashboardInteractionConfig;
  `isCapped`?: boolean;
  `loadingBlock`?: string;
  `measure`?: string;
  `measures`?: string[];
  `options`?: Record<unknown>;
  `renderer`?: string;
  `rendererOptions`?: Record<Record<unknown>>;
  `resetVersion`?: int64;
  `rowCap`?: int64;
  `rowHeight`?: int64;
  `selection`: DashboardInteractionSelectionEntry[];
  `series`?: string[];
  `shape`?: string;
  `sort`?: DashboardVisualSort;
  `style`?: DashboardVisualStyle;
  `title`: string;
  `type`: string;
  `version`: int64;
  `unit`: string;
}

model LineDashboardVisual extends DashboardVisual { `type`: "line"; }
model AreaDashboardVisual extends DashboardVisual { `type`: "area"; }
model BarDashboardVisual extends DashboardVisual { `type`: "bar"; }
model ColumnDashboardVisual extends DashboardVisual { `type`: "column"; }
model PieDashboardVisual extends DashboardVisual { `type`: "pie"; }
model DonutDashboardVisual extends DashboardVisual { `type`: "donut"; }
model ScatterDashboardVisual extends DashboardVisual { `type`: "scatter"; }
model FunnelDashboardVisual extends DashboardVisual { `type`: "funnel"; }
model TreemapDashboardVisual extends DashboardVisual { `type`: "treemap"; }
model GaugeDashboardVisual extends DashboardVisual { `type`: "gauge"; }
model HeatmapDashboardVisual extends DashboardVisual { `type`: "heatmap"; }
model SankeyDashboardVisual extends DashboardVisual { `type`: "sankey"; }
model GraphDashboardVisual extends DashboardVisual { `type`: "graph"; }
model MapDashboardVisual extends DashboardVisual { `type`: "map"; }
model CandlestickDashboardVisual extends DashboardVisual { `type`: "candlestick"; }
model BoxplotDashboardVisual extends DashboardVisual { `type`: "boxplot"; }
model ComboDashboardVisual extends DashboardVisual { `type`: "combo"; }
model WaterfallDashboardVisual extends DashboardVisual { `type`: "waterfall"; }
model HistogramDashboardVisual extends DashboardVisual { `type`: "histogram"; }
model RadarDashboardVisual extends DashboardVisual { `type`: "radar"; }
model TreeDashboardVisual extends DashboardVisual { `type`: "tree"; }
model SunburstDashboardVisual extends DashboardVisual { `type`: "sunburst"; }
model KPIDashboardVisual extends DashboardVisual { `type`: "kpi"; }
model TableDashboardVisual extends DashboardVisual { `type`: "table"; }
model MatrixDashboardVisual extends DashboardVisual { `type`: "matrix"; }
model PivotDashboardVisual extends DashboardVisual { `type`: "pivot"; }

@apigen.contract(#{ kind: "ui-command", tags: #["data", "command"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "command", `x-leapview-surface`: "data" })
model DataExplorerCommand {
  `block`?: string;
  `columnWidths`?: Record<float64>;
  `count`: int64;
  `limit`: int64;
  `objectKey`?: string;
  `offset`: int64;
  `requestSeq`: int64;
  `resetVersion`: int64;
  `sort`: DataPreviewSortSignal;
  `start`: int64;
  `visibleColumns`?: string[];
  `workspaceId`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataExplorerObjectSignal {
  `assetId`?: string;
  `columnCount`: int64;
  `columns`?: DataPreviewColumnSignal[];
  `description`?: string;
  `detailHref`?: string;
  `key`: string;
  `layer`: string;
  `modelId`?: string;
  `rowCountLabel`?: string;
  `source`?: string;
  `table`?: string;
  `title`: string;
  `workspaceId`: string;
  `workspaceTitle`?: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["data", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "data" })
model DataExplorerPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "dataExplorer" })
  `dataExplorer`: DataExplorerSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: DataExplorerPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataExplorerPageSignal {
  `description`?: string;
  `kind`: RouteKind;
  `selectedObject`?: string;
  `selectedWorkspaceId`?: string;
  `tabs`: WorkspaceTabSignal[];
  `title`: string;
  `workspaceId`?: string;
  `workspaces`: DataExplorerWorkspaceSignal[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataExplorerSignal {
  `command`: DataExplorerCommand;
  `objects`: DataExplorerObjectSignal[];
  `preview`: DataPreviewSignal;
  `selectedKey`?: string;
  `selectedObject`?: DataExplorerObjectSignal;
  `selectedWorkspaceId`?: string;
  `warnings`?: string[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataExplorerWorkspaceSignal {
  `active`: boolean;
  `href`: string;
  `id`: string;
  `objectCount`: int64;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataPreviewBlockSignal {
  `requestSeq`: int64;
  `resetVersion`: int64;
  `rows`: Record<unknown>[];
  `sort`: DataPreviewSortSignal;
  `start`: int64;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataPreviewColumnSignal {
  `key`: string;
  `label`: string;
  `type`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataPreviewSignal {
  `availableRows`: int64;
  `blocks`: Record<DataPreviewBlockSignal>;
  `chunkSize`: int64;
  `columns`: DataPreviewColumnSignal[];
  `error`?: string;
  `loadingBlock`?: string;
  `resetVersion`: int64;
  `rowHeight`: int64;
  `sort`: DataPreviewSortSignal;
  `sql`?: string;
  `totalRowLabel`?: string;
  `totalRows`: int64;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["data", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "data" })
model DataPreviewSortSignal {
  `column`?: string;
  `direction`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model DefinitionFactSignal {
  `code`?: boolean;
  `label`: string;
  `value`: string;
  `wide`?: boolean;
}

@apigen.contract(#{ kind: "ui-command", tags: #["shared", "command"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "command", `x-leapview-surface`: "shared" })
model FilterMenuCommand {
  `action`?: string;
  `menuId`?: string;
  `search`?: string;
  `selected`?: string[];
  `value`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model FilterMenuOptionSignal {
  `countLabel`?: string;
  `description`?: string;
  `disabled`: boolean;
  `icon`?: string;
  `label`: string;
  `selected`: boolean;
  `value`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model FilterMenuSignal {
  `emptyLabel`?: string;
  `error`?: string;
  `id`: string;
  `label`: string;
  `loading`: boolean;
  `mode`?: string;
  `options`?: FilterMenuOptionSignal[];
  `placeholder`?: string;
  `search`?: string;
  `selected`?: string[];
  `summaryLabel`?: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["login", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "login" })
model LoginPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: LoginPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["login", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "login" })
model LoginPageSignal {
  `backgroundModuleSrc`: string;
  `kind`: RouteKind;
  `localAuth`: boolean;
  `mustChangePassword`: boolean;
  `providerLabel`: string;
  `ssoAuth`: boolean;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model RecordTableBadgeSignal {
  `label`: string;
  `tone`?: string;
}

model RecordTableColumnSelector {
  `defaultColumns`?: string[];
  `enabled`: boolean;
  `label`?: string;
  `storageKey`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model RecordTableColumnSignal {
  `align`?: string;
  `header`: string;
  `hrefKey`?: string;
  `id`: string;
  `kind`?: string;
  `toggleable`?: boolean;
  `width`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model RecordTableSignal {
  `columnSelector`?: RecordTableColumnSelector;
  `columns`: RecordTableColumnSignal[];
  `density`?: string;
  `empty`: string;
  `minWidth`?: string;
  `rowAction`?: string;
  `rows`: Record<unknown>[];
}

model ReportFilterConfig {
  `custom`?: boolean;
  `default`: ReportFilterDefault;
  `defaultOperator`?: string;
  `description`?: string;
  `dimension`: string;
  `fact`?: string;
  `fromURLParam`?: string;
  `id`: string;
  `label`: string;
  `operator`?: string;
  `operatorURLParam`?: string;
  `operators`?: string[];
  `options`?: ReportFilterOption[];
  `presets`?: ReportFilterPreset[];
  `targets`?: ReportFilterTargets;
  `toURLParam`?: string;
  `type`: string;
  `urlParam`?: string;
  `values`?: ReportFilterValues;
}

model ReportFilterDefault {
  `from`?: string;
  `operator`?: string;
  `preset`?: string;
  `to`?: string;
  `value`?: string;
  `values`?: string[];
}

model ReportFilterDefinition {
  `custom`?: boolean;
  `default`: ReportFilterDefault;
  `defaultOperator`?: string;
  `description`?: string;
  `dimension`: string;
  `fact`?: string;
  `fromURLParam`?: string;
  `label`: string;
  `operator`?: string;
  `operatorURLParam`?: string;
  `operators`?: string[];
  `options`?: ReportFilterOption[];
  `presets`?: ReportFilterPreset[];
  `targets`?: ReportFilterTargets;
  `toURLParam`?: string;
  `type`: string;
  `urlParam`?: string;
  `values`?: ReportFilterValues;
}

model ReportFilterOption {
  `label`: string;
  `value`: string;
}

model ReportFilterPreset {
  `from`?: string;
  `label`: string;
  `relativeDays`?: int64;
  `to`?: string;
  `value`: string;
}

model ReportFilterTargets {
  `visuals`?: string[];
}

model ReportFilterValues {
  `limit`?: int64;
  `source`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model RouteRuntimeSignal {
  `clientId`?: string;
  `dashboardId`?: string;
  `kind`: RouteKind;
  `modelId`?: string;
  `pageId`?: string;
  `streamInstanceId`?: string;
  `workspaceId`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model SemanticModelGraphEdgeSignal {
  `cardinality`: string;
  `id`: string;
  `label`: string;
  `source`: string;
  `sourceField`: string;
  `target`: string;
  `targetField`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model SemanticModelGraphFieldSignal {
  `join`?: boolean;
  `label`?: string;
  `name`: string;
  `primaryKey`?: boolean;
  `relationships`?: string[];
  `type`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model SemanticModelGraphNodeSignal {
  `badges`?: string[];
  `description`?: string;
  `fields`: SemanticModelGraphFieldSignal[];
  `id`: string;
  `primaryKey`?: string;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model SemanticModelGraphSignal {
  `edges`: SemanticModelGraphEdgeSignal[];
  `facts`?: string[];
  `nodes`: SemanticModelGraphNodeSignal[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarActionSignal {
  `href`: string;
  `icon`: string;
  `label`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarGroupSignal {
  `items`: SidebarItemSignal[];
  `label`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarHistoryItemSignal {
  `active`: boolean;
  `href`: string;
  `id`: string;
  `pending`?: boolean;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarHistorySignal {
  `emptyText`?: string;
  `items`: SidebarHistoryItemSignal[];
  `label`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarItemSignal {
  `active`?: boolean;
  `href`: string;
  `icon`: string;
  `id`: string;
  `label`: string;
  `meta`?: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SidebarSignal {
  `active`: string;
  `compact`: boolean;
  `dashboardId`?: string;
  `dashboardTitle`: string;
  `groups`: SidebarGroupSignal[];
  `history`?: SidebarHistorySignal;
  `modelId`?: string;
  `modelTitle`?: string;
  `pageTitle`: string;
  `primaryAction`?: SidebarActionSignal;
  `userRole`?: string;
  `workspaceTitle`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SubSidebarItemSignal {
  `active`: boolean;
  `href`: string;
  `id`: string;
  `pending`?: boolean;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["shared", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "shared" })
model SubSidebarSignal {
  `activeId`: string;
  `ariaLabel`: string;
  `collapsible`: boolean;
  `disabled`?: boolean;
  `emptyText`?: string;
  `items`: SubSidebarItemSignal[];
  `label`: string;
  `numbered`: boolean;
  `railLabel`: string;
  `storageKey`: string;
}

model WorkspaceAccessCommand {
  `bindingId`: string;
  `email`: string;
  `principalId`: string;
  `privilege`: string;
  `role`: string;
  `subjectId`: string;
  `subjectType`: string;
}

model WorkspaceAccessCandidate {
  `detail`: string;
  `label`: string;
  `subjectId`: string;
  `subjectType`: string;
}

model WorkspaceAccessSearchStatus {
  `error`: string;
  `loading`: boolean;
}

model WorkspaceAccessResponse {
  `bindings`: unknown[];
  `candidates`: WorkspaceAccessCandidate[];
  `canManage`: boolean;
  `mode`?: string;
  `objectId`?: string;
  `objectTitle`?: string;
  `objectType`?: string;
  `roles`: unknown[];
  `search`: string;
  `searchStatus`: WorkspaceAccessSearchStatus;
  `status`: WorkspaceAccessStatus;
  `workspace`: unknown;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAccessSignal {
  `bindings`: unknown[];
  `candidates`: WorkspaceAccessCandidate[];
  `canManage`: boolean;
  `command`: WorkspaceAccessCommand;
  `mode`?: string;
  `objectId`?: string;
  `objectTitle`?: string;
  `objectType`?: string;
  `roles`: unknown[];
  `search`: string;
  `searchStatus`: WorkspaceAccessSearchStatus;
  `status`: WorkspaceAccessStatus;
  `workspace`: unknown;
}

model WorkspaceAccessStatus {
  `error`: string;
  `loading`: boolean;
  `message`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceActionSignal {
  `command`?: string;
  `disabled`?: boolean;
  `href`?: string;
  `icon`?: string;
  `label`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAssetDetailsSignal {
  `overview`: DefinitionFactSignal[];
  `sections`: WorkspaceDetailSectionSignal[];
  `semanticModelGraph`?: SemanticModelGraphSignal;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAssetLineageSignal {
  `count`: int64;
  `graph`: AssetLineageGraphSignal;
  `usedByTable`: RecordTableSignal;
  `usesTable`: RecordTableSignal;
}

model WorkspaceAssetListSignal {
  `activeType`?: string;
  `assets`: WorkspaceAssetSummarySignal[];
  `empty`: string;
  `query`?: string;
  `searchHref`: string;
  `tabs`: WorkspaceTabSignal[];
  `workspaceId`?: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["workspace", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "workspace" })
model WorkspaceAssetPageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: WorkspaceAssetPageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAssetPageSignal {
  `actions`?: WorkspaceActionSignal[];
  `activeSection`: string;
  `asset`: WorkspaceAssetSummarySignal;
  `assetId`: string;
  `breadcrumbs`: WorkspaceBreadcrumbSignal[];
  `details`?: WorkspaceAssetDetailsSignal;
  `environment`?: string;
  `kind`: RouteKind;
  `lineage`?: WorkspaceAssetLineageSignal;
  `refresh`?: WorkspaceAssetRefreshSignal;
  `tabs`: WorkspaceTabSignal[];
  `title`: string;
  `versions`?: WorkspaceAssetVersionsSignal;
  `workspaceId`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAssetRefreshSignal {
  `lastSuccessful`: string;
  `running`: boolean;
  `runsTable`?: RecordTableSignal;
  `status`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceAssetSummarySignal {
  `description`?: string;
  `detailHref`: string;
  `id`: string;
  `key`: string;
  `openHref`: string;
  `parentHref`?: string;
  `parentTitle`?: string;
  `title`: string;
  `type`: string;
  `typeLabel`: string;
}

model WorkspaceAssetVersionsSignal {
  `currentContentHash`: string;
  `table`: RecordTableSignal;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceBreadcrumbSignal {
  `current`?: boolean;
  `href`?: string;
  `label`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceCatalogItemSignal {
  `description`: string;
  `href`: string;
  `id`: string;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceDetailSectionSignal {
  `code`?: string;
  `facts`?: DefinitionFactSignal[];
  `lang`?: string;
  `table`?: RecordTableSignal;
  `title`: string;
}

@apigen.contract(#{ kind: "ui-envelope", tags: #["workspace", "envelope"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "envelope", `x-leapview-surface`: "workspace" })
model WorkspacePageEnvelope {
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "chrome" })
  `chrome`: ChromeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "page" })
  `page`: WorkspacePageSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "runtime" })
  `runtime`: RouteRuntimeSignal;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "status" })
  `status`: DashboardStatus;
  @apigen.`metadata`(#{ `x-leapview-patch-mode`: "replace", `x-leapview-signal-key`: "workspaceAccess" })
  `workspaceAccess`?: WorkspaceAccessSignal;
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspacePageSignal {
  `assetList`?: WorkspaceAssetListSignal;
  `description`?: string;
  `environment`?: string;
  `kind`: RouteKind;
  `title`: string;
  `workspaceId`?: string;
  `workspaces`?: WorkspaceCatalogItemSignal[];
}

@apigen.contract(#{ kind: "ui-signal", tags: #["workspace", "signal"] })
@apigen.`metadata`(#{ `x-leapview-contract-role`: "signal", `x-leapview-surface`: "workspace" })
model WorkspaceTabSignal {
  `active`: boolean;
  `count`?: int64;
  `href`: string;
  `id`: string;
  `label`: string;
}
