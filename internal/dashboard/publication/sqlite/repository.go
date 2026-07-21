package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type scanner interface{ Scan(...any) error }

func scanPublication(row scanner) (publication.Publication, error) {
	var out publication.Publication
	var origins, dependencies string
	var configured int
	var servingStateID, suspendedAt, configuredAt, disabledAt, rotatedAt sql.NullString
	err := row.Scan(
		&out.ID, &out.ProjectID, &out.WorkspaceID, &out.Name, &out.PublicID,
		&out.Dashboard, &out.DefaultPage, &out.ConfigurationDigest, &origins, &dependencies, &configured,
		&servingStateID, &suspendedAt, &out.SuspendedBy, &configuredAt, &disabledAt, &rotatedAt,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return publication.Publication{}, publication.ErrNotFound
	}
	if err != nil {
		return publication.Publication{}, err
	}
	if err := json.Unmarshal([]byte(origins), &out.AllowedOrigins); err != nil {
		return publication.Publication{}, fmt.Errorf("decode publication origins: %w", err)
	}
	if err := json.Unmarshal([]byte(dependencies), &out.DependencyAssetIDs); err != nil {
		return publication.Publication{}, fmt.Errorf("decode publication dependencies: %w", err)
	}
	out.Configured = configured == 1
	out.ServingStateID = servingStateID.String
	out.SuspendedAt = suspendedAt.String
	out.ConfiguredAt = configuredAt.String
	out.DisabledAt = disabledAt.String
	out.RotatedAt = rotatedAt.String
	return out, nil
}

const publicationColumns = `id, project_id, workspace_id, name, public_id, dashboard, default_page,
configuration_digest, allowed_origins_json, dependency_asset_ids_json, configured, active_serving_state_id, suspended_at,
suspended_by, configured_at, disabled_at, rotated_at, created_at, updated_at`

func (r *Repository) Get(ctx context.Context, workspaceID, name string) (publication.Publication, error) {
	return scanPublication(r.db.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM dashboard_publications
WHERE workspace_id = ? AND name = ? ORDER BY configured DESC, updated_at DESC LIMIT 1`, strings.TrimSpace(workspaceID), strings.TrimSpace(name)))
}

func (r *Repository) GetByPublicID(ctx context.Context, publicID string) (publication.Publication, error) {
	return scanPublication(r.db.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM dashboard_publications
WHERE public_id = ? AND configured = 1 AND active_serving_state_id IS NOT NULL`, strings.TrimSpace(publicID)))
}

func (r *Repository) List(ctx context.Context, workspaceID string) ([]publication.Publication, error) {
	return r.list(ctx, `SELECT `+publicationColumns+` FROM dashboard_publications WHERE workspace_id = ? ORDER BY name, project_id`, strings.TrimSpace(workspaceID))
}

func (r *Repository) ListAll(ctx context.Context) ([]publication.Publication, error) {
	return r.list(ctx, `SELECT `+publicationColumns+` FROM dashboard_publications ORDER BY workspace_id, name, project_id`)
}

func (r *Repository) ListEvents(ctx context.Context, publicationID string) ([]publication.Event, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT event_type, actor_id, COALESCE(serving_state_id, ''), created_at FROM dashboard_publication_events WHERE publication_id = ? ORDER BY id DESC`, strings.TrimSpace(publicationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []publication.Event{}
	for rows.Next() {
		var event publication.Event
		if err := rows.Scan(&event.Type, &event.ActorID, &event.ServingStateID, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *Repository) list(ctx context.Context, query string, args ...any) ([]publication.Publication, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []publication.Publication{}
	for rows.Next() {
		row, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) Suspend(ctx context.Context, workspaceID, name, actorID string) (publication.Publication, error) {
	return r.mutate(ctx, workspaceID, name, actorID, "suspended", `
UPDATE dashboard_publications SET suspended_at = COALESCE(suspended_at, CURRENT_TIMESTAMP), suspended_by = ?, updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = ? AND name = ? AND configured = 1`, false)
}

func (r *Repository) Resume(ctx context.Context, workspaceID, name, actorID string) (publication.Publication, error) {
	return r.mutate(ctx, workspaceID, name, actorID, "resumed", `
UPDATE dashboard_publications SET suspended_at = NULL, suspended_by = '', updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = ? AND name = ? AND configured = 1`, true)
}

func (r *Repository) Rotate(ctx context.Context, workspaceID, name, actorID string) (publication.Publication, error) {
	publicID, err := newPublicID()
	if err != nil {
		return publication.Publication{}, err
	}
	return r.mutateWithArgs(ctx, workspaceID, name, actorID, "rotated", `
UPDATE dashboard_publications SET public_id = ?, rotated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = ? AND name = ? AND configured = 1`, []any{publicID, workspaceID, name})
}

func (r *Repository) mutate(ctx context.Context, workspaceID, name, actorID, eventType, statement string, resume bool) (publication.Publication, error) {
	args := []any{strings.TrimSpace(actorID), strings.TrimSpace(workspaceID), strings.TrimSpace(name)}
	if resume {
		args = []any{strings.TrimSpace(workspaceID), strings.TrimSpace(name)}
	}
	return r.mutateWithArgs(ctx, workspaceID, name, actorID, eventType, statement, args)
}

func (r *Repository) mutateWithArgs(ctx context.Context, workspaceID, name, actorID, eventType, statement string, args []any) (publication.Publication, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return publication.Publication{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return publication.Publication{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return publication.Publication{}, err
	}
	if changed == 0 {
		var configured int
		err := tx.QueryRowContext(ctx, `SELECT configured FROM dashboard_publications WHERE workspace_id = ? AND name = ? ORDER BY configured DESC LIMIT 1`, strings.TrimSpace(workspaceID), strings.TrimSpace(name)).Scan(&configured)
		if errors.Is(err, sql.ErrNoRows) {
			return publication.Publication{}, publication.ErrNotFound
		}
		if err != nil {
			return publication.Publication{}, err
		}
		return publication.Publication{}, publication.ErrConflict
	}
	row, err := scanPublication(tx.QueryRowContext(ctx, `SELECT `+publicationColumns+` FROM dashboard_publications WHERE workspace_id = ? AND name = ? AND configured = 1`, workspaceID, name))
	if err != nil {
		return publication.Publication{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_publication_events (publication_id, event_type, actor_id, serving_state_id) VALUES (?, ?, ?, NULLIF(?, ''))`, row.ID, eventType, strings.TrimSpace(actorID), row.ServingStateID); err != nil {
		return publication.Publication{}, err
	}
	if err := tx.Commit(); err != nil {
		return publication.Publication{}, err
	}
	return r.Get(ctx, workspaceID, name)
}

