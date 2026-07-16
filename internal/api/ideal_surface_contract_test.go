package api_test

import (
	"strings"
	"testing"
)

func TestIdealV1Surface(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")

	required := map[string][]string{
		"/api/v1/capabilities":                                                                       {"get"},
		"/api/v1/projects":                                                                           {"get"},
		"/api/v1/projects/{project}":                                                                 {"get"},
		"/api/v1/projects/{project}/workspaces":                                                      {"get"},
		"/api/v1/projects/{project}/connections":                                                     {"get"},
		"/api/v1/projects/{project}/connections/{connection}":                                        {"get"},
		"/api/v1/projects/{project}/connections/{connection}/active-revision":                        {"get"},
		"/api/v1/projects/{project}/releases":                                                        {"get", "post"},
		"/api/v1/projects/{project}/releases/{release}":                                              {"get"},
		"/api/v1/projects/{project}/releases/{release}/workspaces/{workspace}/artifact":              {"put"},
		"/api/v1/projects/{project}/releases/{release}/finalize":                                     {"post"},
		"/api/v1/projects/{project}/releases/{release}/events":                                       {"get"},
		"/api/v1/projects/{project}/deployments":                                                     {"get", "post"},
		"/api/v1/projects/{project}/deployments/{deployment}/events":                                 {"get"},
		"/api/v1/projects/{project}/deployments/{deployment}/cancel":                                 {"post"},
		"/api/v1/projects/{project}/deployments/{deployment}/rollback":                               {"post"},
		"/api/v1/workspaces/{workspace}":                                                             {"get"},
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}":                         {"get"},
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}":          {"get"},
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}":        {"get"},
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}/query":    {"post"},
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/values": {"post"},
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/relationships":                       {"get"},
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/sources":                             {"get"},
		"/api/v1/workspaces/{workspace}/refresh-runs/{run}/events":                                   {"get"},
		"/api/v1/workspaces/{workspace}/refresh-runs/{run}/cancel":                                   {"post"},
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs":                     {"get", "post"},
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/cancel":        {"post"},
		"/api/v1/workspaces/{workspace}/grants/{grant}":                                              {"get", "patch", "delete"},
		"/api/v1/workspaces/{workspace}/data-policies/{policy}":                                      {"get", "patch", "delete"},
	}
	for path, methods := range required {
		for _, method := range methods {
			_ = openAPIOperation(t, paths, path, method)
		}
	}

	removed := []string{
		"/api/v1/agent/conversations",
		"/api/v1/projects/{project}/workspaces/{workspace}/deployment-candidates",
		"/api/v1/projects/{project}/deployments/{deployment}/activate",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/components",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/tables/{table}/query",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}/data",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/options",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/query",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/turns",
	}
	for _, path := range removed {
		if _, ok := paths[path]; ok {
			t.Errorf("legacy path remains in OpenAPI: %s", path)
		}
	}
}

func TestIdealQueryAndEventRepresentations(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")

	for _, tc := range []struct {
		path   string
		method string
		media  string
	}{
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/query", "post", "application/vnd.apache.arrow.stream"},
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview", "post", "application/vnd.apache.arrow.stream"},
		{"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}/query", "post", "application/vnd.apache.arrow.stream"},
		{"/api/v1/projects/{project}/releases/{release}/events", "get", "text/event-stream"},
		{"/api/v1/projects/{project}/deployments/{deployment}/events", "get", "text/event-stream"},
		{"/api/v1/workspaces/{workspace}/refresh-runs/{run}/events", "get", "text/event-stream"},
		{"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events", "get", "text/event-stream"},
	} {
		op := openAPIOperation(t, paths, tc.path, tc.method)
		if !operationHasResponseMedia(op, "200", tc.media) {
			t.Errorf("%s %s does not declare %s", tc.method, tc.path, tc.media)
		}
	}

	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	columns := schemaProperty(t, openAPISchema(t, schemas, "SemanticQueryResponse"), "columns")
	if items, _ := columns["items"].(map[string]any); items == nil || items["$ref"] != "#/components/schemas/QueryColumn" {
		t.Fatalf("semantic query columns are not typed descriptors: %#v", columns)
	}
	rows := schemaProperty(t, openAPISchema(t, schemas, "SemanticQueryResponse"), "rows")
	if rows["type"] != "array" {
		t.Fatalf("semantic query rows are not positional arrays: %#v", rows)
	}
}

func TestDashboardVisualResponsesAreShapeDiscriminated(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	visual := openAPISchema(t, schemas, "DashboardVisualDataResponse")
	discriminator, _ := visual["discriminator"].(map[string]any)
	if discriminator["propertyName"] != "shape" {
		t.Fatalf("visual response discriminator = %#v", discriminator)
	}
	variants, _ := visual["oneOf"].([]any)
	if len(variants) != 12 {
		t.Fatalf("visual response variants = %d, want 12: %#v", len(variants), visual)
	}
	for _, name := range []string{
		"CategoryValueVisualDatum", "CategorySeriesValueVisualDatum", "CategoryMultiMeasureVisualDatum",
		"CategoryDeltaVisualDatum", "BinnedMeasureVisualDatum", "HierarchyVisualDatum",
		"SingleValueVisualDatum", "MatrixVisualDatum", "GraphVisualDatum", "GeoVisualDatum",
		"OHLCVisualDatum", "DistributionVisualDatum",
	} {
		schema := openAPISchema(t, schemas, name)
		if len(openAPIMap(t, schema, "properties")) == 0 {
			t.Errorf("%s has no explicit fields", name)
		}
	}
}

func TestCapabilitiesUseCanonicalEnums(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	assertEnum(t, openAPISchema(t, schemas, "AuthenticationMode"), "bearer")
	assertEnum(t, openAPISchema(t, schemas, "QueryFormat"), "application/json", "application/vnd.apache.arrow.stream")
	assertEnum(t, openAPISchema(t, schemas, "UploadProtocol"), "tus", "s3_multipart")
	assertEnum(t, openAPISchema(t, schemas, "VisualShape"),
		"category_value", "category_series_value", "category_multi_measure", "category_delta",
		"binned_measure", "hierarchy", "single_value", "matrix", "graph", "geo", "ohlc", "distribution")
}

func operationHasResponseMedia(operation map[string]any, status, media string) bool {
	responses, _ := operation["responses"].(map[string]any)
	response, _ := responses[status].(map[string]any)
	content, _ := response["content"].(map[string]any)
	_, ok := content[media]
	return ok
}

func TestIdealAPIUsesProblemDetailsAndMutationHeaders(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")
	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	problem := openAPISchema(t, schemas, "ProblemDetails")
	for _, name := range []string{"type", "title", "status", "detail", "instance", "code", "requestId", "errors"} {
		_ = schemaProperty(t, problem, name)
	}

	for _, tc := range []struct {
		path   string
		method string
	}{
		{"/api/v1/projects/{project}/releases", "post"},
		{"/api/v1/projects/{project}/releases/{release}/finalize", "post"},
		{"/api/v1/projects/{project}/deployments", "post"},
		{"/api/v1/workspaces/{workspace}/refresh-runs", "post"},
		{"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs", "post"},
	} {
		op := openAPIOperation(t, paths, tc.path, tc.method)
		if !operationHasParameter(op, "header", "Idempotency-Key") {
			t.Errorf("%s %s does not require Idempotency-Key", tc.method, tc.path)
		}
	}

	encoded := string(mustJSON(t, spec))
	if !strings.Contains(encoded, "application/problem+json") {
		t.Fatal("OpenAPI does not declare application/problem+json")
	}
}
