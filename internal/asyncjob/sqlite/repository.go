package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/asyncjob"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
)

type Repository struct{ q *platformdb.Queries }

func NewRepository(db platformdb.DBTX) *Repository { return &Repository{q: platformdb.New(db)} }

func (r *Repository) Enqueue(ctx context.Context, input asyncjob.EnqueueInput) (asyncjob.Job, error) {
	input.ID, input.Kind = strings.TrimSpace(input.ID), strings.TrimSpace(input.Kind)
	input.ResourceKind, input.ResourceID = strings.TrimSpace(input.ResourceKind), strings.TrimSpace(input.ResourceID)
	if input.ID == "" || input.Kind == "" || input.ResourceKind == "" || input.ResourceID == "" || !json.Valid(input.Payload) {
		return asyncjob.Job{}, fmt.Errorf("invalid async job")
	}
	digest := jobDigest(input)
	err := r.q.EnqueueAPIAsyncJob(ctx, platformdb.EnqueueAPIAsyncJobParams{ID: input.ID, JobKind: input.Kind,
		ResourceKind: input.ResourceKind, ResourceID: input.ResourceID, PayloadJson: string(input.Payload), RequestDigest: digest})
	if err != nil {
		existing, getErr := r.Get(ctx, input.ID)
		if getErr == nil {
			if storedDigest, scanErr := r.q.GetAPIAsyncJobDigest(ctx, input.ID); scanErr == nil && storedDigest == digest {
				return existing, nil
			}
			return asyncjob.Job{}, asyncjob.ErrConflict
		}
		return asyncjob.Job{}, err
	}
	return r.Get(ctx, input.ID)
}

