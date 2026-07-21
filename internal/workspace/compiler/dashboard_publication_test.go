package compiler

import (
	"strings"
	"testing"
)

func TestCompileProjectLoadsDashboardPublicationsAndDependencyClosure(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml": workspaceYAMLWithPublications("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{
			"https://leapview.dev",
			"http://localhost:4321",
		}),
		"workspaces/sales/publications/partner.yaml": dashboardPublicationYAML("sales", "partner", "executive-sales", "overview", nil),
	})

	compiled, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	publications := compiled.Workspaces["sales"].Definition.Publications
	if len(publications) != 2 {
		t.Fatalf("publications = %#v, want 2", publications)
	}
	publication := publications["website"]
	if publication.Dashboard != "executive-sales" || publication.DefaultPage != "overview" {
		t.Fatalf("publication = %#v", publication)
	}
	if got := strings.Join(publication.AllowedOrigins, ","); got != "http://localhost:4321,https://leapview.dev" {
		t.Fatalf("allowed origins = %q", got)
	}
	for _, want := range []string{
		"dashboard:sales.executive-sales",
		"semantic_model:sales.sales",
		"measure:sales.sales.order_count",
		"model_table:sales.orders",
		"source:olist.orders",
	} {
		if !containsString(publication.DependencyAssetIDs, want) {
			t.Fatalf("dependency closure = %v, missing %q", publication.DependencyAssetIDs, want)
		}
	}
	if publication.ConfigurationDigest == "" {
		t.Fatal("configuration digest is empty")
	}
}

func TestCompileProjectPublicationClosureIncludesDerivedMetricAndItsMeasures(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublications("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", nil),
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables:
    - orders
  measures:
    order_count:
      fact: orders
      aggregation: count
      empty: zero
    revenue:
      fact: orders
      aggregation: count
      empty: zero
  metrics:
    aov:
      expression: safe_divide(${revenue}, ${order_count})
`,
		"workspaces/sales/dashboards/executive-sales.yaml": `
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  workspace: sales
  name: executive-sales
  title: Executive sales
spec:
  semanticModel: sales
  visuals:
    aov:
      type: kpi
      query:
        measures:
          aov:
  pages:
    - id: overview
      title: Overview
      components:
        - id: aov
          kind: visual
          visual: aov
          placement:
            col: 1
            row: 1
            col_span: 3
            row_span: 2
`,
	})

	compiled, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public_metric"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	closure := compiled.Workspaces["sales"].Definition.Publications["website"].DependencyAssetIDs
	for _, want := range []string{
		"measure:sales.sales.aov",
		"measure:sales.sales.order_count",
		"measure:sales.sales.revenue",
	} {
		if !containsString(closure, want) {
			t.Fatalf("dependency closure = %v, missing %q", closure, want)
		}
	}
}

func TestCompileProjectPublicationClosureExcludesUnusedSemanticAssets(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublications("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", nil),
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables:
    - orders
  measures:
    order_count:
      fact: orders
      aggregation: count
      empty: zero
    unused_revenue:
      fact: orders
      aggregation: count
      empty: zero
`,
	})

	compiled, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public_least_privilege"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	closure := compiled.Workspaces["sales"].Definition.Publications["website"].DependencyAssetIDs
	if containsString(closure, "measure:sales.sales.unused_revenue") {
		t.Fatalf("dependency closure includes unused semantic asset: %v", closure)
	}
}

