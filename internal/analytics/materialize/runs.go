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
)

type RefreshRunner interface {
	RefreshMaterializations(ctx context.Context, modelID string) error
}

type RunRecord struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspaceId"`
	ModelID      string `json:"modelId"`
	DeploymentID string `json:"deploymentId,omitempty"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	StartedAt    string `json:"startedAt,omitempty"`
	FinishedAt   string `json:"finishedAt,omitempty"`
	Error        string `json:"error,omitempty"`
}

type RunInput struct {
	WorkspaceID  string
	ModelID      string
	DeploymentID string
}

type RunRepository interface {
	CreateRun(ctx context.Context, input RunInput) (RunRecord, error)
	GetRun(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	ListRuns(ctx context.Context, workspaceID string, page RunPage) ([]RunRecord, error)
	MarkRunRunning(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (RunRecord, error)
}

type RunPage struct {
	Limit int
	After string
}

type RunService struct {
	Repo   RunRepository
	Runner RefreshRunner
}

func (s RunService) Enqueue(ctx context.Context, input RunInput) (RunRecord, error) {
	if s.Repo == nil {
		return RunRecord{}, fmt.Errorf("materialization run repository is required")
	}
	return s.Repo.CreateRun(ctx, input)
}

func (s RunService) Execute(ctx context.Context, workspaceID, runID string) (RunRecord, error) {
	if s.Repo == nil {
		return RunRecord{}, fmt.Errorf("materialization run repository is required")
	}
	if s.Runner == nil {
		return RunRecord{}, fmt.Errorf("materialization refresh runner is required")
	}
	run, err := s.Repo.MarkRunRunning(ctx, workspaceID, runID)
	if err != nil {
		return RunRecord{}, err
	}
	if err := s.Runner.RefreshMaterializations(ctx, run.ModelID); err != nil {
		failed, finishErr := s.Repo.MarkRunFailed(ctx, workspaceID, runID, err.Error())
		if finishErr != nil {
			return failed, finishErr
		}
		return failed, err
	}
	return s.Repo.MarkRunSucceeded(ctx, workspaceID, runID)
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
	workspaceID, modelID, deploymentID, err := normalizeRunInput(input)
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
	`, jobID, workspaceID, deploymentID, modelID, RunStatusQueued); err != nil {
		return RunRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO materialization_job_runs (id, job_id, status)
		VALUES (?, ?, ?)
	`, runID, jobID, RunStatusQueued); err != nil {
		return RunRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RunRecord{}, err
	}
	return r.GetRun(ctx, workspaceID, runID)
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
	rows, err := r.db.QueryContext(ctx, materializationRunSelect()+`
		WHERE j.workspace_id = ?
		ORDER BY j.created_at DESC, r.id DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
	return pageRuns(out, page), nil
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

func scanRun(row runScanner) (RunRecord, error) {
	var run RunRecord
	var deploymentID, finishedAt sql.NullString
	if err := row.Scan(
		&run.ID,
		&run.WorkspaceID,
		&deploymentID,
		&run.ModelID,
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
		SELECT r.id, j.workspace_id, j.deployment_id, j.model_id, r.status, j.created_at, j.updated_at, r.started_at, r.finished_at, r.error
		FROM materialization_job_runs r
		JOIN materialization_jobs j ON j.id = r.job_id
	`
}

func normalizeRunInput(input RunInput) (string, string, string, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	modelID := strings.TrimSpace(input.ModelID)
	deploymentID := strings.TrimSpace(input.DeploymentID)
	if workspaceID == "" {
		return "", "", "", fmt.Errorf("workspace id is required")
	}
	if modelID == "" {
		return "", "", "", fmt.Errorf("model id is required")
	}
	return workspaceID, modelID, deploymentID, nil
}

func pageRuns(rows []RunRecord, page RunPage) []RunRecord {
	limit := page.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
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
