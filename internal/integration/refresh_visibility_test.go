package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	materializesqlite "github.com/Yacobolo/leapview/internal/analytics/materialize/sqlite"
)

func TestRefreshVisibilityStreamsAndPersistsSemanticModelRuns(t *testing.T) {
	h := newDuckLakeHarness(t)
	ctx := context.Background()
	workspaceID := "sales"
	semanticAssetID := integrationAssetID(t, h.store, workspaceID, "semantic_model", "sales.sales")
	ordersAssetID := integrationAssetID(t, h.store, workspaceID, "model_table", "sales.orders")
	pipelineAssetID := integrationAssetID(t, h.store, workspaceID, "refresh_pipeline", "sales.sales-refresh")

	details := h.getAuthenticatedHydrated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/details")
	for _, want := range []string{"Refresh status", "/updates?", "route=workspace_asset", "section=details"} {
		if !strings.Contains(details, want) {
			t.Fatalf("semantic model details page did not contain %q", want)
		}
	}
	if strings.Contains(details, "Refresh materializations") || strings.Contains(details, "Run now") {
		t.Fatalf("semantic model exposed a refresh action:\n%s", details)
	}
	pipelineDetails := h.getAuthenticatedHydrated(t, "/workspaces/"+workspaceID+"/assets/"+pipelineAssetID+"/details")
	for _, want := range []string{"Run now", "Semantic model", "Schedule", "0 6 * * *", "Europe/Copenhagen", "Next run", "Current data version", "publish"} {
		if !strings.Contains(pipelineDetails, want) {
			t.Fatalf("refresh pipeline details page did not contain %q", want)
		}
	}
	pipelineRefreshes := h.getAuthenticatedHydrated(t, "/workspaces/"+workspaceID+"/assets/"+pipelineAssetID+"/refreshes")
	for _, want := range []string{"Triggered by", "Trigger", "Run ID"} {
		if !strings.Contains(pipelineRefreshes, want) {
			t.Fatalf("refresh pipeline history page did not contain %q", want)
		}
	}

	pipelineStream := h.openAssetUpdatesStream(t, workspaceID, pipelineAssetID, "refreshes")
	_ = pipelineStream.nextPatch(t)

	if got := h.postAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+pipelineAssetID+"/refresh"); got != http.StatusNoContent {
		t.Fatalf("refresh pipeline run status = %d, want %d", got, http.StatusNoContent)
	}
	requireAssetRefreshStatus(t, pipelineStream, materialize.RunStatusRunning)
	requireAssetRefreshStatus(t, pipelineStream, materialize.RunStatusSucceeded)

	repo := materializesqlite.NewSQLRunRepository(h.store.SQLDB())
	pipelineRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetRefreshPipeline, "sales.sales-refresh", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list refresh pipeline runs: %v", err)
	}
	if len(pipelineRuns) != 1 {
		t.Fatalf("refresh pipeline runs = %#v, want one root run", pipelineRuns)
	}
	root := pipelineRuns[0]
	if root.Status != materialize.RunStatusSucceeded || root.TriggerType != materialize.TriggerManual || root.PrincipalID != "dev" {
		t.Fatalf("refresh pipeline run = %#v, want succeeded manual run attributed to dev", root)
	}

	tableRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetModelTable, "sales.orders", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list orders table runs: %v", err)
	}
	if len(tableRuns) != 1 {
		t.Fatalf("orders table runs = %#v, want one child run", tableRuns)
	}
	child := tableRuns[0]
	if child.Status != materialize.RunStatusSucceeded || child.TriggerType != materialize.TriggerDependency || child.ParentRunID != root.ID || child.PrincipalID != "dev" {
		t.Fatalf("orders table run = %#v, want internal dependency run attributed to dev", child)
	}

	pipelineRefreshes = h.getAuthenticatedHydrated(t, "/workspaces/"+workspaceID+"/assets/"+pipelineAssetID+"/refreshes")
	for _, want := range []string{"Local Developer", "Manual", shortRunID(root.ID)} {
		if !strings.Contains(pipelineRefreshes, want) {
			t.Fatalf("refresh pipeline history page did not contain %q after refresh", want)
		}
	}
	ordersDetails := h.getAuthenticatedHydrated(t, "/workspaces/"+workspaceID+"/assets/"+ordersAssetID+"/details")
	if strings.Contains(ordersDetails, "Run now") || strings.Contains(ordersDetails, "Refresh materializations") {
		t.Fatalf("model table exposed a refresh action:\n%s", ordersDetails)
	}

	request, err := http.NewRequest(http.MethodPost, h.serverURL(t)+"/api/v1/workspaces/sales/refresh-runs", strings.NewReader(`{"pipelineId":"sales-refresh"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer dev")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "manual-pipeline-api")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("manual refresh API status = %d, body=%s", response.StatusCode, body)
	}
	var publicRun map[string]any
	if err := json.Unmarshal(body, &publicRun); err != nil {
		t.Fatalf("decode manual refresh API response: %v; body=%s", err, body)
	}
	if publicRun["pipelineId"] != "sales-refresh" || publicRun["semanticModel"] != "sales" || publicRun["trigger"] != "manual" {
		t.Fatalf("manual refresh API response = %#v", publicRun)
	}
	createdAt, _ := publicRun["createdAt"].(string)
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		t.Fatalf("manual refresh API createdAt = %q, want RFC3339: %v", createdAt, err)
	}
	for _, internalField := range []string{"modelId", "servingStateId", "targetType", "targetId", "parentRunId"} {
		if _, exists := publicRun[internalField]; exists {
			t.Fatalf("manual refresh API exposed internal field %q: %#v", internalField, publicRun)
		}
	}
}

func requireAssetRefreshStatus(t *testing.T, stream *streamClient, status string) map[string]any {
	t.Helper()
	return requireStreamPatch(t, stream, func(patch map[string]any) bool {
		refresh := mapAt(mapAt(patch, "page"), "refresh")
		return stringValue(refresh["status"]) == status
	})
}

func requireStreamPatch(t *testing.T, stream *streamClient, match func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case patch, ok := <-stream.patches:
			if !ok {
				t.Fatal("asset updates stream closed before matching patch")
			}
			if match(patch) {
				return patch
			}
		case err := <-stream.errs:
			if err != nil {
				t.Fatalf("read asset updates stream: %v", err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for matching asset updates patch")
		}
	}
}

func shortRunID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
