package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
	"github.com/Yacobolo/leapview/internal/workspace"
)

type Repository struct {
	root *sql.DB
	db   sqlExecutor
	q    *platformdb.Queries
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const (
	defaultAPITokenTTL               = 90 * 24 * time.Hour
	defaultServicePrincipalSecretTTL = 180 * 24 * time.Hour
)

var secretRandomReader io.Reader = rand.Reader

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{root: sqlDB, db: sqlDB, q: platformdb.New(sqlDB)}
}

// InsertPlatformSettingIfMissing participates in the repository's current
// transaction and is used for one-shot instance initialization markers.
func (r *Repository) InsertPlatformSettingIfMissing(ctx context.Context, key, value string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO platform_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`, key, value)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *Repository) RunAuditedMutation(ctx context.Context, mutation func(access.Repository) (access.AuditEventInput, error)) error {
	return r.RunAuditedMutationBatch(ctx, func(repo access.Repository) ([]access.AuditEventInput, error) {
		input, err := mutation(repo)
		return []access.AuditEventInput{input}, err
	})
}

func (r *Repository) RunAuditedMutationBatch(ctx context.Context, mutation func(access.Repository) ([]access.AuditEventInput, error)) error {
	if r == nil || r.root == nil {
		return fmt.Errorf("access repository database is required")
	}
	if mutation == nil {
		return fmt.Errorf("audited mutation is required")
	}
	tx, err := r.root.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: begin: %v", access.ErrAuditTransaction, err)
	}
	defer func() { _ = tx.Rollback() }()

	txRepo := &Repository{root: r.root, db: tx, q: r.q.WithTx(tx)}
	inputs, err := mutation(txRepo)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("audited mutation requires at least one audit event")
	}
	for _, input := range inputs {
		if err := txRepo.RecordAuditEvent(ctx, input); err != nil {
			return fmt.Errorf("%w: record event: %v", access.ErrAuditTransaction, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit: %v", access.ErrAuditTransaction, err)
	}
	return nil
}

func (r *Repository) roleBindingParts(ctx context.Context, input access.RoleBindingInput) (platformdb.Role, sql.NullString, sql.NullString, error) {
	if strings.TrimSpace(input.Role) == "" {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, err
	}
	subjectID := strings.TrimSpace(input.SubjectID)
	if subjectID == "" {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("subject id is required")
	}
	switch input.SubjectType {
	case access.SubjectPrincipal:
		return role, sql.NullString{String: subjectID, Valid: true}, sql.NullString{}, nil
	case access.SubjectServicePrincipal:
		principal, err := r.q.GetPrincipal(ctx, subjectID)
		if err != nil {
			return platformdb.Role{}, sql.NullString{}, sql.NullString{}, err
		}
		if access.PrincipalKind(principal.Kind) != access.PrincipalKindServicePrincipal {
			return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("principal %q is not a service principal", subjectID)
		}
		return role, sql.NullString{String: subjectID, Valid: true}, sql.NullString{}, nil
	case access.SubjectGroup:
		return role, sql.NullString{}, sql.NullString{String: subjectID, Valid: true}, nil
	default:
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("unsupported subject type %q", input.SubjectType)
	}
}

func mapPrincipal(row platformdb.Principal) access.Principal {
	return access.Principal{
		ID:          row.ID,
		Kind:        access.PrincipalKind(row.Kind),
		Email:       row.Email,
		DisplayName: row.DisplayName,
		DisabledAt:  nullString(row.DisabledAt),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func mapGroup(row platformdb.Group) access.Group {
	return access.Group{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Provider:    row.Provider,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		CreatedAt:   row.CreatedAt,
	}
}

func mapRoleBinding(row platformdb.GetRoleBindingByIDRow) access.RoleBinding {
	return access.RoleBinding{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: access.SubjectType(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: nullString(row.PrincipalID),
		GroupID:     nullString(row.GroupID),
		Email:       nullString(row.Email),
		DisplayName: nullString(row.DisplayName),
		GroupName:   nullString(row.GroupName),
		Role:        row.RoleName,
		CreatedAt:   row.CreatedAt,
	}
}

func mapListedRoleBinding(row platformdb.ListRoleBindingsByWorkspaceRow) access.RoleBinding {
	return access.RoleBinding{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: access.SubjectType(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: nullString(row.PrincipalID),
		GroupID:     nullString(row.GroupID),
		Email:       nullString(row.Email),
		DisplayName: nullString(row.DisplayName),
		GroupName:   nullString(row.GroupName),
		Role:        row.RoleName,
		CreatedAt:   row.CreatedAt,
	}
}

func mapSession(row platformdb.Session) access.Session {
	return access.Session{
		ID:          row.ID,
		PrincipalID: row.PrincipalID,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
		LastSeenAt:  row.LastSeenAt,
		RevokedAt:   nullString(row.RevokedAt),
	}
}

func mapAPIToken(row platformdb.ApiToken) access.APIToken {
	var privileges []access.Privilege
	_ = json.Unmarshal([]byte(row.PrivilegesJson), &privileges)
	return access.APIToken{
		ID:          row.ID,
		PrincipalID: row.PrincipalID,
		WorkspaceID: nullString(row.WorkspaceID),
		Name:        row.Name,
		Privileges:  privileges,
		ExpiresAt:   nullString(row.ExpiresAt),
		CreatedAt:   row.CreatedAt,
		LastUsedAt:  nullString(row.LastUsedAt),
		RevokedAt:   nullString(row.RevokedAt),
	}
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stableAccessID(prefix, workspaceID, name string) string {
	return "cac_" + prefix + "_" + stableID(workspaceID+"|"+name)
}

func sortedWorkspaceGroupNames(values map[string]workspace.WorkspaceGroup) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceRoleBindingNames(values map[string]workspace.WorkspaceRoleBinding) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceGrantNames(values map[string]workspace.WorkspaceGrant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceDataPolicyNames(values map[string]workspace.WorkspaceDataPolicy) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newID(prefix string) (string, error) {
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	return prefix + "_" + secret[:24], nil
}

func newSecret() (string, error) {
	var b [32]byte
	if _, err := io.ReadFull(secretRandomReader, b[:]); err != nil {
		return "", fmt.Errorf("read secure random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func newTemporaryPassword() (string, error) {
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	return secret[:24], nil
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}
