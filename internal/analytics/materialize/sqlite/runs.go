package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/refreshpipeline"
	"github.com/Yacobolo/libredash/internal/servingstate"
)

type SQLRunRepository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewSQLRunRepository(db *sql.DB) *SQLRunRepository {
	return &SQLRunRepository{db: db, q: platformdb.New(db)}
}

func (r *SQLRunRepository) CreateRun(ctx context.Context, input materialize.RunInput) (materialize.RunRecord, error) {
	return r.createRun(ctx, input, nil)
}

func (r *SQLRunRepository) CreateScheduledRun(ctx context.Context, input materialize.RunInput, occurrence refreshpipeline.Occurrence) (materialize.RunRecord, error) {
	return r.createRun(ctx, input, &occurrence)
}

func (r *SQLRunRepository) createRun(ctx context.Context, input materialize.RunInput, occurrence *refreshpipeline.Occurrence) (materialize.RunRecord, error) {
	if r == nil || r.db == nil {
		return materialize.RunRecord{}, fmt.Errorf("refresh run database is required")
	}
	normalized, err := normalizeRunInput(input)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if occurrence != nil {
		if normalized.TriggerType != materialize.TriggerSchedule || occurrence.WorkspaceID != normalized.WorkspaceID ||
			normalized.TargetID != occurrence.WorkspaceID+"."+occurrence.PipelineID || occurrence.Environment == "" ||
			occurrence.ArtifactDigest == "" || occurrence.ScheduledAt.IsZero() {
			return materialize.RunRecord{}, fmt.Errorf("scheduled refresh run does not match its claimed occurrence")
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	jobID := newRunID("matjob")
	runID := newRunID("matrun")
	if err := q.CreateRefreshJob(ctx, platformdb.CreateRefreshJobParams{
		ID: jobID, WorkspaceID: normalized.WorkspaceID, ServingStateID: normalized.ServingStateID,
		ModelID: normalized.ModelID, Kind: normalized.JobKind, PayloadJson: normalized.PayloadJSON, Status: materialize.RunStatusQueued,
	}); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := q.CreateRefreshJobRun(ctx, platformdb.CreateRefreshJobRunParams{
		ID: runID, JobID: jobID, PrincipalID: normalized.PrincipalID, Environment: normalized.Environment, TargetType: normalized.TargetType,
		TargetID: normalized.TargetID, TriggerType: normalized.TriggerType, ParentRunID: normalized.ParentRunID,
		RetryOf: normalized.RetryOf, Status: materialize.RunStatusQueued,
	}); err != nil {
		return materialize.RunRecord{}, err
	}
	if occurrence != nil {
		result, err := q.AttachRefreshPipelineRun(ctx, platformdb.AttachRefreshPipelineRunParams{
			RunID: sql.NullString{String: runID, Valid: true}, WorkspaceID: occurrence.WorkspaceID,
			Environment: occurrence.Environment, PipelineID: occurrence.PipelineID,
			ScheduledAt: occurrence.ScheduledAt.UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return materialize.RunRecord{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return materialize.RunRecord{}, err
		}
		if affected != 1 {
			return materialize.RunRecord{}, fmt.Errorf("scheduled refresh occurrence is not claimable")
		}
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, normalized.WorkspaceID, runID)
}

func (r *SQLRunRepository) ClaimNextExecutableJob(ctx context.Context, environment, owner string, lease time.Duration) (materialize.JobRecord, bool, error) {
	if r == nil || r.db == nil {
		return materialize.JobRecord{}, false, fmt.Errorf("refresh run database is required")
	}
	owner = strings.TrimSpace(owner)
	environment = string(servingstate.NormalizeEnvironment(servingstate.Environment(environment)))
	if owner == "" {
		return materialize.JobRecord{}, false, fmt.Errorf("lease owner is required")
	}
	if lease <= 0 {
		lease = time.Minute
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.JobRecord{}, false, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.NextExecutableRefreshJob(ctx, platformdb.NextExecutableRefreshJobParams{
		RefreshPipelineKind: materialize.JobKindRefreshPipeline,
		QueuedStatus:        materialize.RunStatusQueued, RunQueuedStatus: materialize.RunStatusQueued, RunningStatus: materialize.RunStatusRunning,
		Environment: environment,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return materialize.JobRecord{}, false, nil
		}
		return materialize.JobRecord{}, false, err
	}
	job := materialize.JobRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		Kind: row.Kind, PayloadJSON: row.PayloadJson, RunID: row.RunID, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, AttemptCount: int(row.AttemptCount),
	}
	leaseExpr := sqliteLeaseModifier(lease)
	result, err := q.ClaimRefreshJob(ctx, platformdb.ClaimRefreshJobParams{
		RunningStatus: materialize.RunStatusRunning, LeaseOwner: owner, LeaseModifier: leaseExpr,
		ID: job.ID, QueuedStatus: materialize.RunStatusQueued, PreviousRunningStatus: materialize.RunStatusRunning,
	})
	if err != nil {
		return materialize.JobRecord{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return materialize.JobRecord{}, false, err
	}
	if affected == 0 {
		return materialize.JobRecord{}, false, nil
	}
	if err := q.MarkRefreshJobRunClaimed(ctx, platformdb.MarkRefreshJobRunClaimedParams{Status: materialize.RunStatusRunning, ID: job.RunID}); err != nil {
		return materialize.JobRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return materialize.JobRecord{}, false, err
	}
	job.AttemptCount++
	return job, true, nil
}

func (r *SQLRunRepository) RenewJobLease(ctx context.Context, jobID, owner string, lease time.Duration) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("refresh run database is required")
	}
	return r.q.RenewRefreshJobLease(ctx, platformdb.RenewRefreshJobLeaseParams{
		LeaseModifier: sqliteLeaseModifier(lease), ID: strings.TrimSpace(jobID),
		LeaseOwner: strings.TrimSpace(owner), Status: materialize.RunStatusRunning,
	})
}

func (r *SQLRunRepository) JobQueueStats(ctx context.Context, environment string) (materialize.JobQueueStats, error) {
	if r == nil || r.db == nil {
		return materialize.JobQueueStats{}, fmt.Errorf("refresh run database is required")
	}
	row, err := r.q.GetRefreshJobQueueStats(ctx, platformdb.GetRefreshJobQueueStatsParams{
		QueuedStatus: materialize.RunStatusQueued, RunningStatus: materialize.RunStatusRunning,
		StaleRunningStatus: materialize.RunStatusRunning, RefreshPipelineKind: materialize.JobKindRefreshPipeline,
		Environment: string(servingstate.NormalizeEnvironment(servingstate.Environment(environment))),
	})
	if err != nil {
		return materialize.JobQueueStats{}, err
	}
	return materialize.JobQueueStats{QueuedJobs: int(row.QueuedJobs), RunningJobs: int(row.RunningJobs), StaleLeasedJobs: int(row.StaleLeasedJobs)}, nil
}

func (r *SQLRunRepository) GetRun(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID == "" {
		return materialize.RunRecord{}, fmt.Errorf("workspace id is required")
	}
	if runID == "" {
		return materialize.RunRecord{}, fmt.Errorf("run id is required")
	}
	row, err := r.q.GetMaterializationRun(ctx, platformdb.GetMaterializationRunParams{RunID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return materialize.RunRecord{}, err
	}
	return materializationRunFromGetRow(row), nil
}

func (r *SQLRunRepository) ListRuns(ctx context.Context, workspaceID string, page materialize.RunPage) ([]materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	limit := runPageLimit(page)
	cursor := runPageCursor{}
	after := strings.TrimSpace(page.After)
	if after != "" {
		resolved, ok, err := r.runPageCursor(ctx, workspaceID, environmentForPage(page), "", "", after)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []materialize.RunRecord{}, nil
		}
		cursor = resolved
	}
	rows, err := r.q.ListMaterializationRuns(ctx, platformdb.ListMaterializationRunsParams{
		WorkspaceID: workspaceID, Environment: environmentForPage(page), CursorCreatedAt: cursor.CreatedAt, CursorSequence: cursor.Sequence, Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]materialize.RunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, materializationRunFromListRow(row))
	}
	return out, nil
}

func (r *SQLRunRepository) ListTargetRuns(ctx context.Context, workspaceID, targetType, targetID string, page materialize.RunPage) ([]materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if targetType == "" {
		return nil, fmt.Errorf("target type is required")
	}
	if targetID == "" {
		return nil, fmt.Errorf("target id is required")
	}
	limit := runPageLimit(page)
	cursor := runPageCursor{}
	after := strings.TrimSpace(page.After)
	if after != "" {
		resolved, ok, err := r.runPageCursor(ctx, workspaceID, environmentForPage(page), targetType, targetID, after)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []materialize.RunRecord{}, nil
		}
		cursor = resolved
	}
	rows, err := r.q.ListTargetMaterializationRuns(ctx, platformdb.ListTargetMaterializationRunsParams{
		WorkspaceID: workspaceID, Environment: environmentForPage(page), TargetType: targetType, TargetID: targetID,
		CursorCreatedAt: cursor.CreatedAt, CursorSequence: cursor.Sequence, Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]materialize.RunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, materializationRunFromTargetListRow(row))
	}
	return out, nil
}