func TestCompileProjectPublicationClosureIncludesFilterDatasetAncestorsAndLogicalDimension(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublications("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", nil),
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: leapview.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  tables:
    - orders
  dimensions:
    order_key:
      type: string
      bindings:
        orders: {field: orders.order_id}
  measures:
    order_count:
      fact: orders
      aggregation: count
      empty: zero
`,
		"workspaces/sales/dashboards/executive-sales.yaml": `
apiVersion: leapview.dev/v1
kind: Dashboard
metadata:
  workspace: sales
  name: executive-sales
  title: Executive sales
spec:
  semanticModel: sales
  filters:
    status:
      type: multi_select
      label: Status
      field: order_key
      operator: in
      values:
        source: distinct
  visuals:
    total:
      type: kpi
      query:
        measures:
          order_count:
  pages:
    - id: overview
      title: Overview
      components:
        - id: total
          kind: visual
          visual: total
          placement: {col: 1, row: 1, col_span: 3, row_span: 2}
`,
	})

	compiled, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public_filter"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	closure := compiled.Workspaces["sales"].Definition.Publications["website"].DependencyAssetIDs
	for _, want := range []string{
		"field:sales.sales.order_key",
		"field:sales.sales.orders.order_id",
		"semantic_table:sales.sales.orders",
		"model_table:sales.orders",
		"source:olist.orders",
	} {
		if !containsString(closure, want) {
			t.Fatalf("dependency closure = %v, missing %q", closure, want)
		}
	}
}

func TestCompileProjectRejectsInvalidDashboardPublication(t *testing.T) {
	tests := []struct {
		name        string
		publication string
		want        string
		field       string
		resource    string
	}{
		{name: "unknown dashboard", publication: dashboardPublicationYAML("sales", "website", "missing", "overview", nil), want: `unknown Dashboard "missing"`, field: "spec.dashboard"},
		{name: "unknown page", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "missing", nil), want: `unknown page "missing"`, field: "spec.defaultPage"},
		{name: "workspace mismatch", publication: dashboardPublicationYAML("other", "website", "executive-sales", "overview", nil), want: `workspace = "other", want "sales"`, field: "metadata.workspace", resource: "dashboard_publication:other.website"},
		{name: "http internet", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"http://example.com"}), want: "must use https", field: "spec.embedding.allowedOrigins[0]"},
		{name: "wildcard", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://*.example.com"}), want: "wildcards", field: "spec.embedding.allowedOrigins[0]"},
		{name: "credentials", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://user@example.com"}), want: "credentials", field: "spec.embedding.allowedOrigins[0]"},
		{name: "path", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://example.com/path"}), want: "origin only", field: "spec.embedding.allowedOrigins[0]"},
		{name: "query", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://example.com?x=1"}), want: "origin only", field: "spec.embedding.allowedOrigins[0]"},
		{name: "fragment", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://example.com#x"}), want: "origin only", field: "spec.embedding.allowedOrigins[0]"},
		{name: "duplicate origin", publication: dashboardPublicationYAML("sales", "website", "executive-sales", "overview", []string{"https://example.com", "https://example.com"}), want: "duplicate origin", field: "spec.embedding.allowedOrigins[1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := minimalProjectFiles(map[string]string{
				"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublications("sales"),
				"workspaces/sales/publications/website.yaml": tt.publication,
			})
			_, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public"})
			assertCompileErrorContains(t, err, tt.want)
			resource := tt.resource
			if resource == "" {
				resource = "dashboard_publication:sales.website"
			}
			assertDiagnostic(t, err, resource, tt.field)
		})
	}
}

func TestCompileProjectSupportsDashboardPublicationDataPolicySubject(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublicationsAndAccess("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", nil),
		"workspaces/sales/access/public-filter.yaml": `
apiVersion: leapview.dev/v1
kind: DataPolicy
metadata:
  workspace: sales
  name: public-filter
spec:
  object:
    type: semantic_model
    id: sales
  subject:
    kind: dashboard_publication
    publication: website
  policyType: row_filter
  expression:
    eq:
      field: orders.status
      value: delivered
`,
	})

	compiled, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public"})
	if err != nil {
		t.Fatalf("CompileProject() error = %v", err)
	}
	subject := compiled.Workspaces["sales"].Definition.Access.DataPolicies["public-filter"].Subject
	if subject.Kind != "dashboard_publication" || subject.Publication != "website" {
		t.Fatalf("subject = %#v", subject)
	}
}

func TestCompileProjectRejectsDashboardPublicationGrantSubject(t *testing.T) {
	files := minimalProjectFiles(map[string]string{
		"workspaces/sales/workspace.yaml":            workspaceYAMLWithPublicationsAndAccess("sales"),
		"workspaces/sales/publications/website.yaml": dashboardPublicationYAML("sales", "website", "executive-sales", "overview", nil),
		"workspaces/sales/access/public-grant.yaml": `
apiVersion: leapview.dev/v1
kind: Grant
metadata:
  workspace: sales
  name: public-query
spec:
  object:
    type: semantic_model
    id: sales
  subject:
    kind: dashboard_publication
    publication: website
  privilege: QUERY_DATA
`,
	})

	_, err := CompileProject(writeProjectFixture(t, files), Options{ServingStateID: "dep_public_grant"})
	assertCompileErrorContains(t, err, "dashboard_publication")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func workspaceYAMLWithPublications(name string) string {
	return strings.Replace(workspaceYAML(name), "  access:\n", "  publications:\n    include:\n      - publications/*.yaml\n  access:\n", 1)
}

func workspaceYAMLWithPublicationsAndAccess(name string) string {
	return strings.Replace(workspaceYAMLWithPublications(name), "include: []", "include:\n      - access/*.yaml", 1)
}

func dashboardPublicationYAML(workspace, name, dashboard, defaultPage string, origins []string) string {
	content := `
apiVersion: leapview.dev/v1
kind: DashboardPublication
metadata:
  workspace: ` + workspace + `
  name: ` + name + `
spec:
  dashboard: ` + dashboard + `
  defaultPage: ` + defaultPage + `
  embedding:
    allowedOrigins:`
	if len(origins) == 0 {
		return content + " []\n"
	}
	content += "\n"
	for _, origin := range origins {
		content += "      - " + origin + "\n"
	}
	return content
}
