package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestGeneratedArtifactsAreCurrent(t *testing.T) {
	outputs, err := generatedOutputs()
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range outputs {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale; run task config:generate", path)
		}
	}
}

func TestGeneratedEnvironmentSchemaCompiles(t *testing.T) {
	_ = compileEnvironmentSchema(t)
}

func TestGeneratedEnvironmentSchemaEnforcesProductionRelationships(t *testing.T) {
	schema := compileEnvironmentSchema(t)
	valid := map[string]any{
		"LIBREDASH_PRODUCTION":           "1",
		"LIBREDASH_LOCAL_AUTH":           "true",
		"LIBREDASH_CSRF_KEY":             "0123456789abcdef0123456789abcdef",
		"LIBREDASH_METRICS_BEARER_TOKEN": "0123456789abcdef0123456789abcdef",
		"LIBREDASH_ALLOWED_HOSTS":        "libredash.example.com",
		"LIBREDASH_COOKIE_SECURE":        "true",
	}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("valid production environment rejected: %v", err)
	}
	if err := schema.Validate(map[string]any{"LIBREDASH_PRODUCTION": "1"}); err == nil {
		t.Fatal("schema accepted production environment without authentication and secrets")
	}
}

func compileEnvironmentSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	outputs, err := generatedOutputs()
	if err != nil {
		t.Fatal(err)
	}
	path := repositoryRoot() + "/schemas/config/environment.schema.json"
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(outputs[path]))
	if err != nil {
		t.Fatalf("unmarshal generated environment schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	const location = "https://libredash.dev/schemas/config/environment.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatalf("compile generated environment schema: %v", err)
	}
	return schema
}
