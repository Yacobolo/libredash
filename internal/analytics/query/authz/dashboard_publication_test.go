package authz

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func TestDashboardPublicationCapabilityAllowsOnlyCompiledDashboardDependencies(t *testing.T) {
	capability := DashboardPublicationCapability{
		WorkspaceID: "visuals", Publication: "website", Dashboard: "showcase", ModelID: "visuals",
		DependencyAssetIDs: []string{
			"dashboard:visuals.showcase",
			"semantic_model:visuals.visuals",
			"semantic_table:visuals.visuals.orders",
			"field:visuals.visuals.orders.status",
			"measure:visuals.visuals.order_count",
		},
	}
	request := dataquery.Query{
		WorkspaceID: "visuals", Surface: dataquery.SurfacePublicDashboard, Operation: dataquery.OperationDashboardAggregate,
		ModelID: "visuals", Kind: dataquery.KindSemanticAggregate,
	}
	objects := []access.ObjectRef{
		access.ItemObject(access.SecurableSemanticModel, "visuals", "visuals"),
		access.ItemObject(access.SecurableSemanticField, "visuals", "visuals/orders.status"),
		access.ItemObject(access.SecurableSemanticField, "visuals", "visuals/order_count"),
		access.ItemObject(access.SecurableDataset, "visuals", "visuals/orders"),
		access.ItemObject(access.SecurableColumn, "visuals", "visuals/orders/status"),
	}
	if err := validateDashboardPublicationQuery(capability, request, objects); err != nil {
		t.Fatalf("validateDashboardPublicationQuery() error = %v", err)
	}
}

func TestDashboardPublicationCapabilityRejectsExpansion(t *testing.T) {
	base := DashboardPublicationCapability{
		WorkspaceID: "visuals", Publication: "website", Dashboard: "showcase", ModelID: "visuals",
		DependencyAssetIDs: []string{"dashboard:visuals.showcase", "semantic_model:visuals.visuals", "semantic_table:visuals.visuals.orders", "field:visuals.visuals.orders.status"},
	}
	request := dataquery.Query{WorkspaceID: "visuals", Surface: dataquery.SurfacePublicDashboard, Operation: dataquery.OperationDashboardRows, ModelID: "visuals", Kind: dataquery.KindSemanticRows}
	tests := []struct {
		name    string
		modify  func(*DashboardPublicationCapability, *dataquery.Query)
		objects []access.ObjectRef
	}{
		{name: "workspace", modify: func(_ *DashboardPublicationCapability, q *dataquery.Query) { q.WorkspaceID = "other" }},
		{name: "surface", modify: func(_ *DashboardPublicationCapability, q *dataquery.Query) { q.Surface = dataquery.SurfaceAPI }},
		{name: "model", modify: func(_ *DashboardPublicationCapability, q *dataquery.Query) { q.ModelID = "other" }},
		{name: "operation", modify: func(_ *DashboardPublicationCapability, q *dataquery.Query) { q.Operation = dataquery.OperationAPIQuery }},
		{name: "kind", modify: func(_ *DashboardPublicationCapability, q *dataquery.Query) { q.Kind = dataquery.KindModelTableRows }},
		{name: "field", objects: []access.ObjectRef{access.ItemObject(access.SecurableColumn, "visuals", "visuals/orders/secret")}},
		{name: "semantic field", objects: []access.ObjectRef{access.ItemObject(access.SecurableSemanticField, "visuals", "visuals/secret_measure")}},
		{name: "dataset", objects: []access.ObjectRef{access.ItemObject(access.SecurableDataset, "visuals", "visuals/customers")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := base
			candidate := request
			if tt.modify != nil {
				tt.modify(&capability, &candidate)
			}
			if err := validateDashboardPublicationQuery(capability, candidate, tt.objects); err == nil {
				t.Fatal("validation accepted capability expansion")
			}
		})
	}
}
