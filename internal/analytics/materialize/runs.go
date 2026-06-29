package materialize

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	RunStatusQueued    = "queued"
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"

	TargetSemanticModel = "semantic_model"
	TargetModelTable    = "model_table"

	TriggerDirect        = "direct"
	TriggerSemanticModel = "semantic_model"
	TriggerDependency    = "dependency"
)

type RunRecord struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspaceId"`
	ModelID              string `json:"modelId"`
	DeploymentID         string `json:"deploymentId,omitempty"`
	PrincipalID          string `json:"principalId,omitempty"`
	PrincipalDisplayName string `json:"principalDisplayName,omitempty"`
	TargetType           string `json:"targetType"`
	TargetID             string `json:"targetId"`
	TriggerType          string `json:"triggerType"`
	ParentRunID          string `json:"parentRunId,omitempty"`
	Status               string `json:"status"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
	StartedAt            string `json:"startedAt,omitempty"`
	FinishedAt           string `json:"finishedAt,omitempty"`
	Error                string `json:"error,omitempty"`
}

type RunInput struct {
	WorkspaceID  string
	ModelID      string
	DeploymentID string
	PrincipalID  string
	TargetType   string
	TargetID     string
	TriggerType  string
	ParentRunID  string
}

type RunRepository interface {
	CreateRun(ctx context.Context, input RunInput) (RunRecord, error)
	GetRun(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	ListRuns(ctx context.Context, workspaceID string, page RunPage) ([]RunRecord, error)
	ListTargetRuns(ctx context.Context, workspaceID, targetType, targetID string, page RunPage) ([]RunRecord, error)
	LatestTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (RunRecord, bool, error)
	LatestSuccessfulTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (RunRecord, bool, error)
	MarkRunRunning(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (RunRecord, error)
}

type RunPage struct {
	Limit int
	After string
}

type SQLRunRepository struct {
	db *sql.DB
}

func NewSQLRunRepository(db *sql.DB) *SQLRunRepository {
	return &SQLRunRepository{db: db}
}

func (r *SQLRunRepository) CreateRun(ctx context.Context, input RunInput) (RunRecord, error) {
	if r == nil || r.db == nil {
		return RunRecord{}, fmt.Errorf("materialization run database is required")
	}
	normalized, err := normalizeRunInput(input)
	if err != nil {
		return RunRecord{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RunRecord{}, err
	}
	defer tx.Rollback()
	jobID := newRunID("matjob")
	runID := newRunID("matrun")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO materialization_jobs (id, workspace_id, deployment_id, model_id, status)
		VALUES (?, ?, NULLIF(?, ''), ?, ?)
	`, jobID, normalized.WorkspaceID, normalized.DeploymentID, normalized.ModelID, RunStatusQueued); err != nil {
		return RunRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO materialization_job_runs (id, job_id, principal_id, target_type, target_id, trigger_type, parent_run_id, status)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?)
	`, runID, jobID, normalized.PrincipalID, normalized.TargetType, normalized.TargetID, normalized.TriggerType, normalized.ParentRunID, RunStatusQueued); err != nil {
		return RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RunRecord{}, err
	}
	return r.GetRun(ctx, normalized.WorkspaceID, runID)
}

func (r *SQLRunRepository) GetRun(ctx context.Context, workspaceID, runID string) (RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID == "" {
		return RunRecord{}, fmt.Errorf("workspace id is required")
	}
	if runID == "" {
		return RunRecord{}, fmt.Errorf("run id is required")
	}
	row := r.db.QueryRowContext(ctx, materializationRunSelect()+`
		WHERE r.id = ? AND j.workspace_id = ?
	`, runID, workspaceID)
	return scanRun(row)
}

func (r *SQLRunRepository) ListRuns(ctx context.Context, workspaceID string, page RunPage) ([]RunRecord, error) {
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
			return []RunRecord{}, nil
		}
		cursorClause = " AND (j.created_at < ? OR (j.created_at = ? AND r.rowid < ?))"
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.RowID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, materializationRunSelect()+`
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

