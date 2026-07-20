// Package sqlite persists public API idempotency records and execution leases.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
)

type Record struct {
	Digest       string
	Owner        string
	LeaseExpires time.Time
	Status       int
	Header       http.Header
	Body         []byte
}

type Store struct{ q *platformdb.Queries }

func NewStore(db platformdb.DBTX) *Store { return &Store{q: platformdb.New(db)} }

func (s *Store) Claim(ctx context.Context, scope, digest, owner string, lease, lifetime time.Duration) (Record, bool, error) {
	now := time.Now().UTC()
	if err := s.q.DeleteExpiredAPIIdempotencyRecord(ctx, platformdb.DeleteExpiredAPIIdempotencyRecordParams{Scope: scope, ExpiresAt: now.Format(time.RFC3339Nano)}); err != nil {
		return Record{}, false, err
	}
	rows, err := s.q.CreateAPIIdempotencyRecord(ctx, platformdb.CreateAPIIdempotencyRecordParams{Scope: scope, RequestDigest: digest, OwnerID: owner,
		LeaseExpiresAt: now.Add(lease).Format(time.RFC3339Nano), CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(lifetime).Format(time.RFC3339Nano)})
	if err != nil {
		return Record{}, false, err
	}
	execute := rows == 1
	if !execute {
		rows, err = s.q.ReclaimAPIIdempotencyRecord(ctx, platformdb.ReclaimAPIIdempotencyRecordParams{OwnerID: owner,
			NewLeaseExpiresAt: now.Add(lease).Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Scope: scope,
			RequestDigest: digest, Now: now.Format(time.RFC3339Nano)})
		if err != nil {
			return Record{}, false, err
		}
		execute = rows == 1
	}
	record, err := s.Load(ctx, scope)
	return record, execute, err
}

func (s *Store) Load(ctx context.Context, scope string) (Record, error) {
	row, err := s.q.GetAPIIdempotencyRecord(ctx, scope)
	if err != nil {
		return Record{}, err
	}
	parsedLease, _ := time.Parse(time.RFC3339Nano, row.LeaseExpiresAt)
	record := Record{Digest: row.RequestDigest, Owner: row.OwnerID, LeaseExpires: parsedLease}
	if row.State != "completed" {
		return record, nil
	}
	record.Status = int(row.ResponseStatus.Int64)
	record.Body = append([]byte(nil), row.ResponseBody...)
	record.Header = http.Header{}
	if row.ResponseHeadersJson.Valid && row.ResponseHeadersJson.String != "" {
		if err := json.Unmarshal([]byte(row.ResponseHeadersJson.String), &record.Header); err != nil {
			return Record{}, err
		}
	}
	return record, nil
}

func (s *Store) Renew(ctx context.Context, scope, digest, owner string, lease time.Duration) error {
	now := time.Now().UTC()
	changed, err := s.q.RenewAPIIdempotencyRecord(ctx, platformdb.RenewAPIIdempotencyRecordParams{LeaseExpiresAt: now.Add(lease).Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), Scope: scope, RequestDigest: digest, OwnerID: owner})
	return requireOne(changed, err, "renew")
}

func (s *Store) Complete(ctx context.Context, scope, digest, owner string, status int, header http.Header, body []byte) error {
	headersJSON, err := json.Marshal(header)
	if err != nil {
		return err
	}
	changed, err := s.q.CompleteAPIIdempotencyRecord(ctx, platformdb.CompleteAPIIdempotencyRecordParams{ResponseStatus: sql.NullInt64{Int64: int64(status), Valid: true}, ResponseHeadersJson: sql.NullString{String: string(headersJSON), Valid: true}, ResponseBody: body, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: scope, RequestDigest: digest, OwnerID: owner})
	return requireOne(changed, err, "complete")
}

func (s *Store) Abandon(ctx context.Context, scope, digest, owner string) error {
	changed, err := s.q.AbandonAPIIdempotencyRecord(ctx, platformdb.AbandonAPIIdempotencyRecordParams{Scope: scope, RequestDigest: digest, OwnerID: owner})
	return requireOne(changed, err, "abandon")
}

func requireOne(rows int64, err error, operation string) error {
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("idempotency %s changed %d records", operation, rows)
	}
	return nil
}
