package refreshpipeline

import (
	"context"
	"errors"
	"time"
)

const (
	DataVersionSourcePublish = "publish"
	DataVersionSourceRefresh = "refresh"
)

type ReconcileInput struct {
	WorkspaceID    string
	Environment    string
	ArtifactDigest string
	Pipelines      []Definition
	Now            time.Time
}

type Occurrence struct {
	WorkspaceID    string
	Environment    string
	PipelineID     string
	SemanticModel  string
	ArtifactDigest string
	ScheduledAt    time.Time
}

type DataVersion struct {
	WorkspaceID    string
	Environment    string
	SemanticModel  string
	SnapshotID     int64
	ServingStateID string
	RefreshedAt    time.Time
	Source         string
	PipelineID     string
	RunID          string
}

type Repository interface {
	Reconcile(context.Context, ReconcileInput) error
	ClaimDue(context.Context, string, time.Time) ([]Occurrence, error)
	AttachRun(context.Context, Occurrence, string) error
	ReleaseOccurrence(context.Context, Occurrence) error
	NextRun(context.Context, string, string, string) (time.Time, bool, error)
	SaveDataVersion(context.Context, DataVersion) error
	DataVersion(context.Context, string, string, string) (DataVersion, bool, error)
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type Trigger func(context.Context, Occurrence) (string, error)

type Scheduler struct {
	Repository  Repository
	Clock       Clock
	Trigger     Trigger
	Environment string
}

func (scheduler Scheduler) DispatchDue(ctx context.Context) error {
	clock := scheduler.Clock
	if clock == nil {
		clock = RealClock{}
	}
	occurrences, err := scheduler.Repository.ClaimDue(ctx, scheduler.Environment, clock.Now())
	if err != nil {
		return err
	}
	var dispatchErrors []error
	for _, occurrence := range occurrences {
		runID, triggerErr := scheduler.Trigger(ctx, occurrence)
		if triggerErr != nil {
			dispatchErrors = append(dispatchErrors, triggerErr)
			if releaseErr := scheduler.Repository.ReleaseOccurrence(ctx, occurrence); releaseErr != nil {
				dispatchErrors = append(dispatchErrors, releaseErr)
			}
			continue
		}
		if err := scheduler.Repository.AttachRun(ctx, occurrence, runID); err != nil {
			dispatchErrors = append(dispatchErrors, err)
		}
	}
	return errors.Join(dispatchErrors...)
}