func (r *SQLRunRepository) ListChildRuns(ctx context.Context, workspaceID, parentRunID string) ([]materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	parentRunID = strings.TrimSpace(parentRunID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if parentRunID == "" {
		return nil, fmt.Errorf("parent run id is required")
	}
	rows, err := r.q.ListChildMaterializationRuns(ctx, platformdb.ListChildMaterializationRunsParams{
		WorkspaceID: workspaceID, ParentRunID: sql.NullString{String: parentRunID, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]materialize.RunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, materializationRunFromChildRow(row))
	}
	return out, nil
}

func (r *SQLRunRepository) LatestTargetRun(ctx context.Context, workspaceID, environment, targetType, targetID string) (materialize.RunRecord, bool, error) {
	runs, err := r.ListTargetRuns(ctx, workspaceID, targetType, targetID, materialize.RunPage{Limit: 1, Environment: environment})
	if err != nil {
		return materialize.RunRecord{}, false, err
	}
	if len(runs) == 0 {
		return materialize.RunRecord{}, false, nil
	}
	return runs[0], true, nil
}

func (r *SQLRunRepository) LatestSuccessfulTargetRun(ctx context.Context, workspaceID, environment, targetType, targetID string) (materialize.RunRecord, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	if workspaceID == "" {
		return materialize.RunRecord{}, false, fmt.Errorf("workspace id is required")
	}
	if targetType == "" {
		return materialize.RunRecord{}, false, fmt.Errorf("target type is required")
	}
	if targetID == "" {
		return materialize.RunRecord{}, false, fmt.Errorf("target id is required")
	}
	row, err := r.q.LatestSuccessfulMaterializationRun(ctx, platformdb.LatestSuccessfulMaterializationRunParams{
		WorkspaceID: workspaceID, Environment: string(servingstate.NormalizeEnvironment(servingstate.Environment(environment))),
		TargetType: targetType, TargetID: targetID, Status: materialize.RunStatusSucceeded,
	})
	if err == sql.ErrNoRows {
		return materialize.RunRecord{}, false, nil
	}
	if err != nil {
		return materialize.RunRecord{}, false, err
	}
	return materializationRunFromLatestRow(row), true, nil
}

func (r *SQLRunRepository) MarkRunRunning(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, materialize.RunStatusRunning, "")
}

