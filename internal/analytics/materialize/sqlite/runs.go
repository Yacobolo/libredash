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
)

type SQLRunRepository struct {
	db *sql.DB
}

func NewSQLRunRepository(db *sql.DB) *SQLRunRepository {
	return &SQLRunRepository{db: db}
}

func (r *SQLRunRepository) CreateRun(ctx context.Context, input materialize.RunInput) (materialize.RunRecord, error) {
	if r == nil || r.db == nil {
		return materialize.RunRecord{}, fmt.Errorf("refresh run database is required")
	}
	normalized, err := normalizeRunInput(input)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	jobID := newRunID("matjob")
	runID := newRunID("matrun")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_jobs (id, workspace_id, serving_state_id, model_id, kind, payload_json, status, queued_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, jobID, normalized.WorkspaceID, normalized.ServingStateID, normalized.ModelID, normalized.JobKind, normalized.PayloadJSON, materialize.RunStatusQueued); err != nil {
		return materialize.RunRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_job_runs (id, job_id, principal_id, target_type, target_id, trigger_type, parent_run_id, retry_of, status)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
	`, runID, jobID, normalized.PrincipalID, normalized.TargetType, normalized.TargetID, normalized.TriggerType, normalized.ParentRunID, normalized.RetryOf, materialize.RunStatusQueued); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, normalized.WorkspaceID, runID)
}

func (r *SQLRunRepository) ClaimNextExecutableJob(ctx context.Context, owner string, lease time.Duration) (materialize.JobRecord, bool, error) {
	if r == nil || r.db == nil {
		return materialize.JobRecord{}, false, fmt.Errorf("refresh run database is required")
	}
	owner = strings.TrimSpace(owner)
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
	row := tx.QueryRowContext(ctx, `
		SELECT j.id, j.workspace_id, COALESCE(j.serving_state_id, ''), j.model_id, j.kind, j.payload_json,
		       r.id, r.target_type, r.target_id, r.trigger_type, j.attempt_count
		FROM refresh_jobs j
		JOIN refresh_job_runs r ON r.job_id = j.id
		WHERE COALESCE(r.parent_run_id, '') = ''
		  AND j.kind IN (?, ?)
		  AND (
		    (j.status = ? AND r.status = ?)
		    OR (j.status = ? AND (j.lease_expires_at IS NULL OR j.lease_expires_at <= CURRENT_TIMESTAMP))
		  )
		ORDER BY COALESCE(NULLIF(j.queued_at, ''), j.created_at) ASC, j.id ASC
		LIMIT 1
	`, materialize.JobKindRefresh, materialize.JobKindWorkspaceAssetRefresh, materialize.RunStatusQueued, materialize.RunStatusQueued, materialize.RunStatusRunning)
	var job materialize.JobRecord
	if err := row.Scan(&job.ID, &job.WorkspaceID, &job.ServingStateID, &job.ModelID, &job.Kind, &job.PayloadJSON, &job.RunID, &job.TargetType, &job.TargetID, &job.TriggerType, &job.AttemptCount); err != nil {
		if err == sql.ErrNoRows {
			return materialize.JobRecord{}, false, nil
		}
		return materialize.JobRecord{}, false, err
	}
	leaseExpr := sqliteLeaseModifier(lease)
	result, err := tx.ExecContext(ctx, `
		UPDATE refresh_jobs
		SET status = ?, started_at = COALESCE(started_at, CURRENT_TIMESTAMP), finished_at = NULL,
		    lease_owner = ?, lease_expires_at = datetime('now', ?),
		    attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND (
		    status = ?
		    OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= CURRENT_TIMESTAMP))
		  )
	`, materialize.RunStatusRunning, owner, leaseExpr, job.ID, materialize.RunStatusQueued, materialize.RunStatusRunning)
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_job_runs
		SET status = ?, started_at = CURRENT_TIMESTAMP, finished_at = NULL, error = ''
		WHERE id = ?
	`, materialize.RunStatusRunning, job.RunID); err != nil {
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
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_jobs
		SET lease_expires_at = datetime('now', ?), updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND lease_owner = ? AND status = ?
	`, sqliteLeaseModifier(lease), strings.TrimSpace(jobID), strings.TrimSpace(owner), materialize.RunStatusRunning)
	return err
}

func (r *SQLRunRepository) JobQueueStats(ctx context.Context) (materialize.JobQueueStats, error) {
	if r == nil || r.db == nil {
		return materialize.JobQueueStats{}, fmt.Errorf("refresh run database is required")
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN j.status = ? THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN j.status = ? AND j.lease_expires_at IS NOT NULL AND j.lease_expires_at > CURRENT_TIMESTAMP THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN j.status = ? AND (j.lease_expires_at IS NULL OR j.lease_expires_at <= CURRENT_TIMESTAMP) THEN 1 ELSE 0 END), 0)
		FROM refresh_jobs j
		JOIN refresh_job_runs r ON r.job_id = j.id
		WHERE COALESCE(r.parent_run_id, '') = ''
		  AND j.kind IN (?, ?)
	`, materialize.RunStatusQueued, materialize.RunStatusRunning, materialize.RunStatusRunning, materialize.JobKindRefresh, materialize.JobKindWorkspaceAssetRefresh)
	var stats materialize.JobQueueStats
	if err := row.Scan(&stats.QueuedJobs, &stats.RunningJobs, &stats.StaleLeasedJobs); err != nil {
		return materialize.JobQueueStats{}, err
	}
	return stats, nil
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
	row := r.db.QueryRowContext(ctx, refreshRunSelect()+`
		WHERE r.id = ? AND j.workspace_id = ?
	`, runID, workspaceID)
	return scanRun(row)
}

