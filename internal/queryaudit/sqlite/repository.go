package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/queryaudit"
)

type Repository struct {
	db *sql.DB
	q  *db.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: db.New(sqlDB)}
}

func (r *Repository) RecordQueryEvent(ctx context.Context, input queryaudit.EventInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(input.QueryJSON) == "" {
		input.QueryJSON = "{}"
	}
	return r.q.InsertQueryEvent(ctx, db.InsertQueryEventParams{
		ID:             newID("queryevent"),
		WorkspaceID:    input.WorkspaceID,
		PrincipalID:    input.PrincipalID,
		Surface:        input.Surface,
		Operation:      input.Operation,
		QueryKind:      input.QueryKind,
		ModelID:        input.ModelID,
		Target:         input.Target,
		ObjectType:     input.ObjectType,
		ObjectID:       input.ObjectID,
		RequestID:      input.RequestID,
		CorrelationID:  input.CorrelationID,
		Status:         input.Status,
		DurationMs:     input.DurationMS,
		QueueWaitMs:    input.QueueWaitMS,
		ExecutionMs:    input.ExecutionMS,
		ExecutionState: input.ExecutionState,
		RowsReturned:   int64(input.RowsReturned),
		BytesEstimate:  input.BytesEstimate,
		Error:          input.Error,
		SqlText:        input.SQL,
		PlanText:       input.PlanText,
		QueryJson:      input.QueryJSON,
	})
}

func (r *Repository) GetQueryEvent(ctx context.Context, id string) (queryaudit.Event, error) {
	row, err := r.q.GetQueryEvent(ctx, strings.TrimSpace(id))
	if err != nil {
		return queryaudit.Event{}, err
	}
	return queryEventFromDB(row), nil
}

func (r *Repository) ListQueryEvents(ctx context.Context, filter queryaudit.Filter) ([]queryaudit.Event, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if filter.PageToken != "" && filter.CursorTime == "" && filter.CursorID == "" {
		filter.CursorTime, filter.CursorID = decodePageToken(filter.PageToken)
	}
	query, args := listQueryEventsSQL(filter, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []queryaudit.Event{}
	for rows.Next() {
		var row db.QueryEvent
		if err := rows.Scan(
			&row.ID,
			&row.WorkspaceID,
			&row.PrincipalID,
			&row.Surface,
			&row.Operation,
			&row.QueryKind,
			&row.ModelID,
			&row.Target,
			&row.ObjectType,
			&row.ObjectID,
			&row.RequestID,
			&row.CorrelationID,
			&row.Status,
			&row.DurationMs,
			&row.QueueWaitMs,
			&row.ExecutionMs,
			&row.ExecutionState,
			&row.RowsReturned,
			&row.BytesEstimate,
			&row.Error,
			&row.SqlText,
			&row.PlanText,
			&row.QueryJson,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, queryEventFromDB(row))
	}
	return events, rows.Err()
}

func (r *Repository) ListQueryEventFilterOptions(ctx context.Context, field, search string, limit int) ([]queryaudit.FilterOption, error) {
	column, ok := queryEventFilterOptionColumn(strings.TrimSpace(field))
	if !ok {
		return nil, fmt.Errorf("unsupported query event filter option field %q", field)
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	var where []string
	var args []any
	where = append(where, column+" <> ''")
	if search = strings.TrimSpace(search); search != "" {
		where = append(where, column+" LIKE '%' || ? || '%'")
		args = append(args, search)
	}
	query := fmt.Sprintf("SELECT %s, COUNT(*) FROM query_events WHERE %s GROUP BY %s ORDER BY COUNT(*) DESC, %s ASC LIMIT ?", column, strings.Join(where, " AND "), column, column)
	args = append(args, int64(limit))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []queryaudit.FilterOption{}
	for rows.Next() {
		var option queryaudit.FilterOption
		if err := rows.Scan(&option.Value, &option.Count); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func queryEventFilterOptionColumn(field string) (string, bool) {
	switch field {
	case "workspace":
		return "workspace_id", true
	case "principal":
		return "principal_id", true
	case "surface":
		return "surface", true
	case "kind":
		return "query_kind", true
	case "status":
		return "status", true
	default:
		return "", false
	}
}

func listQueryEventsSQL(filter queryaudit.Filter, limit int) (string, []any) {
	var where []string
	var args []any
	addExact := func(column string, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		where = append(where, column+" = ?")
		args = append(args, strings.TrimSpace(value))
	}
	addIn := func(column string, values []string) {
		values = cleanValues(values)
		if len(values) == 0 {
			return
		}
		placeholders := make([]string, len(values))
		for i, value := range values {
			placeholders[i] = "?"
			args = append(args, value)
		}
		where = append(where, fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")))
	}
	addInOrExact := func(column, value string, values []string) {
		if len(cleanValues(values)) > 0 {
			addIn(column, values)
			return
		}
		addExact(column, value)
	}
	addInOrExact("workspace_id", filter.WorkspaceID, filter.WorkspaceIDs)
	addInOrExact("principal_id", filter.PrincipalID, filter.PrincipalIDs)
	addInOrExact("surface", filter.Surface, filter.Surfaces)
	addExact("operation", filter.Operation)
	addInOrExact("query_kind", filter.QueryKind, filter.QueryKinds)
	addExact("model_id", filter.ModelID)
	addExact("target", filter.Target)
	addInOrExact("status", filter.Status, filter.Statuses)
	if from := sqliteTime(filter.From); from != "" {
		where = append(where, "created_at >= ?")
		args = append(args, from)
	}
	if to := sqliteTime(filter.To); to != "" {
		where = append(where, "created_at <= ?")
		args = append(args, to)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		where = append(where, "(target LIKE '%' || ? || '%' OR sql_text LIKE '%' || ? || '%' OR query_json LIKE '%' || ? || '%')")
		args = append(args, search, search, search)
	}
	if cursorTime := sqliteTime(filter.CursorTime); cursorTime != "" {
		where = append(where, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.CursorID)
	}
	query := "SELECT id, workspace_id, principal_id, surface, operation, query_kind, model_id, target, object_type, object_id, request_id, correlation_id, status, duration_ms, queue_wait_ms, execution_ms, execution_state, rows_returned, bytes_estimate, error, sql_text, plan_text, query_json, created_at FROM query_events"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, int64(limit))
	return query, args
}

func cleanValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func queryEventFromDB(row db.QueryEvent) queryaudit.Event {
	return queryaudit.Event{
		ID: row.ID,
		EventInput: queryaudit.EventInput{
			WorkspaceID:    row.WorkspaceID,
			PrincipalID:    row.PrincipalID,
			Surface:        row.Surface,
			Operation:      row.Operation,
			QueryKind:      row.QueryKind,
			ModelID:        row.ModelID,
			Target:         row.Target,
			ObjectType:     row.ObjectType,
			ObjectID:       row.ObjectID,
			RequestID:      row.RequestID,
			CorrelationID:  row.CorrelationID,
			Status:         row.Status,
			DurationMS:     row.DurationMs,
			QueueWaitMS:    row.QueueWaitMs,
			ExecutionMS:    row.ExecutionMs,
			ExecutionState: row.ExecutionState,
			RowsReturned:   int(row.RowsReturned),
			BytesEstimate:  row.BytesEstimate,
			Error:          row.Error,
			SQL:            row.SqlText,
			PlanText:       row.PlanText,
			QueryJSON:      row.QueryJson,
		},
		CreatedAt: row.CreatedAt,
	}
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

func decodePageToken(token string) (string, string) {
	bytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(string(bytes), "\x00", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func sqliteTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return value
}