func jobDigest(input asyncjob.EnqueueInput) string {
	sum := sha256.Sum256([]byte(input.Kind + "\x00" + input.ResourceKind + "\x00" + input.ResourceID + "\x00" + string(input.Payload)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (r *Repository) Get(ctx context.Context, id string) (asyncjob.Job, error) {
	row, err := r.q.GetAPIAsyncJob(ctx, strings.TrimSpace(id))
	if errors.Is(err, sql.ErrNoRows) {
		return asyncjob.Job{}, asyncjob.ErrNotFound
	}
	return jobFromGetRow(row), err
}

func (r *Repository) Claim(ctx context.Context, owner string, lease time.Duration) (asyncjob.Job, bool, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || lease <= 0 {
		return asyncjob.Job{}, false, fmt.Errorf("worker owner and positive lease are required")
	}
	modifier := fmt.Sprintf("+%d seconds", max(1, int(lease.Seconds())))
	row, err := r.q.ClaimAPIAsyncJob(ctx, platformdb.ClaimAPIAsyncJobParams{LeaseOwner: owner, LeaseModifier: modifier})
	if errors.Is(err, sql.ErrNoRows) {
		return asyncjob.Job{}, false, nil
	}
	job := jobFromClaimRow(row)
	return job, err == nil, err
}

func (r *Repository) Renew(ctx context.Context, id, owner string, lease time.Duration) error {
	modifier := fmt.Sprintf("+%d seconds", max(1, int(lease.Seconds())))
	changed, err := r.q.RenewAPIAsyncJob(ctx, platformdb.RenewAPIAsyncJobParams{LeaseModifier: modifier, ID: strings.TrimSpace(id), LeaseOwner: strings.TrimSpace(owner)})
	return requireChanged(changed, err)
}

func (r *Repository) Complete(ctx context.Context, id, owner string) error {
	changed, err := r.q.CompleteAPIAsyncJob(ctx, platformdb.CompleteAPIAsyncJobParams{ID: strings.TrimSpace(id), LeaseOwner: strings.TrimSpace(owner)})
	return requireChanged(changed, err)
}

func (r *Repository) Fail(ctx context.Context, id, owner string, problem []byte) error {
	if !json.Valid(problem) {
		problem = []byte(`{"code":"ASYNC_JOB_FAILED"}`)
	}
	changed, err := r.q.FailAPIAsyncJob(ctx, platformdb.FailAPIAsyncJobParams{ErrorJson: string(problem), ID: strings.TrimSpace(id), LeaseOwner: strings.TrimSpace(owner)})
	return requireChanged(changed, err)
}

func (r *Repository) Cancel(ctx context.Context, id string) error {
	changed, err := r.q.CancelQueuedAPIAsyncJob(ctx, strings.TrimSpace(id))
	return requireChanged(changed, err)
}

func (r *Repository) CancelClaimed(ctx context.Context, id, owner string) error {
	changed, err := r.q.CancelClaimedAPIAsyncJob(ctx, platformdb.CancelClaimedAPIAsyncJobParams{ID: strings.TrimSpace(id), LeaseOwner: strings.TrimSpace(owner)})
	return requireChanged(changed, err)
}

func (r *Repository) AppendEvent(ctx context.Context, resourceKind, resourceID, eventType string, data []byte) (asyncjob.Event, error) {
	resourceKind, resourceID, eventType = strings.TrimSpace(resourceKind), strings.TrimSpace(resourceID), strings.TrimSpace(eventType)
	if resourceKind == "" || resourceID == "" || eventType == "" || !json.Valid(data) {
		return asyncjob.Event{}, fmt.Errorf("invalid async event")
	}
	row, err := r.q.AppendAPIAsyncEvent(ctx, platformdb.AppendAPIAsyncEventParams{ResourceKind: resourceKind, ResourceID: resourceID, EventType: eventType, DataJson: string(data)})
	if err != nil {
		return asyncjob.Event{}, err
	}
	return eventFromValues(row.EventID, row.ResourceKind, row.ResourceID, row.EventType, row.DataJson, row.CreatedAt), nil
}

func (r *Repository) ListEvents(ctx context.Context, resourceKind, resourceID string, after int64, limit int) ([]asyncjob.Event, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("event limit must be between 1 and 200")
	}
	rows, err := r.q.ListAPIAsyncEvents(ctx, platformdb.ListAPIAsyncEventsParams{ResourceKind: resourceKind, ResourceID: resourceID, EventID: after, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	events := make([]asyncjob.Event, 0, len(rows))
	for _, row := range rows {
		event := eventFromValues(row.EventID, row.ResourceKind, row.ResourceID, row.EventType, row.DataJson, row.CreatedAt)
		events = append(events, event)
	}
	return events, nil
}

func (r *Repository) event(ctx context.Context, kind, id string, eventID int64) (asyncjob.Event, error) {
	row, err := r.q.GetAPIAsyncEvent(ctx, platformdb.GetAPIAsyncEventParams{ResourceKind: kind, ResourceID: id, EventID: eventID})
	return eventFromValues(row.EventID, row.ResourceKind, row.ResourceID, row.EventType, row.DataJson, row.CreatedAt), err
}

func jobFromGetRow(row platformdb.GetAPIAsyncJobRow) asyncjob.Job {
	return asyncjob.Job{ID: row.ID, Kind: row.JobKind, ResourceKind: row.ResourceKind, ResourceID: row.ResourceID,
		Payload: []byte(row.PayloadJson), Status: asyncjob.Status(row.Status), Attempts: int(row.AttemptCount), LeaseOwner: row.LeaseOwner,
		LeaseExpiresAt: row.LeaseExpiresAt, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, ErrorJSON: row.ErrorJson}
}

func jobFromClaimRow(row platformdb.ClaimAPIAsyncJobRow) asyncjob.Job {
	return asyncjob.Job{ID: row.ID, Kind: row.JobKind, ResourceKind: row.ResourceKind, ResourceID: row.ResourceID,
		Payload: []byte(row.PayloadJson), Status: asyncjob.Status(row.Status), Attempts: int(row.AttemptCount), LeaseOwner: row.LeaseOwner,
		LeaseExpiresAt: row.LeaseExpiresAt, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, ErrorJSON: row.ErrorJson}
}

func eventFromValues(eventID int64, kind, id, eventType, data, createdAt string) asyncjob.Event {
	return asyncjob.Event{ID: eventID, ResourceKind: kind, ResourceID: id, EventType: eventType, Data: []byte(data), CreatedAt: createdAt}
}

func requireChanged(changed int64, err error) error {
	if err != nil {
		return err
	}
	if changed != 1 {
		return asyncjob.ErrConflict
	}
	return nil
}