func (r *SQLRunRepository) ListRuns(ctx context.Context, workspaceID string, page materialize.RunPage) ([]materialize.RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	limit := runPageLimit(page)
	args := []any{workspaceID}
	after := strings.TrimSpace(page.After)
	cursorClause := ""
	if after != "" {
		cursor, ok, err := r.runPageCursor(ctx, workspaceID, "", "", after)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []materialize.RunRecord{}, nil
		}
		cursorClause = " AND (j.created_at < ? OR (j.created_at = ? AND r.rowid < ?))"
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.RowID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, refreshRunSelect()+`
		WHERE j.workspace_id = ?`+cursorClause+`
		ORDER BY j.created_at DESC, r.rowid DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunRows(rows)
}

func (r *SQLRunRepository) ListModelRuns(ctx context.Context, workspaceID, modelID string, page materialize.RunPage) ([]materialize.RunRecord, error) {
	return r.ListTargetRuns(ctx, workspaceID, materialize.TargetSemanticModel, modelID, page)
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
	args := []any{workspaceID, targetType, targetID}
	after := strings.TrimSpace(page.After)
	cursorClause := ""
	if after != "" {
		cursor, ok, err := r.runPageCursor(ctx, workspaceID, targetType, targetID, after)
		if err != nil {
			return nil, err
		}
		if !ok {
			return []materialize.RunRecord{}, nil
		}
		cursorClause = " AND (j.created_at < ? OR (j.created_at = ? AND r.rowid < ?))"
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.RowID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, refreshRunSelect()+`
		WHERE j.workspace_id = ? AND r.target_type = ? AND r.target_id = ?`+cursorClause+`
		ORDER BY j.created_at DESC, r.rowid DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunRows(rows)
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
	rows, err := r.db.QueryContext(ctx, refreshRunSelect()+`
		WHERE j.workspace_id = ? AND r.parent_run_id = ?
		ORDER BY r.rowid ASC
	`, workspaceID, parentRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunRows(rows)
}

func (r *SQLRunRepository) LatestModelRun(ctx context.Context, workspaceID, modelID string) (materialize.RunRecord, bool, error) {
	return r.LatestTargetRun(ctx, workspaceID, materialize.TargetSemanticModel, modelID)
}

func (r *SQLRunRepository) LatestTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (materialize.RunRecord, bool, error) {
	runs, err := r.ListTargetRuns(ctx, workspaceID, targetType, targetID, materialize.RunPage{Limit: 1})
	if err != nil {
		return materialize.RunRecord{}, false, err
	}
	if len(runs) == 0 {
		return materialize.RunRecord{}, false, nil
	}
	return runs[0], true, nil
}

func (r *SQLRunRepository) LatestSuccessfulModelRun(ctx context.Context, workspaceID, modelID string) (materialize.RunRecord, bool, error) {
	return r.LatestSuccessfulTargetRun(ctx, workspaceID, materialize.TargetSemanticModel, modelID)
}

func (r *SQLRunRepository) LatestSuccessfulTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (materialize.RunRecord, bool, error) {
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
	row := r.db.QueryRowContext(ctx, refreshRunSelect()+`
		WHERE j.workspace_id = ? AND r.target_type = ? AND r.target_id = ? AND r.status = ?
		ORDER BY j.created_at DESC, r.rowid DESC
		LIMIT 1
	`, workspaceID, targetType, targetID, materialize.RunStatusSucceeded)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return materialize.RunRecord{}, false, nil
	}
	if err != nil {
		return materialize.RunRecord{}, false, err
	}
	return run, true, nil
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE refresh_job_runs
		SET status = ?, finished_at = CURRENT_TIMESTAMP, error = ''
		WHERE id = ? AND status = ?
		  AND job_id IN (SELECT id FROM refresh_jobs WHERE workspace_id = ?)
	`, materialize.RunStatusCancelled, runID, materialize.RunStatusQueued, workspaceID)
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_jobs
		SET status = ?, finished_at = CURRENT_TIMESTAMP, lease_owner = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT job_id FROM refresh_job_runs WHERE id = ?) AND workspace_id = ? AND status = ?
	`, materialize.RunStatusCancelled, runID, workspaceID, materialize.RunStatusQueued); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, workspaceID, runID)
}

