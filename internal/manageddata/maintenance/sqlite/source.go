// Package sqlite implements managed-data reachability over the platform SQLite database.
package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/maintenance"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
)

const reachabilityQuery = `
SELECT source_type, source_id, source_status, revision_digest, manifest_json, file_count, size_bytes
FROM (
  SELECT
    'revision' AS source_type,
    id AS source_id,
    status AS source_status,
    digest AS revision_digest,
    manifest_json,
    file_count,
    size_bytes
  FROM managed_data_revisions
  WHERE status = 'ready'
  UNION ALL
  SELECT
    'upload' AS source_type,
    id AS source_id,
    status AS source_status,
    '' AS revision_digest,
    manifest_json,
    expected_file_count AS file_count,
    expected_size_bytes AS size_bytes
  FROM managed_data_upload_sessions
  WHERE status IN ('open', 'committing')
)
ORDER BY source_type, source_id`

const transactionCleanupTimeout = 5 * time.Second

type Source struct {
	db *sql.DB
}

func New(db *sql.DB) (*Source, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: SQLite database is required", maintenance.ErrInvalidMaintenance)
	}
	return &Source{db: db}, nil
}

func (s *Source) Snapshot(ctx context.Context) (maintenance.ReachabilitySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return maintenance.ReachabilitySnapshot{}, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return maintenance.ReachabilitySnapshot{}, sourceError(ctx, "acquire SQLite connection", err)
	}
	defer conn.Close()
	return readSnapshot(ctx, conn)
}

func (s *Source) WithStableSnapshot(
	ctx context.Context,
	expectedGeneration uint64,
	use func(maintenance.ReachabilitySnapshot) error,
) (returnErr error) {
	if use == nil {
		return fmt.Errorf("%w: stable snapshot callback is required", maintenance.ErrInvalidMaintenance)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return sourceError(ctx, "acquire SQLite connection", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return sourceError(ctx, "begin stable SQLite snapshot", err)
	}
	transactionActive := true
	defer func() {
		if !transactionActive {
			return
		}
		if err := rollback(conn); err != nil && returnErr == nil {
			returnErr = sourceError(context.Background(), "rollback stable SQLite snapshot", err)
		}
	}()

	snapshot, err := readSnapshot(ctx, conn)
	if err != nil {
		return err
	}
	if snapshot.Generation != expectedGeneration {
		return maintenance.ErrReachabilityChanged
	}
	if err := use(snapshot); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return sourceError(ctx, "commit stable SQLite snapshot", err)
	}
	transactionActive = false
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type durableManifest struct {
	sourceType     string
	id             string
	status         string
	revisionDigest string
	manifestJSON   string
	fileCount      int64
	sizeBytes      int64
}

func readSnapshot(ctx context.Context, query queryer) (maintenance.ReachabilitySnapshot, error) {
	rows, err := query.QueryContext(ctx, reachabilityQuery)
	if err != nil {
		return maintenance.ReachabilitySnapshot{}, sourceError(ctx, "query managed-data reachability", err)
	}
	defer rows.Close()

	digests := make(map[string]struct{})
	generation := sha256.New()
	for rows.Next() {
		var row durableManifest
		if err := rows.Scan(
			&row.sourceType,
			&row.id,
			&row.status,
			&row.revisionDigest,
			&row.manifestJSON,
			&row.fileCount,
			&row.sizeBytes,
		); err != nil {
			return maintenance.ReachabilitySnapshot{}, sourceError(ctx, "scan managed-data reachability", err)
		}
		manifest, canonical, err := validateDurableManifest(row)
		if err != nil {
			return maintenance.ReachabilitySnapshot{}, err
		}
		writeGenerationRecord(generation, row, canonical)
		for _, file := range manifest.Files {
			digests[file.SHA256] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return maintenance.ReachabilitySnapshot{}, sourceError(ctx, "iterate managed-data reachability", err)
	}

	sha256s := make([]string, 0, len(digests))
	for digest := range digests {
		sha256s = append(sha256s, digest)
	}
	sort.Strings(sha256s)
	sum := generation.Sum(nil)
	return maintenance.ReachabilitySnapshot{
		Generation: binary.BigEndian.Uint64(sum[:8]),
		SHA256s:    sha256s,
	}, nil
}

func validateDurableManifest(row durableManifest) (manageddata.Manifest, []byte, error) {
	if row.id == "" || row.fileCount < 0 || row.sizeBytes < 0 {
		return manageddata.Manifest{}, nil, integrityError("invalid durable manifest metadata")
	}
	switch row.sourceType {
	case "revision":
		if row.status != string(manageddata.RevisionStatusReady) {
			return manageddata.Manifest{}, nil, integrityError("invalid ready revision status")
		}
	case "upload":
		if row.status != string(manageddata.UploadStatusOpen) && row.status != string(manageddata.UploadStatusCommitting) {
			return manageddata.Manifest{}, nil, integrityError("invalid nonterminal upload status")
		}
	default:
		return manageddata.Manifest{}, nil, integrityError("invalid durable manifest source")
	}

	decoder := json.NewDecoder(strings.NewReader(row.manifestJSON))
	decoder.DisallowUnknownFields()
	var manifest manageddata.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return manageddata.Manifest{}, nil, integrityError("invalid durable manifest JSON")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return manageddata.Manifest{}, nil, integrityError("invalid durable manifest JSON")
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil || !bytes.Equal(canonical, []byte(row.manifestJSON)) {
		return manageddata.Manifest{}, nil, integrityError("noncanonical durable manifest")
	}

	var totalSize int64
	for _, file := range manifest.Files {
		if storage.ValidateSHA256(file.SHA256) != nil {
			return manageddata.Manifest{}, nil, integrityError("invalid durable file digest")
		}
		totalSize += file.Size
	}
	if int64(len(manifest.Files)) != row.fileCount || totalSize != row.sizeBytes {
		return manageddata.Manifest{}, nil, integrityError("durable manifest totals do not match")
	}
	if row.sourceType == "revision" {
		if row.revisionDigest != manifest.RevisionID() {
			return manageddata.Manifest{}, nil, integrityError("durable revision digest does not match manifest")
		}
	} else if row.revisionDigest != "" {
		return manageddata.Manifest{}, nil, integrityError("upload has an unexpected revision digest")
	}
	return manifest, canonical, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func writeGenerationRecord(target hash.Hash, row durableManifest, canonical []byte) {
	writeFramed(target, row.sourceType)
	writeFramed(target, row.id)
	writeFramed(target, row.status)
	writeFramed(target, row.revisionDigest)
	writeFramed(target, string(canonical))
	var numeric [16]byte
	binary.BigEndian.PutUint64(numeric[:8], uint64(row.fileCount))
	binary.BigEndian.PutUint64(numeric[8:], uint64(row.sizeBytes))
	_, _ = target.Write(numeric[:])
}

func writeFramed(target hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = target.Write(length[:])
	_, _ = target.Write([]byte(value))
}

func rollback(conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), transactionCleanupTimeout)
	defer cancel()
	_, err := conn.ExecContext(ctx, "ROLLBACK")
	return err
}

func integrityError(operation string) error {
	return fmt.Errorf("%w: %s", storage.ErrIntegrity, operation)
}

func sourceError(ctx context.Context, operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %s", storage.ErrBackend, operation)
}

var _ maintenance.ReachabilitySource = (*Source)(nil)
