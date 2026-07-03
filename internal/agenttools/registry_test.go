package agenttools

import (
	"reflect"
	"testing"
)

func TestParseExtensionOutputMetadata(t *testing.T) {
	extension, ok := ParseExtension(map[string]any{
		"enabled":      true,
		"name":         "list_workspaces",
		"risk":         "read",
		"tags":         []any{"workspace"},
		"defaultLimit": float64(25),
		"output": map[string]any{
			"itemsPath":  "items",
			"fields":     []any{"id", "title", "description"},
			"cursorPath": "page.nextCursor",
			"count":      true,
		},
	})
	if !ok {
		t.Fatal("ParseExtension returned ok=false")
	}
	if !extension.Enabled || extension.Name != "list_workspaces" || extension.Risk != "read" || extension.DefaultLimit != 25 {
		t.Fatalf("extension scalar fields = %#v", extension)
	}
	if got, want := extension.Tags, []string{"workspace"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	if extension.Output.ItemsPath != "items" || extension.Output.CursorPath != "page.nextCursor" || !extension.Output.Count {
		t.Fatalf("output scalar fields = %#v", extension.Output)
	}
	if got, want := extension.Output.Fields, []string{"id", "title", "description"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output fields = %#v, want %#v", got, want)
	}
}

func TestParseExtensionRichOutputMetadata(t *testing.T) {
	extension, ok := ParseExtension(map[string]any{
		"enabled": true,
		"name":    "query_dashboard_page",
		"risk":    "read",
		"output": map[string]any{
			"rootFields": []any{"title", "availableRows"},
			"collections": []any{
				map[string]any{
					"path":   "blocks.a.rows",
					"as":     "rows",
					"fields": []any{"order_id", "status"},
					"count":  true,
				},
			},
			"maps": []any{
				map[string]any{
					"path":   "visuals",
					"as":     "visuals",
					"fields": []any{"id", "title", "type"},
					"collection": map[string]any{
						"path":  "data",
						"as":    "data",
						"count": true,
					},
				},
			},
		},
	})
	if !ok {
		t.Fatal("ParseExtension returned ok=false")
	}
	if got, want := extension.Output.RootFields, []string{"title", "availableRows"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root fields = %#v, want %#v", got, want)
	}
	if len(extension.Output.Collections) != 1 {
		t.Fatalf("collections = %#v, want one collection", extension.Output.Collections)
	}
	collection := extension.Output.Collections[0]
	if collection.Path != "blocks.a.rows" || collection.As != "rows" || !collection.Count {
		t.Fatalf("collection scalar fields = %#v", collection)
	}
	if got, want := collection.Fields, []string{"order_id", "status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("collection fields = %#v, want %#v", got, want)
	}
	if len(extension.Output.Maps) != 1 {
		t.Fatalf("maps = %#v, want one map", extension.Output.Maps)
	}
	outputMap := extension.Output.Maps[0]
	if outputMap.Path != "visuals" || outputMap.As != "visuals" || outputMap.Collection.Path != "data" || outputMap.Collection.As != "data" || !outputMap.Collection.Count {
		t.Fatalf("map metadata = %#v", outputMap)
	}
	if got, want := outputMap.Fields, []string{"id", "title", "type"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("map fields = %#v, want %#v", got, want)
	}
}
