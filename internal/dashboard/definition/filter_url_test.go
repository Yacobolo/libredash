package definition

import (
	"net/url"
	"testing"

	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
)

func TestTypedFilterURLRoundTripOmitsDefaultsAndIgnoresOtherPages(t *testing.T) {
	definition := Definition{
		ID: "sales",
		FilterDefinitions: map[string]dashboardfilter.Definition{
			"state": {
				ValueKind: dashboardfilter.ValueString,
				Predicates: []dashboardfilter.PredicatePolicy{{
					Kind: dashboardfilter.ExpressionSet, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorIn},
				}},
			},
		},
		FilterApplication: dashboardfilter.ApplicationPolicy{Mode: dashboardfilter.ApplicationImmediate},
		FilterBindings: map[string]dashboardfilter.Binding{
			"report_state": {
				Key: "fb_report", ID: "report_state", Filter: "state", Scope: dashboardfilter.ScopeReport,
				Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
				URL:     dashboardfilter.URLPolicy{Param: "state", Encoding: dashboardfilter.URLEncodingTypedV1},
			},
		},
	}
	expression := dashboardfilter.Expression{
		Kind: dashboardfilter.ExpressionSet, Operator: dashboardfilter.OperatorIn,
		Values: []dashboardfilter.Value{{Kind: dashboardfilter.ValueString, Value: "WA"}},
	}
	encoded, err := dashboardfilter.EncodeTypedV1(expression, dashboardfilter.ValueString)
	if err != nil {
		t.Fatal(err)
	}
	state, err := definition.FilterStateFromURL("overview", url.Values{"state": []string{encoded}})
	if err != nil {
		t.Fatal(err)
	}
	if got := state.AppliedControls["fb_report"].Expression; got.Kind != dashboardfilter.ExpressionSet {
		t.Fatalf("URL state = %#v", got)
	}
	params, err := definition.URLParamsFromFilterState("overview", state)
	if err != nil {
		t.Fatal(err)
	}
	if params.Get("state") != encoded {
		t.Fatalf("round-trip param = %q, want %q", params.Get("state"), encoded)
	}
	defaults := definition.DefaultFilterState()
	defaultParams, err := definition.URLParamsFromFilterState("overview", defaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultParams) != 0 {
		t.Fatalf("default params = %#v, want omitted", defaultParams)
	}
}