func (r *SQLRunRepository) FailRunsForTerminalServingStates(ctx context.Context, message string) error {
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_job_runs
		SET status = ?, finished_at = CURRENT_TIMESTAMP,
		    error = CASE WHEN error <> '' THEN error ELSE ? END
		WHERE status IN (?, ?)
		  AND job_id IN (
		    SELECT j.id
		    FROM refresh_jobs j
		    JOIN serving_states d ON d.id = j.serving_state_id
		    WHERE d.status IN ('failed', 'delete_scheduled', 'deleted')
		  )
	`, materialize.RunStatusFailed, message, materialize.RunStatusQueued, materialize.RunStatusRunning); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_jobs
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE status IN (?, ?)
		  AND serving_state_id IN (
		    SELECT id FROM serving_states WHERE status IN ('failed', 'delete_scheduled', 'deleted')
		  )
	`, materialize.RunStatusFailed, materialize.RunStatusQueued, materialize.RunStatusRunning); err != nil {
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
	finishedExpr := "finished_at"
	if status == materialize.RunStatusSucceeded || status == materialize.RunStatusFailed {
		finishedExpr = "CURRENT_TIMESTAMP"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return materialize.RunRecord{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE refresh_job_runs
		SET status = ?, finished_at = %s, error = ?
		WHERE id = ?
		  AND job_id IN (SELECT id FROM refresh_jobs WHERE workspace_id = ?)
	`, finishedExpr), status, message, runID, workspaceID)
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_jobs
		SET status = ?, updated_at = CURRENT_TIMESTAMP,
		    finished_at = CASE WHEN ? IN (?, ?) THEN CURRENT_TIMESTAMP ELSE finished_at END,
		    lease_owner = CASE WHEN ? IN (?, ?) THEN '' ELSE lease_owner END,
		    lease_expires_at = CASE WHEN ? IN (?, ?) THEN NULL ELSE lease_expires_at END,
		    last_error = CASE WHEN ? = ? THEN ? ELSE last_error END
		WHERE id = (SELECT job_id FROM refresh_job_runs WHERE id = ?)
		  AND workspace_id = ?
	`, status, status, materialize.RunStatusSucceeded, materialize.RunStatusFailed, status, materialize.RunStatusSucceeded, materialize.RunStatusFailed, status, materialize.RunStatusSucceeded, materialize.RunStatusFailed, status, materialize.RunStatusFailed, message, runID, workspaceID); err != nil {
		return materialize.RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return materialize.RunRecord{}, err
	}
	return r.GetRun(ctx, workspaceID, runID)
}

type runScanner interface {
	Scan(dest ...any) error
}

type runRows interface {
	Next() bool
	Err() error
	runScanner
}

type runPageCursor struct {
	CreatedAt string
	RowID     int64
}

func (r *SQLRunRepository) runPageCursor(ctx context.Context, workspaceID, targetType, targetID, runID string) (runPageCursor, bool, error) {
	args := []any{runID, workspaceID}
	targetClause := ""
	if targetType != "" || targetID != "" {
		targetClause = " AND r.target_type = ? AND r.target_id = ?"
		args = append(args, targetType, targetID)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT j.created_at, r.rowid
		FROM refresh_job_runs r
		JOIN refresh_jobs j ON j.id = r.job_id
		WHERE r.id = ? AND j.workspace_id = ?`+targetClause+`
	`, args...)
	var cursor runPageCursor
	if err := row.Scan(&cursor.CreatedAt, &cursor.RowID); err != nil {
		if err == sql.ErrNoRows {
			return runPageCursor{}, false, nil
		}
		return runPageCursor{}, false, err
	}
	return cursor, true, nil
}

func scanRunRows(rows runRows) ([]materialize.RunRecord, error) {
	var out []materialize.RunRecord
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanRun(row runScanner) (materialize.RunRecord, error) {
	var run materialize.RunRecord
	var servingStateID, principalID, principalDisplayName, parentRunID, retryOf, finishedAt sql.NullString
	if err := row.Scan(
		&run.ID,
		&run.WorkspaceID,
		&servingStateID,
		&run.ModelID,
		&principalID,
		&principalDisplayName,
		&run.TargetType,
		&run.TargetID,
		&run.TriggerType,
		&parentRunID,
		&retryOf,
		&run.Status,
		&run.CreatedAt,
		&run.UpdatedAt,
		&run.StartedAt,
		&finishedAt,
		&run.Error,
	); err != nil {
		return materialize.RunRecord{}, err
	}
	if servingStateID.Valid {
		run.ServingStateID = servingStateID.String
	}
	if principalID.Valid {
		run.PrincipalID = principalID.String
	}
	if principalDisplayName.Valid {
		run.PrincipalDisplayName = principalDisplayName.String
	}
	if parentRunID.Valid {
		run.ParentRunID = parentRunID.String
	}
	if retryOf.Valid {
		run.RetryOf = retryOf.String
	}
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.String
	}
	if run.Status == materialize.RunStatusQueued {
		run.StartedAt = ""
	}
	return run, nil
}

func refreshRunSelect() string {
	return `
		SELECT r.id, j.workspace_id, j.serving_state_id, j.model_id, r.principal_id, COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name, r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status, j.created_at, j.updated_at, r.started_at, r.finished_at, r.error
		FROM refresh_job_runs r
		JOIN refresh_jobs j ON j.id = r.job_id
		LEFT JOIN principals p ON p.id = r.principal_id
	`
}

type normalizedRunInput struct {
	WorkspaceID    string
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
	if targetType == "" {
		targetType = materialize.TargetSemanticModel
	}
	if targetID == "" && targetType == materialize.TargetSemanticModel {
		targetID = modelID
	}
	if triggerType == "" {
		triggerType = materialize.TriggerDirect
	}
	if jobKind == "" {
		if parentRunID != "" {
			jobKind = materialize.JobKindChildRun
		} else {
			jobKind = materialize.JobKindRefresh
		}
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
	return normalizedRunInput{
		WorkspaceID:    workspaceID,
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

func validateRunTarget(targetType, targetID string) error {
	switch targetType {
	case materialize.TargetSemanticModel, materialize.TargetModelTable:
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
	case materialize.TriggerDirect, materialize.TriggerSemanticModel, materialize.TriggerDependency:
		return nil
	default:
		return fmt.Errorf("unsupported materialization trigger type %q", triggerType)
	}
}

func validateJobKind(kind string) error {
	switch kind {
	case materialize.JobKindRefresh, materialize.JobKindWorkspaceAssetRefresh, materialize.JobKindChildRun:
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
