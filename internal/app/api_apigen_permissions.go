package app

import (
	"github.com/Yacobolo/libredash/internal/access/httpauth"
	agenthttp "github.com/Yacobolo/libredash/internal/agent/http"
	queryhttp "github.com/Yacobolo/libredash/internal/analytics/query/http"
	dashboardhttp "github.com/Yacobolo/libredash/internal/dashboard/http"
	workspacehttp "github.com/Yacobolo/libredash/internal/workspace/http"
)

var apigenOperationObjectResolvers = map[string]httpauth.ObjectResolver{
	"getWorkspaceAsset":         workspacehttp.AssetObjectRefs,
	"getWorkspaceAssetLineage":  workspacehttp.AssetObjectRefs,
	"getDashboard":              dashboardhttp.DashboardObjectRefs,
	"getDashboardPage":          dashboardhttp.DashboardObjectRefs,
	"getDashboardTable":         dashboardhttp.DashboardObjectRefs,
	"getDashboardFilter":        dashboardhttp.DashboardObjectRefs,
	"getDashboardVisual":        dashboardhttp.DashboardObjectRefs,
	"queryDashboardPage":        dashboardhttp.DashboardObjectRefs,
	"queryDashboardVisualData":  dashboardhttp.DashboardObjectRefs,
	"queryDashboardTable":       dashboardhttp.DashboardObjectRefs,
	"listDashboardFilterValues": dashboardhttp.DashboardObjectRefs,
	"getSemanticModel":          queryhttp.SemanticDatasetObjectRefs,
	"listSemanticModelFields":   queryhttp.SemanticDatasetObjectRefs,
	"querySemanticModel":        queryhttp.SemanticDatasetObjectRefs,
	"explainSemanticModelQuery": queryhttp.SemanticDatasetObjectRefs,
	"listSemanticDatasets":      queryhttp.SemanticDatasetObjectRefs,
	"getSemanticDataset":        queryhttp.SemanticDatasetObjectRefs,
	"listSemanticFields":        queryhttp.SemanticDatasetObjectRefs,
	"previewSemanticDataset":    queryhttp.SemanticDatasetObjectRefs,
	"explainSemanticPreview":    queryhttp.SemanticDatasetObjectRefs,
	"listSemanticRelationships": queryhttp.SemanticDatasetObjectRefs,
	"listSemanticSources":       queryhttp.SemanticDatasetObjectRefs,
	"getAgentConversation":      agenthttp.ConversationObjectRefs,
	"updateAgentConversation":   agenthttp.ConversationObjectRefs,
	"archiveAgentConversation":  agenthttp.ConversationObjectRefs,
	"listAgentMessages":         agenthttp.ConversationObjectRefs,
	"createAgentRun":            agenthttp.ConversationObjectRefs,
	"listAgentRuns":             agenthttp.ConversationObjectRefs,
	"getAgentRun":               agenthttp.ConversationObjectRefs,
	"listAgentEvents":           agenthttp.ConversationObjectRefs,
	"cancelAgentRun":            agenthttp.ConversationObjectRefs,
}
