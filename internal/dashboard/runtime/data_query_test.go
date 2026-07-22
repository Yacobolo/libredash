package runtime

import (
	"testing"

	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func TestReportAggregateDataQueryDefaultsToDashboardCacheOperation(t *testing.T) {
	request := reportAggregateDataQuery("sales", reportdef.AggregateQuery{
		Dimensions: []reportdef.QueryField{{Field: "region"}},
	})
	if request.Surface != dataquery.SurfaceDashboard {
		t.Fatalf("surface = %q, want dashboard", request.Surface)
	}
	if request.Operation != dataquery.OperationDashboardAggregate {
		t.Fatalf("operation = %q, want dashboard aggregate", request.Operation)
	}
}

func TestReportCountDataQueryPreservesAuthorizationProjection(t *testing.T) {
	request := reportRowDataQuery("sales", reportdef.RowQuery{
		Table: "orders",
		Dimensions: []reportdef.QueryField{
			{Field: "orders.order_id", Alias: "order_id"},
			{Field: "orders.customer_email", Alias: "email"},
		},
		Measures: []reportdef.QueryField{{Field: "order_value", Alias: "value"}},
	}, true)
	request = countOnlyDataQuery(request)

	if request.Operation != dataquery.OperationDashboardCount {
		t.Fatalf("operation = %q, want dashboard count", request.Operation)
	}
	if len(request.Fields) != 0 || len(request.Measures) != 0 {
		t.Fatalf("physical projection = fields %#v measures %#v, want count-only", request.Fields, request.Measures)
	}
	if got := request.AuthorizationFields; len(got) != 3 || got[0].Field != "orders.order_id" || got[1].Field != "orders.customer_email" || got[2].Field != "order_value" {
		t.Fatalf("authorization projection = %#v", got)
	}
}
