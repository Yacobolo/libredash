package app

import (
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/servingstate"
)

func TestActiveRefreshPipelineScopesMatchServerEnvironment(t *testing.T) {
	scopes := []servingstate.ActiveScope{
		{WorkspaceID: "sales", Environment: "dev"},
		{WorkspaceID: "sales", Environment: "prod"},
		{WorkspaceID: "support", Environment: "prod"},
	}
	got := activeRefreshPipelineScopes(scopes, "prod")
	want := []servingstate.ActiveScope{
		{WorkspaceID: "sales", Environment: "prod"},
		{WorkspaceID: "support", Environment: "prod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("activeRefreshPipelineScopes() = %#v, want %#v", got, want)
	}
}
