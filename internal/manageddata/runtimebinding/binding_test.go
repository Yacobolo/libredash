package runtimebinding

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestBindRootsUsesTrustedRuntimeResolution(t *testing.T) {
	definition := &workspace.Definition{Models: map[string]*semanticmodel.Model{
		"sales": {Connections: map[string]semanticmodel.Connection{
			"olist": {Kind: "managed"},
			"cloud": {Kind: "s3", Scope: "s3://warehouse/"},
		}},
	}}
	resolution := runtimehost.ManagedDataResolution{
		RevisionID: "sha256:" + strings.Repeat("a", 64),
		Roots:      map[string]string{"olist": "/managed/olist/revision"},
	}
	if err := BindRoots(definition, resolution); err != nil {
		t.Fatal(err)
	}
	if got := definition.Models["sales"].Connections["olist"].Root; got != "/managed/olist/revision" {
		t.Fatalf("olist root = %q", got)
	}
	if got := definition.Models["sales"].Connections["cloud"].Scope; got != "s3://warehouse/" {
		t.Fatalf("cloud scope = %q", got)
	}
}

func TestBindRootsRequiresEveryManagedConnection(t *testing.T) {
	definition := &workspace.Definition{Models: map[string]*semanticmodel.Model{
		"sales": {Connections: map[string]semanticmodel.Connection{"olist": {Kind: "managed"}}},
	}}
	err := BindRoots(definition, runtimehost.ManagedDataResolution{})
	if err == nil || !strings.Contains(err.Error(), "olist") {
		t.Fatalf("bind error = %v, want missing olist revision", err)
	}
}
