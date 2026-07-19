package model

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedConnectionRejectsAuthoredPhysicalLocation(t *testing.T) {
	for _, connection := range []Connection{
		{Kind: "managed", Root: "/server/revision"},
		{Kind: "managed", Scope: "s3://private-bucket/revision"},
	} {
		if _, err := connection.Validate("olist"); err == nil || !strings.Contains(err.Error(), "physical location") {
			t.Fatalf("Validate() error = %v, want managed physical location rejection", err)
		}
	}
}

func TestConnectionRejectsRemovedLocalKind(t *testing.T) {
	_, err := (Connection{Kind: "local"}).Validate("files")
	if err == nil || !strings.Contains(err.Error(), `unsupported kind "local"`) {
		t.Fatalf("Validate() error = %v, want unsupported local kind", err)
	}
}

func TestObjectStorageCredentialModes(t *testing.T) {
	for _, connection := range []Connection{
		{Kind: "s3", Scope: "s3://public/", Credentials: ConnectionCredentials{Provider: "none"}},
		{Kind: "s3", Scope: "s3://private/", Credentials: ConnectionCredentials{Provider: "ambient", Region: "eu-west-1"}},
		{Kind: "azure_blob", Scope: "az://container/", Credentials: ConnectionCredentials{Provider: "ambient", AccountName: "analytics"}},
	} {
		if _, err := connection.Validate("lake"); err != nil {
			t.Fatalf("Validate(%#v): %v", connection.Credentials, err)
		}
	}
	for _, connection := range []Connection{
		{Kind: "r2", Credentials: ConnectionCredentials{Provider: "ambient"}},
		{Kind: "azure_blob", Credentials: ConnectionCredentials{Provider: "ambient"}},
		{Kind: "s3", Credentials: ConnectionCredentials{Provider: "ambient"}},
		{Kind: "azure_blob", Credentials: ConnectionCredentials{Provider: "ambient", AccountName: "analytics"}},
		{Kind: "azure_blob", Scope: "az://container/", Credentials: ConnectionCredentials{Provider: "ambient", AccountName: "analytics", Endpoint: "blob.example.com"}},
	} {
		if _, err := connection.Validate("lake"); err == nil {
			t.Fatalf("Validate(%#v) succeeded", connection)
		}
	}
}

func TestManagedSourceRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	connections := map[string]Connection{"olist": {Kind: "managed"}}
	for _, value := range []string{filepath.Join(string(filepath.Separator), "orders.csv"), "../orders.csv"} {
		source := Source{Connection: "olist", Path: value, Format: "csv"}
		if err := source.Validate("orders", connections); err == nil {
			t.Fatalf("Validate(%q) error = nil, want unsafe managed path rejection", value)
		}
	}
}

func TestValidateRejectsAuthoredSourceReads(t *testing.T) {
	model := &Model{
		Name:        "test",
		Connections: map[string]Connection{"files": {Kind: "managed"}},
		Sources: map[string]Source{
			"orders": {Connection: "files", Path: "orders.csv", Format: "csv"},
		},
		Tables: map[string]Table{
			"orders": {
				Sources:     []string{"orders"},
				SourceReads: map[string][]string{"orders": {"order_id"}},
				PrimaryKey:  "order_id",
				Dimensions:  map[string]MetricDimension{"order_id": {Label: "Order ID"}},
				Transform:   Transform{SQL: "SELECT order_id FROM source.orders"},
			},
		},
	}

	err := model.Validate()
	if err == nil || !strings.Contains(err.Error(), "source_reads is no longer supported") {
		t.Fatalf("Validate() error = %v, want source_reads rejection", err)
	}
}

