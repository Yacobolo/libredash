package materialize

import (
	"context"
	"errors"
)

var ErrRunNotCancellable = errors.New("refresh run is not cancellable")

const (
	RunStatusQueued    = "queued"
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"

	TargetModelTable      = "model_table"
	TargetRefreshPipeline = "refresh_pipeline"

	TriggerDependency = "dependency"
	TriggerManual     = "manual"
	TriggerSchedule   = "schedule"
	TriggerRetry      = "retry"

	JobKindRefreshPipeline = "refresh_pipeline"
	JobKindChildRun        = "child_run"
)

type RunRecord struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspaceId"`
	Environment          string `json:"-"`
	ModelID              string `json:"modelId"`
	ServingStateID       string `json:"servingStateId,omitempty"`
	PrincipalID          string `json:"principalId,omitempty"`
	PrincipalDisplayName string `json:"principalDisplayName,omitempty"`
	TargetType           string `json:"targetType"`
	TargetID             string `json:"targetId"`
	TriggerType          string `json:"triggerType"`
	ParentRunID          string `json:"parentRunId,omitempty"`
	RetryOf              string `json:"retryOf,omitempty"`
	Status               string `json:"status"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
	StartedAt            string `json:"startedAt,omitempty"`
	FinishedAt           string `json:"finishedAt,omitempty"`
	Error                string `json:"error,omitempty"`
}

type RunInput struct {
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

type JobRecord struct {
	ID             string
	WorkspaceID    string
	Environment    string
	ServingStateID string
	ModelID        string
	Kind           string
	PayloadJSON    string
	RunID          string
	TargetType     string
	TargetID       string
	TriggerType    string
	AttemptCount   int
}

type JobQueueStats struct {
	QueuedJobs      int
	RunningJobs     int
	StaleLeasedJobs int
}

type RunRepository interface {
	CreateRun(ctx context.Context, input RunInput) (RunRecord, error)
	GetRun(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	ListRuns(ctx context.Context, workspaceID string, page RunPage) ([]RunRecord, error)
	ListTargetRuns(ctx context.Context, workspaceID, targetType, targetID string, page RunPage) ([]RunRecord, error)
	ListChildRuns(ctx context.Context, workspaceID, parentRunID string) ([]RunRecord, error)
	LatestTargetRun(ctx context.Context, workspaceID, environment, targetType, targetID string) (RunRecord, bool, error)
	LatestSuccessfulTargetRun(ctx context.Context, workspaceID, environment, targetType, targetID string) (RunRecord, bool, error)
	MarkRunRunning(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (RunRecord, error)
	MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (RunRecord, error)
}

type RunPage struct {
	Limit       int
	After       string
	Environment string
}
