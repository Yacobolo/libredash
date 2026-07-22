package api_test

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apigen "github.com/Yacobolo/leapview/internal/api/gen"
)

func TestAPIPackageStaysTransportContractOnly(t *testing.T) {
	forbidden := map[string]bool{
		"github.com/Yacobolo/leapview/internal/app":     true,
		"github.com/Yacobolo/leapview/internal/ui":      true,
		"github.com/go-chi/chi/v5":                       true,
		"github.com/starfederation/datastar-go/datastar": true,
		"maragu.dev/gomponents":                          true,
		"maragu.dev/gomponents-datastar":                 true,
		"net/http":                                       true,
	}
	assertPackageDoesNotImport(t, ".", forbidden)
}

func TestAgentDoesNotDependOnHeadlessAPIContract(t *testing.T) {
	assertPackageDoesNotImport(t, filepath.Join("..", "agent"), map[string]bool{
		"github.com/Yacobolo/leapview/internal/api": true,
	})
}

func TestGeneratedAssetResponseRequiresSnapshotAndPayload(t *testing.T) {
	typ := reflect.TypeOf(apigen.AssetResponse{})
	for _, name := range []string{"SnapshotId", "Payload"} {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("AssetResponse.%s missing", name)
		}
		if field.Type.Kind() == reflect.Pointer {
			t.Fatalf("AssetResponse.%s is optional pointer type %s", name, field.Type)
		}
		if strings.Contains(string(field.Tag), "omitempty") {
			t.Fatalf("AssetResponse.%s JSON tag is optional: %s", name, field.Tag)
		}
	}
}

func TestGeneratedAssetListUsesSummaryWithoutPayload(t *testing.T) {
	listTyp := reflect.TypeOf(apigen.AssetListResponse{})
	items, ok := listTyp.FieldByName("Items")
	if !ok {
		t.Fatal("AssetListResponse.Items missing")
	}
	if items.Type.Kind() != reflect.Slice || items.Type.Elem() != reflect.TypeOf(apigen.AssetSummaryResponse{}) {
		t.Fatalf("AssetListResponse.Items type = %s, want []AssetSummaryResponse", items.Type)
	}
	if _, ok := reflect.TypeOf(apigen.AssetSummaryResponse{}).FieldByName("Payload"); ok {
		t.Fatal("AssetSummaryResponse unexpectedly includes Payload")
	}
}

func TestGeneratedAssetPayloadOpenAPIAllowsArbitraryJSON(t *testing.T) {
	spec, err := apigen.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	assetResponse, _ := schemas["AssetResponse"].(map[string]any)
	properties, _ := assetResponse["properties"].(map[string]any)
	payload, _ := properties["payload"].(map[string]any)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload schema: %v", err)
	}
	if strings.Contains(string(raw), `"additionalProperties":{"type":"string"}`) {
		t.Fatalf("payload schema is string-only, want arbitrary JSON: %s", raw)
	}
}

func assertPackageDoesNotImport(t *testing.T, dir string, forbidden map[string]bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("stat %s: %v", file, err)
		}
		if info.IsDir() {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", file, err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if forbidden[path] {
				t.Fatalf("%s imports forbidden package %s", file, path)
			}
		}
	}
}