func (r *SQLRunRepository) MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, materialize.RunStatusSucceeded, "")
}

func (r *SQLRunRepository) MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (materialize.RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, materialize.RunStatusFailed, message)
}

func (r *SQLRunRepository) CancelRun(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID == "" || runID == "" {
		return materialize.RunRecord{}, fmt.Errorf("workspace id and run id are required")
	}
	prior, err := r.GetRun(ctx, workspaceID, runID)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if prior.ParentRunID != "" || prior.TargetType != materialize.TargetRefreshPipeline || prior.ServingStateID == "" {
		return materialize.RunRecord{}, materialize.ErrRunNotCancellable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	result, err := q.CancelQueuedMaterializationRun(ctx, platformdb.CancelQueuedMaterializationRunParams{
		CancelledStatus: materialize.RunStatusCancelled, RunID: runID,
		QueuedStatus: materialize.RunStatusQueued, WorkspaceID: workspaceID,
	})
	if err != nil {
		return materialize.RunRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if affected == 0 {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return materialize.RunRecord{}, rollbackErr
		}
		if _, getErr := r.GetRun(ctx, workspaceID, runID); getErr != nil {
			return materialize.RunRecord{}, getErr
		}
		return materialize.RunRecord{}, materialize.ErrRunNotCancellable
	}
	if err := q.CancelQueuedChildMaterializationRuns(ctx, platformdb.CancelQueuedChildMaterializationRunsParams{
		CancelledStatus: materialize.RunStatusCancelled, ParentRunID: sql.NullString{String: runID, Valid: true},
		QueuedStatus: materialize.RunStatusQueued, WorkspaceID: workspaceID,
	}); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := q.CancelQueuedChildRefreshJobs(ctx, platformdb.CancelQueuedChildRefreshJobsParams{
		CancelledStatus: materialize.RunStatusCancelled, WorkspaceID: workspaceID,
		QueuedStatus: materialize.RunStatusQueued, ParentRunID: sql.NullString{String: runID, Valid: true},
	}); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := q.CancelQueuedRefreshJobForRun(ctx, platformdb.CancelQueuedRefreshJobForRunParams{
		CancelledStatus: materialize.RunStatusCancelled, RunID: runID,
		WorkspaceID: workspaceID, QueuedStatus: materialize.RunStatusQueued,
	}); err != nil {
		return materialize.RunRecord{}, err
	}
	failed, err := q.FailCancelledRefreshCandidate(ctx, prior.ServingStateID)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	failedCount, err := failed.RowsAffected()
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if failedCount != 1 {
		return materialize.RunRecord{}, fmt.Errorf("refresh candidate is not cancellable")
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, workspaceID, runID)
}

func (r *SQLRunRepository) FailRunsForTerminalServingStates(ctx context.Context, environment, message string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("refresh run database is required")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "refresh did not complete"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	if err := q.FailTerminalServingStateRuns(ctx, platformdb.FailTerminalServingStateRunsParams{
		FailedStatus: materialize.RunStatusFailed, ErrorMessage: message,
		QueuedStatus: materialize.RunStatusQueued, RunningStatus: materialize.RunStatusRunning, Environment: environment,
	}); err != nil {
		return err
	}
	if err := q.FailTerminalServingStateJobs(ctx, platformdb.FailTerminalServingStateJobsParams{
		FailedStatus: materialize.RunStatusFailed, QueuedStatus: materialize.RunStatusQueued, RunningStatus: materialize.RunStatusRunning, Environment: environment,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRunRepository) markRun(ctx context.Context, workspaceID, runID, status, message string) (materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID == "" {
		return materialize.RunRecord{}, fmt.Errorf("workspace id is required")
	}
	if runID == "" {
		return materialize.RunRecord{}, fmt.Errorf("run id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	params := platformdb.MarkMaterializationRunActiveParams{Status: status, ErrorMessage: message, RunID: runID, WorkspaceID: workspaceID}
	var result sql.Result
	if status == materialize.RunStatusSucceeded || status == materialize.RunStatusFailed {
		result, err = q.MarkMaterializationRunTerminal(ctx, platformdb.MarkMaterializationRunTerminalParams(params))
	} else {
		result, err = q.MarkMaterializationRunActive(ctx, params)
	}
	if err != nil {
		return materialize.RunRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if affected == 0 {
		return materialize.RunRecord{}, sql.ErrNoRows
	}
	switch status {
	case materialize.RunStatusSucceeded:
		err = q.CompleteRefreshJobSucceeded(ctx, platformdb.CompleteRefreshJobSucceededParams{RunID: runID, WorkspaceID: workspaceID})
	case materialize.RunStatusFailed:
		err = q.CompleteRefreshJobFailed(ctx, platformdb.CompleteRefreshJobFailedParams{ErrorMessage: message, RunID: runID, WorkspaceID: workspaceID})
	default:
		err = q.UpdateRefreshJobForActiveRun(ctx, platformdb.UpdateRefreshJobForActiveRunParams{NewStatus: status, RunID: runID, WorkspaceID: workspaceID})
	}
	if err != nil {
		return materialize.RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, workspaceID, runID)
}

type materializationRunDBRow struct {
	ID                   string
	WorkspaceID          string
	Environment          string
	ServingStateID       sql.NullString
	ModelID              string
	PrincipalID          sql.NullString
	PrincipalDisplayName string
	TargetType           string
	TargetID             string
	TriggerType          string
	ParentRunID          sql.NullString
	RetryOf              sql.NullString
	Status               string
	CreatedAt            string
	UpdatedAt            string
	StartedAt            string
	FinishedAt           sql.NullString
	Error                string
}

func materializationRunFromGetRow(row platformdb.GetMaterializationRunRow) materialize.RunRecord {
	return materializationRunFromDB(materializationRunDBRow{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID, RetryOf: row.RetryOf, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error,
	})
}

func materializationRunFromChildRow(row platformdb.ListChildMaterializationRunsRow) materialize.RunRecord {
	return materializationRunFromDB(materializationRunDBRow{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID, RetryOf: row.RetryOf, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error,
	})
}

func materializationRunFromLatestRow(row platformdb.LatestSuccessfulMaterializationRunRow) materialize.RunRecord {
	return materializationRunFromDB(materializationRunDBRow{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID, RetryOf: row.RetryOf, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error,
	})
}

func materializationRunFromListRow(row platformdb.ListMaterializationRunsRow) materialize.RunRecord {
	return materializationRunFromDB(materializationRunDBRow{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID, RetryOf: row.RetryOf, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error,
	})
}

func materializationRunFromTargetListRow(row platformdb.ListTargetMaterializationRunsRow) materialize.RunRecord {
	return materializationRunFromDB(materializationRunDBRow{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID, RetryOf: row.RetryOf, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error,
	})
}

func materializationRunFromDB(row materializationRunDBRow) materialize.RunRecord {
	run := materialize.RunRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Environment: row.Environment, ServingStateID: row.ServingStateID.String, ModelID: row.ModelID,
		PrincipalID: row.PrincipalID.String, PrincipalDisplayName: row.PrincipalDisplayName, TargetType: row.TargetType,
		TargetID: row.TargetID, TriggerType: row.TriggerType, ParentRunID: row.ParentRunID.String, RetryOf: row.RetryOf.String, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt.String, Error: row.Error,
	}
	if run.Status == materialize.RunStatusQueued {
		run.StartedAt = ""
	}
	return run
}

type runPageCursor struct {
	CreatedAt string
	Sequence  int64
}

func (r *SQLRunRepository) runPageCursor(ctx context.Context, workspaceID, environment, targetType, targetID, runID string) (runPageCursor, bool, error) {
	row, err := r.q.GetMaterializationRunCursor(ctx, platformdb.GetMaterializationRunCursorParams{
		RunID: runID, WorkspaceID: workspaceID, Environment: environment, TargetType: targetType, TargetID: targetID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return runPageCursor{}, false, nil
		}
		return runPageCursor{}, false, err
	}
	return runPageCursor{CreatedAt: row.CreatedAt, Sequence: row.CreatedSequence}, true, nil
}

type normalizedRunInput struct {
	WorkspaceID    string
	Environment    string
	ModelID        string
	ServingStateID string
	PrincipalID    string
	TargetType     string
	TargetID       string
	TriggerType    string
	ParentRunID    string
	RetryOf        string
	JobKind        string
	PayloadJSON    string
}

func normalizeRunInput(input materialize.RunInput) (normalizedRunInput, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	environment := string(servingstate.NormalizeEnvironment(servingstate.Environment(strings.TrimSpace(input.Environment))))
	modelID := strings.TrimSpace(input.ModelID)
	servingStateID := strings.TrimSpace(input.ServingStateID)
	principalID := strings.TrimSpace(input.PrincipalID)
	targetType := strings.TrimSpace(input.TargetType)
	targetID := strings.TrimSpace(input.TargetID)
	triggerType := strings.TrimSpace(input.TriggerType)
	parentRunID := strings.TrimSpace(input.ParentRunID)
	retryOf := strings.TrimSpace(input.RetryOf)
	jobKind := strings.TrimSpace(input.JobKind)
	payloadJSON := strings.TrimSpace(input.PayloadJSON)
	if workspaceID == "" {
		return normalizedRunInput{}, fmt.Errorf("workspace id is required")
	}
	if modelID == "" {
		return normalizedRunInput{}, fmt.Errorf("model id is required")
	}
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	if err := validateRunTarget(targetType, targetID); err != nil {
		return normalizedRunInput{}, err
	}
	if err := validateRunTrigger(triggerType); err != nil {
		return normalizedRunInput{}, err
	}
	if err := validateJobKind(jobKind); err != nil {
		return normalizedRunInput{}, err
	}
	if parentRunID == "" {
		if targetType != materialize.TargetRefreshPipeline || jobKind != materialize.JobKindRefreshPipeline {
			return normalizedRunInput{}, fmt.Errorf("root refresh runs must target a refresh pipeline")
		}
		if triggerType == materialize.TriggerDependency {
			return normalizedRunInput{}, fmt.Errorf("root refresh runs cannot use dependency trigger")
		}
		if servingStateID == "" {
			return normalizedRunInput{}, fmt.Errorf("root refresh run serving state id is required")
		}
	} else {
		if targetType != materialize.TargetModelTable || triggerType != materialize.TriggerDependency || jobKind != materialize.JobKindChildRun {
			return normalizedRunInput{}, fmt.Errorf("child refresh tasks must be model-table dependencies")
		}
		if retryOf != "" {
			return normalizedRunInput{}, fmt.Errorf("child refresh tasks cannot be retries")
		}
	}
	if retryOf != "" && triggerType != materialize.TriggerRetry {
		return normalizedRunInput{}, fmt.Errorf("retry refresh runs must use retry trigger")
	}
	if triggerType == materialize.TriggerRetry && retryOf == "" {
		return normalizedRunInput{}, fmt.Errorf("retry trigger requires retry_of")
	}
	return normalizedRunInput{
		WorkspaceID:    workspaceID,
		Environment:    environment,
		ModelID:        modelID,
		ServingStateID: servingStateID,
		PrincipalID:    principalID,
		TargetType:     targetType,
		TargetID:       targetID,
		TriggerType:    triggerType,
		ParentRunID:    parentRunID,
		RetryOf:        retryOf,
		JobKind:        jobKind,
		PayloadJSON:    payloadJSON,
	}, nil
}

func environmentForPage(page materialize.RunPage) string {
	return string(servingstate.NormalizeEnvironment(servingstate.Environment(strings.TrimSpace(page.Environment))))
}

func validateRunTarget(targetType, targetID string) error {
	switch targetType {
	case materialize.TargetModelTable, materialize.TargetRefreshPipeline:
	default:
		return fmt.Errorf("unsupported materialization target type %q", targetType)
	}
	if targetID == "" {
		return fmt.Errorf("target id is required")
	}
	return nil
}

func validateRunTrigger(triggerType string) error {
	switch triggerType {
	case materialize.TriggerDependency, materialize.TriggerManual, materialize.TriggerSchedule, materialize.TriggerRetry:
		return nil
	default:
		return fmt.Errorf("unsupported materialization trigger type %q", triggerType)
	}
}

func validateJobKind(kind string) error {
	switch kind {
	case materialize.JobKindRefreshPipeline, materialize.JobKindChildRun:
		return nil
	default:
		return fmt.Errorf("unsupported materialization job kind %q", kind)
	}
}

func sqliteLeaseModifier(duration time.Duration) string {
	seconds := int(duration.Seconds())
	if seconds <= 0 {
		seconds = 60
	}
	return fmt.Sprintf("+%d seconds", seconds)
}

func pageRuns(rows []materialize.RunRecord, page materialize.RunPage) []materialize.RunRecord {
	limit := runPageLimit(page)
	start := 0
	after := strings.TrimSpace(page.After)
	if after != "" {
		start = len(rows)
		for i, row := range rows {
			if row.ID == after {
				start = i + 1
				break
			}
		}
	}
	if start >= len(rows) {
		return []materialize.RunRecord{}
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return append([]materialize.RunRecord(nil), rows[start:end]...)
}

func runPageLimit(page materialize.RunPage) int {
	limit := page.Limit
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}

func newRunID(prefix string) string {
	return prefix + "_" + newRunSecret()[:24]
}

func newRunSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}
