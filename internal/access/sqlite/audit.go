package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
)

func (r *Repository) RecordAuditEvent(ctx context.Context, input access.AuditEventInput) error {
	if strings.TrimSpace(input.Action) == "" {
		return fmt.Errorf("audit action is required")
	}
	if strings.TrimSpace(input.MetadataJSON) == "" {
		input.MetadataJSON = "{}"
	}
	id, err := newID("audit")
	if err != nil {
		return err
	}
	return r.q.InsertAuditEvent(ctx, platformdb.InsertAuditEventParams{
		ID: id, WorkspaceID: sql.NullString{String: input.WorkspaceID, Valid: strings.TrimSpace(input.WorkspaceID) != ""},
		PrincipalID: sql.NullString{String: input.PrincipalID, Valid: strings.TrimSpace(input.PrincipalID) != ""},
		Action:      input.Action, TargetType: input.TargetType, TargetID: input.TargetID, Privilege: string(input.Privilege),
		Status: input.Status, RequestID: input.RequestID, CorrelationID: input.CorrelationID, MetadataJson: input.MetadataJSON,
	})
}

func (r *Repository) ListAuditEvents(ctx context.Context, filter access.AuditEventFilter) ([]access.AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if filter.PageToken != "" && filter.CursorTime == "" && filter.CursorID == "" {
		filter.CursorTime, filter.CursorID = decodeAuditPageToken(filter.PageToken)
	}
	from, to, cursorTime := sqliteAuditTime(filter.From), sqliteAuditTime(filter.To), sqliteAuditTime(filter.CursorTime)
	rows, err := r.q.ListAuditEvents(ctx, platformdb.ListAuditEventsParams{
		Column1: filter.WorkspaceID, WorkspaceID: sql.NullString{String: filter.WorkspaceID, Valid: strings.TrimSpace(filter.WorkspaceID) != ""},
		Column3: filter.PrincipalID, PrincipalID: sql.NullString{String: filter.PrincipalID, Valid: strings.TrimSpace(filter.PrincipalID) != ""},
		Column5: filter.Action, Action: filter.Action, Column7: filter.TargetType, TargetType: filter.TargetType,
		Column9: filter.TargetID, TargetID: filter.TargetID, Column11: from, CreatedAt: from, Column13: to, CreatedAt_2: to,
		Column15: cursorTime, CreatedAt_3: cursorTime, CreatedAt_4: cursorTime, ID: filter.CursorID, Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	events := make([]access.AuditEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, access.AuditEvent{
			ID: row.ID, WorkspaceID: nullString(row.WorkspaceID), PrincipalID: nullString(row.PrincipalID), Action: row.Action,
			TargetType: row.TargetType, TargetID: row.TargetID, Privilege: access.Privilege(row.Privilege), Status: row.Status,
			RequestID: row.RequestID, CorrelationID: row.CorrelationID, MetadataJSON: row.MetadataJson, CreatedAt: row.CreatedAt,
		})
	}
	return events, nil
}

func auditPageToken(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func sqliteAuditTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return value
}

func decodeAuditPageToken(token string) (string, string) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", ""
	}
	createdAt, id, ok := strings.Cut(string(raw), "\x00")
	if !ok {
		return "", ""
	}
	return createdAt, id
}
