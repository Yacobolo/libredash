package sqlite

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/platform"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
)

func TestRepositoryReconcileAndClaimDueCoalescesCatchUp(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	repo := NewRepository(store.SQLDB())
	schedule, err := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	deployedAt := time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC)
	if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
		WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "sha256:a",
		Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", Name: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}},
		Now:       deployedAt,
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	next, ok, err := repo.NextRun(t.Context(), "sales", "prod", "sales-refresh")
	if err != nil || !ok || !next.Equal(time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextRun() = %s, %v, %v", next, ok, err)
	}

	due, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("due = %#v, want one catch-up occurrence", due)
	}
	if due[0].PipelineID != "sales-refresh" || due[0].SemanticModel != "sales" {
		t.Fatalf("occurrence = %#v", due[0])
	}
	if due[0].ArtifactDigest != "sha256:a" {
		t.Fatalf("artifact digest = %q, want sha256:a", due[0].ArtifactDigest)
	}
	if !due[0].ScheduledAt.Equal(time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("scheduled at = %s", due[0].ScheduledAt)
	}

	again, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second ClaimDue() error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second due = %#v, want none", again)
	}
}

func TestRepositoryClaimDueDoesNotAdvanceAnotherEnvironment(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(store.SQLDB())
	schedule, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	deployedAt := time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC)
	for _, environment := range []string{"dev", "prod"} {
		if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
			WorkspaceID: "sales", Environment: environment, ArtifactDigest: "sha256:" + environment, Now: deployedAt,
			Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	due, err := repo.ClaimDue(t.Context(), "dev", time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC))
	if err != nil || len(due) != 1 || due[0].Environment != "dev" {
		t.Fatalf("dev ClaimDue() = %#v, %v", due, err)
	}
	prodNext, ok, err := repo.NextRun(t.Context(), "sales", "prod", "sales-refresh")
	if err != nil || !ok || !prodNext.Equal(time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("prod next run = %s, found=%v, err=%v", prodNext, ok, err)
	}
}

func TestRepositoryCoalescesSimultaneouslyDueScheduleEntries(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	morning, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	later, _ := refreshpipeline.ParseSchedule("0 7 * * *", "UTC")
	repo := NewRepository(store.SQLDB())
	if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
		WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "sha256:a", Now: time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC),
		Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{morning, later}}},
	}); err != nil {
		t.Fatal(err)
	}
	due, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || !due[0].ScheduledAt.Equal(time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("due = %#v, want one coalesced latest occurrence", due)
	}
}

func TestRepositoryReleaseOccurrenceMakesQueueFailureRetryable(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	schedule, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	repo := NewRepository(store.SQLDB())
	if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
		WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "sha256:a", Now: time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC),
		Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	first, err := repo.ClaimDue(t.Context(), "prod", now)
	if err != nil || len(first) != 1 {
		t.Fatalf("first ClaimDue() = %#v, %v", first, err)
	}
	if err := repo.ReleaseOccurrence(t.Context(), first[0]); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ClaimDue(t.Context(), "prod", now)
	if err != nil || len(second) != 1 {
		t.Fatalf("second ClaimDue() = %#v, %v, want retry", second, err)
	}
	if second[0].ArtifactDigest != "sha256:a" || !second[0].ScheduledAt.Equal(first[0].ScheduledAt) {
		t.Fatalf("retry occurrence = %#v, want %#v", second[0], first[0])
	}
}

func TestRepositoryRecoversAbandonedOccurrenceClaim(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	schedule, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	repo := NewRepository(store.SQLDB())
	if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
		WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "sha256:a", Now: time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC),
		Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}},
	}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC))
	if err != nil || len(first) != 1 {
		t.Fatalf("first ClaimDue() = %#v, %v", first, err)
	}
	if _, err := store.SQLDB().ExecContext(t.Context(), `
UPDATE refresh_pipeline_occurrences SET claimed_at = '2026-07-18T06:00:00Z'
WHERE workspace_id = 'sales' AND environment = 'prod' AND pipeline_id = 'sales-refresh'`); err != nil {
		t.Fatal(err)
	}
	recovered, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 18, 7, 10, 0, 0, time.UTC))
	if err != nil || len(recovered) != 1 {
		t.Fatalf("recovered ClaimDue() = %#v, %v", recovered, err)
	}
	if !recovered[0].ScheduledAt.Equal(first[0].ScheduledAt) || recovered[0].ArtifactDigest != first[0].ArtifactDigest {
		t.Fatalf("recovered occurrence = %#v, want %#v", recovered[0], first[0])
	}
}

func TestRepositoryClaimDueDeduplicatesConcurrentDispatchers(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	schedule, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	repo := NewRepository(store.SQLDB())
	if err := repo.Reconcile(t.Context(), refreshpipeline.ReconcileInput{
		WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "sha256:a", Now: time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC),
		Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}},
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan []refreshpipeline.Occurrence, 2)
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			due, err := repo.ClaimDue(t.Context(), "prod", time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC))
			results <- due
			errors <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)
	claimed := 0
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for due := range results {
		claimed += len(due)
	}
	if claimed != 1 {
		t.Fatalf("claimed occurrences = %d, want exactly one", claimed)
	}
}

func TestRepositoryReconcileRemovesSupersededSchedules(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(store.SQLDB())
	schedule, _ := refreshpipeline.ParseSchedule("0 6 * * *", "UTC")
	now := time.Date(2026, 7, 18, 5, 0, 0, 0, time.UTC)
	input := refreshpipeline.ReconcileInput{WorkspaceID: "sales", Environment: "prod", ArtifactDigest: "old", Pipelines: []refreshpipeline.Definition{{ID: "sales-refresh", SemanticModel: "sales", Schedules: []refreshpipeline.Schedule{schedule}}}, Now: now}
	if err := repo.Reconcile(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	input.ArtifactDigest = "new"
	input.Pipelines = nil
	if err := repo.Reconcile(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	due, err := repo.ClaimDue(t.Context(), "prod", now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("due = %#v, want removed schedule to be ineligible", due)
	}
}

func TestRepositorySemanticModelDataVersion(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO serving_states (id, workspace_id, status, digest, manifest_json, environment) VALUES ('dep_1', 'sales', 'active', 'a', '{}', 'prod')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQLDB().ExecContext(t.Context(), `
INSERT INTO refresh_jobs (id, workspace_id, serving_state_id, model_id, kind, status) VALUES ('job_1', 'sales', 'dep_1', 'sales', 'refresh_pipeline', 'succeeded');
INSERT INTO refresh_job_runs (id, job_id, environment, target_type, target_id, trigger_type, status) VALUES ('run_1', 'job_1', 'prod', 'refresh_pipeline', 'sales.sales-refresh', 'manual', 'succeeded');
`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(store.SQLDB())
	want := refreshpipeline.DataVersion{
		WorkspaceID: "sales", Environment: "prod", SemanticModel: "sales", SnapshotID: 42,
		ServingStateID: "dep_1", RefreshedAt: time.Date(2026, 7, 18, 6, 1, 0, 0, time.UTC),
		Source: refreshpipeline.DataVersionSourceRefresh, PipelineID: "sales-refresh", RunID: "run_1",
	}
	if err := repo.SaveDataVersion(t.Context(), want); err != nil {
		t.Fatalf("SaveDataVersion() error = %v", err)
	}
	got, ok, err := repo.DataVersion(t.Context(), "sales", "prod", "sales")
	if err != nil || !ok {
		t.Fatalf("DataVersion() = %#v, %v, %v", got, ok, err)
	}
	if got != want {
		t.Fatalf("DataVersion() = %#v, want %#v", got, want)
	}
}