func ReconcileTx(ctx context.Context, tx *sql.Tx, input publication.ReconcileInput) error {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.ServingStateID = strings.TrimSpace(input.ServingStateID)
	if input.ProjectID == "" || input.WorkspaceID == "" || input.ServingStateID == "" {
		return fmt.Errorf("publication reconciliation requires project, workspace, and serving state")
	}
	if err := disableSupersededProjectPublications(ctx, tx, input); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, name, configured, configuration_digest FROM dashboard_publications WHERE project_id = ? AND workspace_id = ?`, input.ProjectID, input.WorkspaceID)
	if err != nil {
		return err
	}
	type existingRow struct {
		id, name, digest string
		configured       bool
	}
	existing := map[string]existingRow{}
	for rows.Next() {
		var row existingRow
		var configured int
		if err := rows.Scan(&row.id, &row.name, &configured, &row.digest); err != nil {
			rows.Close()
			return err
		}
		row.configured = configured == 1
		existing[row.name] = row
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for name, row := range existing {
		if _, ok := input.Publications[name]; ok || !row.configured {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE dashboard_publications SET configured = 0, active_serving_state_id = NULL, disabled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, row.id); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, row.id, "disabled", input.ActorID, input.ServingStateID); err != nil {
			return err
		}
	}

	names := make([]string, 0, len(input.Publications))
	for name := range input.Publications {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		compiled := input.Publications[name]
		principalID := access.DashboardPublicationSubjectID(input.WorkspaceID, name)
		if _, err := tx.ExecContext(ctx, `INSERT INTO principals (id, display_name, kind) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET display_name = excluded.display_name, kind = excluded.kind, updated_at = CURRENT_TIMESTAMP`, principalID, name, access.PrincipalKindDashboardPublication); err != nil {
			return fmt.Errorf("reconcile publication principal %q: %w", name, err)
		}
		origins, err := json.Marshal(compiled.AllowedOrigins)
		if err != nil {
			return err
		}
		dependencies, err := json.Marshal(compiled.DependencyAssetIDs)
		if err != nil {
			return err
		}
		if current, ok := existing[name]; ok {
			eventType := ""
			if !current.configured {
				eventType = "configured"
			} else if current.digest != compiled.ConfigurationDigest {
				eventType = "configuration_changed"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE dashboard_publications SET dashboard = ?, default_page = ?, configuration_digest = ?, allowed_origins_json = ?, dependency_asset_ids_json = ?, configured = 1, active_serving_state_id = ?, configured_at = CASE WHEN configured = 0 THEN CURRENT_TIMESTAMP ELSE configured_at END, disabled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, compiled.Dashboard, compiled.DefaultPage, compiled.ConfigurationDigest, string(origins), string(dependencies), input.ServingStateID, current.id); err != nil {
				return err
			}
			if eventType != "" {
				if err := insertEvent(ctx, tx, current.id, eventType, input.ActorID, input.ServingStateID); err != nil {
					return err
				}
			}
			continue
		}
		publicID, err := newPublicID()
		if err != nil {
			return err
		}
		id := operationalID(input.ProjectID, input.WorkspaceID, name)
		if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_publications (id, project_id, workspace_id, name, public_id, dashboard, default_page, configuration_digest, allowed_origins_json, dependency_asset_ids_json, configured, active_serving_state_id, configured_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, CURRENT_TIMESTAMP)`, id, input.ProjectID, input.WorkspaceID, name, publicID, compiled.Dashboard, compiled.DefaultPage, compiled.ConfigurationDigest, string(origins), string(dependencies), input.ServingStateID); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, id, "configured", input.ActorID, input.ServingStateID); err != nil {
			return err
		}
	}
	return nil
}

func disableSupersededProjectPublications(ctx context.Context, tx *sql.Tx, input publication.ReconcileInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM dashboard_publications WHERE workspace_id = ? AND project_id <> ? AND configured = 1`, input.WorkspaceID, input.ProjectID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE dashboard_publications SET configured = 0, active_serving_state_id = NULL, disabled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, id, "disabled", input.ActorID, input.ServingStateID); err != nil {
			return err
		}
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, id, eventType, actorID, servingStateID string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO dashboard_publication_events (publication_id, event_type, actor_id, serving_state_id) VALUES (?, ?, ?, NULLIF(?, ''))`, id, eventType, strings.TrimSpace(actorID), servingStateID)
	return err
}

func newPublicID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func operationalID(projectID, workspaceID, name string) string {
	sum := sha256.Sum256([]byte(projectID + "\x00" + workspaceID + "\x00" + name))
	return "pub_" + hex.EncodeToString(sum[:16])
}