func TestSemanticDefinitionsValidateTypedMeasuresDimensionsAndMetrics(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Model)
		wantErr string
	}{
		{
			name: "count input",
			mutate: func(model *Model) {
				model.Measures["rating_count"] = MetricMeasure{Fact: "ratings", Aggregation: "count", Input: MeasureInput{Field: "ratings.score"}, Empty: "zero"}
			},
			wantErr: "count must not define input",
		},
		{
			name: "missing non-count input",
			mutate: func(model *Model) {
				model.Measures["rating_count"] = MetricMeasure{Fact: "ratings", Aggregation: "sum", Empty: "zero"}
			},
			wantErr: "requires exactly one input",
		},
		{
			name: "input from another fact",
			mutate: func(model *Model) {
				model.Measures["rating_total"] = MetricMeasure{Fact: "ratings", Aggregation: "sum", Input: MeasureInput{Field: "tags.weight"}, Empty: "null"}
			},
			wantErr: "is not owned by fact",
		},
		{
			name: "metric-only input function",
			mutate: func(model *Model) {
				model.Measures["rating_total"] = MetricMeasure{Fact: "ratings", Aggregation: "sum", Input: MeasureInput{Expression: "safe_divide(${ratings.score}, 2)"}, Empty: "null"}
			},
			wantErr: "metric-only",
		},
		{
			name: "invalid time grain",
			mutate: func(model *Model) {
				model.Dimensions["activity_date"] = SemanticDimension{Type: "timestamp", Grains: []string{"hour"}, Bindings: map[string]DimensionBinding{"ratings": {Field: "ratings.rated_at"}}}
			},
			wantErr: "unsupported time grain",
		},
		{
			name: "incompatible binding type",
			mutate: func(model *Model) {
				model.Dimensions["score_label"] = SemanticDimension{Type: "string", Bindings: map[string]DimensionBinding{"ratings": {Field: "ratings.score"}}}
			},
			wantErr: "is incompatible",
		},
		{
			name: "metric cycle",
			mutate: func(model *Model) {
				model.Metrics = map[string]Metric{"a": {Expression: "${b}"}, "b": {Expression: "${a}"}}
			},
			wantErr: "dependency cycle",
		},
		{
			name: "ambiguous implicit binding",
			mutate: func(model *Model) {
				model.Relationships = append(model.Relationships,
					Relationship{ID: "ratings_movies_alt", From: "ratings.alt_movie_id", To: "movies.movie_id", Cardinality: "many_to_one"},
				)
				model.Dimensions["movie_title"] = SemanticDimension{Type: "string", Bindings: map[string]DimensionBinding{"ratings": {Field: "movies.title"}}}
			},
			wantErr: "ambiguous relationship path",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := semanticDefinitionTestModel()
			test.mutate(model)
			err := model.validateSemanticDefinitions()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateSemanticDefinitions() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestSemanticDimensionExplicitPathResolvesAmbiguousGraph(t *testing.T) {
	model := semanticDefinitionTestModel()
	model.Relationships = append(model.Relationships,
		Relationship{ID: "ratings_movies_alt", From: "ratings.alt_movie_id", To: "movies.movie_id", Cardinality: "many_to_one"},
	)
	model.Dimensions["movie_title"] = SemanticDimension{Type: "string", Bindings: map[string]DimensionBinding{
		"ratings": {Field: "movies.title", Path: []string{"ratings_movies"}},
	}}
	if err := model.validateSemanticDefinitions(); err != nil {
		t.Fatal(err)
	}
}

func semanticDefinitionTestModel() *Model {
	return &Model{
		Name: "activity",
		Tables: map[string]Table{
			"ratings": {Dimensions: map[string]MetricDimension{
				"movie_id": {Type: "string"}, "alt_movie_id": {Type: "string"}, "score": {Type: "number"}, "rated_at": {Type: "timestamp"},
			}},
			"tags":   {Dimensions: map[string]MetricDimension{"weight": {Type: "number"}}},
			"movies": {Dimensions: map[string]MetricDimension{"movie_id": {Type: "string"}, "title": {Type: "string"}}},
		},
		Relationships: []Relationship{{ID: "ratings_movies", From: "ratings.movie_id", To: "movies.movie_id", Cardinality: "many_to_one"}},
		Measures: map[string]MetricMeasure{
			"rating_count": {Fact: "ratings", Aggregation: "count", Empty: "zero"},
			"tag_count":    {Fact: "tags", Aggregation: "count", Empty: "zero"},
		},
		Dimensions: map[string]SemanticDimension{},
		Metrics:    map[string]Metric{},
	}
}
