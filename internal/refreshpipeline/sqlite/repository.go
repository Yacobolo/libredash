package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

const occurrenceClaimTimeout = 5 * time.Minute

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db, q: platformdb.New(db)} }

type persistedSchedule struct {
	artifactDigest string
	nextRunAt      time.Time
}

func (repository *Repository) Reconcile(ctx context.Context, input refreshpipeline.ReconcileInput) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("refresh pipeline database is required")
	}
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Environment = strings.TrimSpace(input.Environment)
	input.ArtifactDigest = strings.TrimSpace(input.ArtifactDigest)
	if input.WorkspaceID == "" || input.Environment == "" || input.ArtifactDigest == "" {
		return fmt.Errorf("workspace, environment, and artifact digest are required")
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := repository.q.WithTx(tx)
	existing, err := loadPersistedSchedules(ctx, queries, input.WorkspaceID, input.Environment)
	if err != nil {
		return err
	}
	if err := queries.DeleteRefreshPipelineSchedules(ctx, platformdb.DeleteRefreshPipelineSchedulesParams{WorkspaceID: input.WorkspaceID, Environment: input.Environment}); err != nil {
		return err
	}
	for _, pipeline := range input.Pipelines {
		for _, schedule := range pipeline.Schedules {
			key := scheduleKey(pipeline.ID, schedule.Expression, schedule.Timezone)
			next := schedule.Next(input.Now)
			if prior, ok := existing[key]; ok && prior.artifactDigest == input.ArtifactDigest {
				next = prior.nextRunAt
			}
			if next.IsZero() {
				return fmt.Errorf("refresh pipeline %q schedule %q has no next occurrence", pipeline.ID, schedule.Expression)
			}
			if err := queries.CreateRefreshPipelineSchedule(ctx, platformdb.CreateRefreshPipelineScheduleParams{
				WorkspaceID: input.WorkspaceID, Environment: input.Environment, PipelineID: pipeline.ID,
				SemanticModelID: pipeline.SemanticModel, ArtifactDigest: input.ArtifactDigest,
				Cron: schedule.Expression, Timezone: schedule.Timezone, NextRunAt: formatTime(next),
			}); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func loadPersistedSchedules(ctx context.Context, queries *platformdb.Queries, workspaceID, environment string) (map[string]persistedSchedule, error) {
	rows, err := queries.ListRefreshPipelineSchedules(ctx, platformdb.ListRefreshPipelineSchedulesParams{WorkspaceID: workspaceID, Environment: environment})
	if err != nil {
		return nil, err
	}
	out := map[string]persistedSchedule{}
	for _, row := range rows {
		next, err := parseTime(row.NextRunAt)
		if err != nil {
			return nil, err
		}
		out[scheduleKey(row.PipelineID, row.Cron, row.Timezone)] = persistedSchedule{artifactDigest: row.ArtifactDigest, nextRunAt: next}
	}
	return out, nil
}

type dueSchedule struct {
	workspaceID    string
	environment    string
	pipelineID     string
	semanticModel  string
	expression     string
	timezone       string
	artifactDigest string
	nextRunAt      time.Time
}

func (repository *Repository) ClaimDue(ctx context.Context, environment string, now time.Time) ([]refreshpipeline.Occurrence, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("refresh pipeline database is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return nil, fmt.Errorf("refresh pipeline environment is required")
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	queries := repository.q.WithTx(tx)
	claimedBefore := formatTime(now.Add(-occurrenceClaimTimeout))
	if err := queries.RequeueAbandonedRefreshPipelineSchedules(ctx, platformdb.RequeueAbandonedRefreshPipelineSchedulesParams{ClaimedBefore: claimedBefore, Environment: environment}); err != nil {
		return nil, err
	}
	if err := queries.DeleteAbandonedRefreshPipelineOccurrences(ctx, platformdb.DeleteAbandonedRefreshPipelineOccurrencesParams{ClaimedBefore: claimedBefore, Environment: environment}); err != nil {
		return nil, err
	}
	rows, err := queries.ListDueRefreshPipelineSchedules(ctx, platformdb.ListDueRefreshPipelineSchedulesParams{Environment: environment, NextRunAt: formatTime(now)})
	if err != nil {
		return nil, err
	}
	due := make([]dueSchedule, 0, len(rows))
	for _, row := range rows {
		item := dueSchedule{
			workspaceID: row.WorkspaceID, environment: row.Environment, pipelineID: row.PipelineID,
			semanticModel: row.SemanticModelID, expression: row.Cron, timezone: row.Timezone, artifactDigest: row.ArtifactDigest,
		}
		item.nextRunAt, err = parseTime(row.NextRunAt)
		if err != nil {
			return nil, err
		}
		due = append(due, item)
	}
	type pipelineDue struct {
		occurrence refreshpipeline.Occurrence
	}
	grouped := map[string]pipelineDue{}
	for _, item := range due {
		schedule, err := refreshpipeline.ParseSchedule(item.expression, item.timezone)
		if err != nil {
			return nil, err
		}
		scheduledAt := item.nextRunAt
		next := schedule.Next(scheduledAt)
		for !next.IsZero() && !next.After(now) {
			scheduledAt = next
			next = schedule.Next(next)
		}
		if next.IsZero() {
			return nil, fmt.Errorf("refresh pipeline %q schedule %q has no next occurrence", item.pipelineID, item.expression)
		}
		if err := queries.AdvanceRefreshPipelineSchedule(ctx, platformdb.AdvanceRefreshPipelineScheduleParams{
			NextRunAt: formatTime(next), WorkspaceID: item.workspaceID, Environment: item.environment,
			PipelineID: item.pipelineID, Cron: item.expression, Timezone: item.timezone,
		}); err != nil {
			return nil, err
		}
		key := item.workspaceID + "\x00" + item.environment + "\x00" + item.pipelineID
		current := grouped[key]
		if current.occurrence.ScheduledAt.IsZero() || scheduledAt.After(current.occurrence.ScheduledAt) {
			current.occurrence = refreshpipeline.Occurrence{
				WorkspaceID: item.workspaceID, Environment: item.environment, PipelineID: item.pipelineID,
				SemanticModel: item.semanticModel, ArtifactDigest: item.artifactDigest, ScheduledAt: scheduledAt,
			}
			grouped[key] = current
		}
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]refreshpipeline.Occurrence, 0, len(keys))
	for _, key := range keys {
		occurrence := grouped[key].occurrence
		result, err := queries.ClaimRefreshPipelineOccurrence(ctx, platformdb.ClaimRefreshPipelineOccurrenceParams{
			WorkspaceID: occurrence.WorkspaceID, Environment: occurrence.Environment, PipelineID: occurrence.PipelineID,
			ArtifactDigest: occurrence.ArtifactDigest, ScheduledAt: formatTime(occurrence.ScheduledAt), ClaimedAt: formatTime(now),
		})
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 1 {
			out = append(out, occurrence)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (repository *Repository) AttachRun(ctx context.Context, occurrence refreshpipeline.Occurrence, runID string) error {
	result, err := repository.q.AttachRefreshPipelineRun(ctx, platformdb.AttachRefreshPipelineRunParams{
		RunID: sql.NullString{String: strings.TrimSpace(runID), Valid: true}, WorkspaceID: occurrence.WorkspaceID,
		Environment: occurrence.Environment, PipelineID: occurrence.PipelineID, ScheduledAt: formatTime(occurrence.ScheduledAt),
	})
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("refresh pipeline occurrence no longer exists")
	}
	return nil
}

func (repository *Repository) ReleaseOccurrence(ctx context.Context, occurrence refreshpipeline.Occurrence) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := repository.q.WithTx(tx)
	result, err := queries.DeleteUnattachedRefreshPipelineOccurrence(ctx, platformdb.DeleteUnattachedRefreshPipelineOccurrenceParams{
		WorkspaceID: occurrence.WorkspaceID, Environment: occurrence.Environment,
		PipelineID: occurrence.PipelineID, ScheduledAt: formatTime(occurrence.ScheduledAt),
	})
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		if err := queries.RetryRefreshPipelineSchedules(ctx, platformdb.RetryRefreshPipelineSchedulesParams{
			RetryAt: formatTime(occurrence.ScheduledAt), WorkspaceID: occurrence.WorkspaceID, Environment: occurrence.Environment,
			PipelineID: occurrence.PipelineID, ArtifactDigest: occurrence.ArtifactDigest,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (repository *Repository) NextRun(ctx context.Context, workspaceID, environment, pipelineID string) (time.Time, bool, error) {
	value, err := repository.q.GetRefreshPipelineNextRun(ctx, platformdb.GetRefreshPipelineNextRunParams{
		WorkspaceID: workspaceID, Environment: environment, PipelineID: pipelineID,
	})
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	next, err := parseTime(value)
	if err != nil {
		return time.Time{}, false, err
	}
	return next, true, nil
}

func (repository *Repository) SaveDataVersion(ctx context.Context, version refreshpipeline.DataVersion) error {
	if version.WorkspaceID == "" || version.Environment == "" || version.SemanticModel == "" || version.SnapshotID <= 0 || version.ServingStateID == "" || version.RefreshedAt.IsZero() {
		return fmt.Errorf("complete semantic-model data version is required")
	}
	if version.Source != refreshpipeline.DataVersionSourcePublish && version.Source != refreshpipeline.DataVersionSourceRefresh {
		return fmt.Errorf("semantic-model data version source must be publish or refresh")
	}
	return repository.q.UpsertSemanticModelDataVersion(ctx, platformdb.UpsertSemanticModelDataVersionParams{
		WorkspaceID: version.WorkspaceID, Environment: version.Environment, SemanticModelID: version.SemanticModel,
		SnapshotID: version.SnapshotID, ServingStateID: version.ServingStateID, RefreshedAt: formatTime(version.RefreshedAt),
		Source: version.Source, PipelineID: version.PipelineID, RunID: version.RunID,
	})
}

func (repository *Repository) DataVersion(ctx context.Context, workspaceID, environment, semanticModel string) (refreshpipeline.DataVersion, bool, error) {
	row, err := repository.q.GetSemanticModelDataVersion(ctx, platformdb.GetSemanticModelDataVersionParams{
		WorkspaceID: workspaceID, Environment: environment, SemanticModelID: semanticModel,
	})
	if err == sql.ErrNoRows {
		return refreshpipeline.DataVersion{}, false, nil
	}
	if err != nil {
		return refreshpipeline.DataVersion{}, false, err
	}
	refreshedAt, err := parseTime(row.RefreshedAt)
	if err != nil {
		return refreshpipeline.DataVersion{}, false, err
	}
	version := refreshpipeline.DataVersion{
		WorkspaceID: row.WorkspaceID, Environment: row.Environment, SemanticModel: row.SemanticModelID,
		SnapshotID: row.SnapshotID, ServingStateID: row.ServingStateID, RefreshedAt: refreshedAt,
		Source: row.Source, PipelineID: row.PipelineID, RunID: row.RunID,
	}
	return version, true, nil
}

func scheduleKey(pipelineID, expression, timezone string) string {
	return pipelineID + "\x00" + expression + "\x00" + timezone
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse refresh pipeline timestamp %q: %w", value, err)
	}
	return parsed.UTC(), nil
}