func (r *SQLRunRepository) ListModelRuns(ctx context.Context, workspaceID, modelID string, page RunPage) ([]RunRecord, error) {
	return r.ListTargetRuns(ctx, workspaceID, TargetSemanticModel, modelID, page)
}

func (r *SQLRunRepository) ListTargetRuns(ctx context.Context, workspaceID, targetType, targetID string, page RunPage) ([]RunRecord, error) {
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
			return []RunRecord{}, nil
		}
		cursorClause = " AND (j.created_at < ? OR (j.created_at = ? AND r.rowid < ?))"
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.RowID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, materializationRunSelect()+`
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

func (r *SQLRunRepository) LatestModelRun(ctx context.Context, workspaceID, modelID string) (RunRecord, bool, error) {
	return r.LatestTargetRun(ctx, workspaceID, TargetSemanticModel, modelID)
}

func (r *SQLRunRepository) LatestTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (RunRecord, bool, error) {
	runs, err := r.ListTargetRuns(ctx, workspaceID, targetType, targetID, RunPage{Limit: 1})
	if err != nil {
		return RunRecord{}, false, err
	}
	if len(runs) == 0 {
		return RunRecord{}, false, nil
	}
	return runs[0], true, nil
}

func (r *SQLRunRepository) LatestSuccessfulModelRun(ctx context.Context, workspaceID, modelID string) (RunRecord, bool, error) {
	return r.LatestSuccessfulTargetRun(ctx, workspaceID, TargetSemanticModel, modelID)
}

func (r *SQLRunRepository) LatestSuccessfulTargetRun(ctx context.Context, workspaceID, targetType, targetID string) (RunRecord, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	if workspaceID == "" {
		return RunRecord{}, false, fmt.Errorf("workspace id is required")
	}
	if targetType == "" {
		return RunRecord{}, false, fmt.Errorf("target type is required")
	}
	if targetID == "" {
		return RunRecord{}, false, fmt.Errorf("target id is required")
	}
	row := r.db.QueryRowContext(ctx, materializationRunSelect()+`
		WHERE j.workspace_id = ? AND r.target_type = ? AND r.target_id = ? AND r.status = ?
		ORDER BY j.created_at DESC, r.rowid DESC
		LIMIT 1
	`, workspaceID, targetType, targetID, RunStatusSucceeded)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return RunRecord{}, false, nil
	}
	if err != nil {
		return RunRecord{}, false, err
	}
	return run, true, nil
}

func (r *SQLRunRepository) MarkRunRunning(ctx context.Context, workspaceID, runID string) (RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, RunStatusRunning, "")
}

func (r *SQLRunRepository) MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, RunStatusSucceeded, "")
}

func (r *SQLRunRepository) MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (RunRecord, error) {
	return r.markRun(ctx, workspaceID, runID, RunStatusFailed, message)
}

func (r *SQLRunRepository) markRun(ctx context.Context, workspaceID, runID, status, message string) (RunRecord, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	if workspaceID == "" {
		return RunRecord{}, fmt.Errorf("workspace id is required")
	}
	if runID == "" {
		return RunRecord{}, fmt.Errorf("run id is required")
	}
	finishedExpr := "finished_at"
	if status == RunStatusSucceeded || status == RunStatusFailed {
		finishedExpr = "CURRENT_TIMESTAMP"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RunRecord{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE materialization_job_runs
		SET status = ?, finished_at = %s, error = ?
		WHERE id = ?
		  AND job_id IN (SELECT id FROM materialization_jobs WHERE workspace_id = ?)
	`, finishedExpr), status, message, runID, workspaceID)
	if err != nil {
		return RunRecord{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RunRecord{}, err
	}
	if affected == 0 {
		return RunRecord{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE materialization_jobs
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT job_id FROM materialization_job_runs WHERE id = ?)
		  AND workspace_id = ?
	`, status, runID, workspaceID); err != nil {
		return RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RunRecord{}, err
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
		FROM materialization_job_runs r
		JOIN materialization_jobs j ON j.id = r.job_id
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

func scanRunRows(rows runRows) ([]RunRecord, error) {
	var out []RunRecord
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

func scanRun(row runScanner) (RunRecord, error) {
	var run RunRecord
	var deploymentID, principalID, principalDisplayName, parentRunID, finishedAt sql.NullString
	if err := row.Scan(
		&run.ID,
		&run.WorkspaceID,
		&deploymentID,
		&run.ModelID,
		&principalID,
		&principalDisplayName,
		&run.TargetType,
		&run.TargetID,
		&run.TriggerType,
		&parentRunID,
		&run.Status,
		&run.CreatedAt,
		&run.UpdatedAt,
		&run.StartedAt,
		&finishedAt,
		&run.Error,
	); err != nil {
		return RunRecord{}, err
	}
	if deploymentID.Valid {
		run.DeploymentID = deploymentID.String
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
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.String
	}
	if run.Status == RunStatusQueued {
		run.StartedAt = ""
	}
	return run, nil
}

func materializationRunSelect() string {
	return `
		SELECT r.id, j.workspace_id, j.deployment_id, j.model_id, r.principal_id, COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name, r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.status, j.created_at, j.updated_at, r.started_at, r.finished_at, r.error
		FROM materialization_job_runs r
		JOIN materialization_jobs j ON j.id = r.job_id
		LEFT JOIN principals p ON p.id = r.principal_id
	`
}

type normalizedRunInput struct {
	WorkspaceID  string
	ModelID      string
	DeploymentID string
	PrincipalID  string
	TargetType   string
	TargetID     string
	TriggerType  string
	ParentRunID  string
}

func normalizeRunInput(input RunInput) (normalizedRunInput, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	modelID := strings.TrimSpace(input.ModelID)
	deploymentID := strings.TrimSpace(input.DeploymentID)
	principalID := strings.TrimSpace(input.PrincipalID)
	targetType := strings.TrimSpace(input.TargetType)
	targetID := strings.TrimSpace(input.TargetID)
	triggerType := strings.TrimSpace(input.TriggerType)
	parentRunID := strings.TrimSpace(input.ParentRunID)
	if workspaceID == "" {
		return normalizedRunInput{}, fmt.Errorf("workspace id is required")
	}
	if modelID == "" {
		return normalizedRunInput{}, fmt.Errorf("model id is required")
	}
	if targetType == "" {
		targetType = TargetSemanticModel
	}
	if targetID == "" && targetType == TargetSemanticModel {
		targetID = modelID
	}
	if triggerType == "" {
		triggerType = TriggerDirect
	}
	if err := validateRunTarget(targetType, targetID); err != nil {
		return normalizedRunInput{}, err
	}
	if err := validateRunTrigger(triggerType); err != nil {
		return normalizedRunInput{}, err
	}
	return normalizedRunInput{
		WorkspaceID:  workspaceID,
		ModelID:      modelID,
		DeploymentID: deploymentID,
		PrincipalID:  principalID,
		TargetType:   targetType,
		TargetID:     targetID,
		TriggerType:  triggerType,
		ParentRunID:  parentRunID,
	}, nil
}

func validateRunTarget(targetType, targetID string) error {
	switch targetType {
	case TargetSemanticModel, TargetModelTable:
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
	case TriggerDirect, TriggerSemanticModel, TriggerDependency:
		return nil
	default:
		return fmt.Errorf("unsupported materialization trigger type %q", triggerType)
	}
}

func pageRuns(rows []RunRecord, page RunPage) []RunRecord {
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
		return []RunRecord{}
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return append([]RunRecord(nil), rows[start:end]...)
}

func runPageLimit(page RunPage) int {
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
