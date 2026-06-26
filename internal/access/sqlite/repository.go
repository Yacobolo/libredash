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
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
)

type Repository struct {
	q *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{q: platformdb.New(sqlDB)}
}

func (r *Repository) PrincipalByID(ctx context.Context, id string) (access.Principal, error) {
	row, err := r.q.GetPrincipal(ctx, id)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) UpsertPrincipal(ctx context.Context, input access.PrincipalInput) (access.Principal, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("principal")
	}
	if err := r.q.UpsertPrincipal(ctx, platformdb.UpsertPrincipalParams{
		ID:          input.ID,
		Email:       input.Email,
		DisplayName: input.DisplayName,
	}); err != nil {
		return access.Principal{}, err
	}
	row, err := r.q.GetPrincipal(ctx, input.ID)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) SetPrincipalRole(ctx context.Context, input access.PrincipalRoleInput) (access.Principal, error) {
	email := access.NormalizeEmail(input.Email)
	if email == "" {
		return access.Principal{}, fmt.Errorf("email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          access.PrincipalIDForEmail(email),
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email),
	})
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: input.WorkspaceID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          newID("rolebinding"),
		WorkspaceID: input.WorkspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) SetPlatformRole(ctx context.Context, input access.PlatformRoleInput) (access.Principal, error) {
	principalID := strings.TrimSpace(input.PrincipalID)
	email := access.NormalizeEmail(input.Email)
	if principalID == "" && email == "" {
		return access.Principal{}, fmt.Errorf("principal id or email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	if principalID == "" {
		principalID = access.PrincipalIDForEmail(email)
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          principalID,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email, principalID),
	})
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertPlatformRoleBinding(ctx, platformdb.InsertPlatformRoleBindingParams{
		ID:          newID("platformrolebinding"),
		RoleID:      role.ID,
		PrincipalID: principal.ID,
	}); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) RemovePrincipalRoles(ctx context.Context, workspaceID, principalID string) error {
	if strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("principal id is required")
	}
	return r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: workspaceID,
		PrincipalID: sql.NullString{String: principalID, Valid: true},
	})
}

func (r *Repository) CreateRoleBinding(ctx context.Context, input access.RoleBindingInput) (access.RoleBinding, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("rolebinding")
	}
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return access.RoleBinding{}, fmt.Errorf("workspace id is required")
	}
	role, principalID, groupID, err := r.roleBindingParts(ctx, input)
	if err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          input.ID,
		WorkspaceID: input.WorkspaceID,
		RoleID:      role.ID,
		PrincipalID: principalID,
		GroupID:     groupID,
	}); err != nil {
		return access.RoleBinding{}, err
	}
	return r.GetRoleBinding(ctx, input.WorkspaceID, input.ID)
}

func (r *Repository) GetRoleBinding(ctx context.Context, workspaceID, id string) (access.RoleBinding, error) {
	row, err := r.q.GetRoleBindingByID(ctx, platformdb.GetRoleBindingByIDParams{
		WorkspaceID: workspaceID,
		ID:          id,
	})
	if err != nil {
		return access.RoleBinding{}, err
	}
	return mapRoleBinding(row), nil
}

func (r *Repository) UpdateRoleBinding(ctx context.Context, workspaceID, id string, input access.RoleBindingInput) (access.RoleBinding, error) {
	input.WorkspaceID = workspaceID
	role, principalID, groupID, err := r.roleBindingParts(ctx, input)
	if err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.q.UpdateRoleBindingByID(ctx, platformdb.UpdateRoleBindingByIDParams{
		RoleID:      role.ID,
		PrincipalID: principalID,
		GroupID:     groupID,
		WorkspaceID: workspaceID,
		ID:          id,
	}); err != nil {
		return access.RoleBinding{}, err
	}
	return r.GetRoleBinding(ctx, workspaceID, id)
}

func (r *Repository) DeleteRoleBinding(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("role binding id is required")
	}
	return r.q.DeleteRoleBindingByID(ctx, platformdb.DeleteRoleBindingByIDParams{
		WorkspaceID: workspaceID,
		ID:          id,
	})
}

func (r *Repository) ListRoleBindings(ctx context.Context, workspaceID string) ([]access.RoleBinding, error) {
	rows, err := r.q.ListRoleBindingsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	bindings := make([]access.RoleBinding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, mapListedRoleBinding(row))
	}
	return bindings, nil
}

func (r *Repository) ListRoles(ctx context.Context) ([]access.Role, error) {
	rows, err := r.q.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]access.Role, 0, len(rows))
	for _, row := range rows {
		var permissions []string
		_ = json.Unmarshal([]byte(row.PermissionsJson), &permissions)
		roles = append(roles, access.Role{Name: row.Name, Permissions: permissions})
	}
	return roles, nil
}

func (r *Repository) HasPermission(ctx context.Context, workspaceID, principalID, permission string) (bool, error) {
	if principalID == "" {
		return false, nil
	}
	rows, err := r.q.ListPrincipalRolePermissions(ctx, platformdb.ListPrincipalRolePermissionsParams{
		WorkspaceID:   workspaceID,
		PrincipalID:   sql.NullString{String: principalID, Valid: true},
		PrincipalID_2: principalID,
		PrincipalID_3: principalID,
	})
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row == permission {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) BootstrapAdmin(ctx context.Context, workspaceID, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          access.PrincipalIDForEmail(email),
		Email:       email,
		DisplayName: email,
	})
	if err != nil {
		return err
	}
	role, err := r.q.GetRoleByName(ctx, access.RoleOwner)
	if err != nil {
		return err
	}
	return r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          newID("rolebinding"),
		WorkspaceID: workspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: principal.ID != ""},
	})
}

func (r *Repository) ResolveExternalPrincipal(ctx context.Context, input access.ExternalIdentityInput) (access.Principal, error) {
	input.Email = access.NormalizeEmail(input.Email)
	if input.Provider == "" || input.Subject == "" {
		return access.Principal{}, fmt.Errorf("external identity requires provider and subject")
	}
	identity, err := r.q.GetExternalIdentity(ctx, platformdb.GetExternalIdentityParams{
		Provider: input.Provider,
		TenantID: input.TenantID,
		Subject:  input.Subject,
	})
	if err == nil {
		return r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          identity.PrincipalID,
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return access.Principal{}, err
	}

	var principal access.Principal
	if input.Email != "" {
		row, err := r.q.GetPrincipalByEmail(ctx, input.Email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return access.Principal{}, err
		}
		if err == nil {
			principal = mapPrincipal(row)
		}
	}
	if principal.ID == "" {
		principal, err = r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          "external_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject),
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
		if err != nil {
			return access.Principal{}, err
		}
	} else {
		principal, err = r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          principal.ID,
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
		if err != nil {
			return access.Principal{}, err
		}
	}

	if err := r.q.UpsertExternalIdentity(ctx, platformdb.UpsertExternalIdentityParams{
		ID:          "identity_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject),
		PrincipalID: principal.ID,
		Provider:    input.Provider,
		TenantID:    input.TenantID,
		Subject:     input.Subject,
		Email:       input.Email,
	}); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) UpsertGroup(ctx context.Context, input access.GroupInput) (access.Group, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return access.Group{}, fmt.Errorf("workspace id is required")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return access.Group{}, fmt.Errorf("group name is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("group")
	}
	if strings.TrimSpace(input.Provider) == "" && strings.TrimSpace(input.ExternalID) == "" {
		input.Provider = "local"
		input.ExternalID = input.ID
	}
	if err := r.q.UpsertGroup(ctx, platformdb.UpsertGroupParams{
		ID:          input.ID,
		WorkspaceID: input.WorkspaceID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
		Name:        input.Name,
	}); err != nil {
		return access.Group{}, err
	}
	row, err := r.q.GetGroupByProviderExternalID(ctx, platformdb.GetGroupByProviderExternalIDParams{
		WorkspaceID: input.WorkspaceID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
	})
	if err != nil {
		return access.Group{}, err
	}
	return mapGroup(row), nil
}

func (r *Repository) ListGroups(ctx context.Context, workspaceID string) ([]access.Group, error) {
	rows, err := r.q.ListGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	groups := make([]access.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, mapGroup(row))
	}
	return groups, nil
}

func (r *Repository) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	if strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("group id is required")
	}
	return r.q.DeleteGroup(ctx, platformdb.DeleteGroupParams{
		WorkspaceID: workspaceID,
		ID:          groupID,
	})
}

func (r *Repository) AddGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.InsertGroupMember(ctx, platformdb.InsertGroupMemberParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) RemoveGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.DeleteGroupMember(ctx, platformdb.DeleteGroupMemberParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) ListGroupMembers(ctx context.Context, workspaceID, groupID string) ([]access.GroupMember, error) {
	rows, err := r.q.ListGroupMembers(ctx, platformdb.ListGroupMembersParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
	})
	if err != nil {
		return nil, err
	}
	members := make([]access.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, access.GroupMember{
			GroupID:     row.GroupID,
			WorkspaceID: row.WorkspaceID,
			PrincipalID: row.PrincipalID,
			Email:       row.Email,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
		})
	}
	return members, nil
}

func (r *Repository) CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error) {
	token := newSecret()
	expires := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	return token, r.q.CreateSession(ctx, platformdb.CreateSessionParams{
		ID:          newID("session"),
		PrincipalID: principalID,
		TokenHash:   tokenHash(token),
		ExpiresAt:   expires,
	})
}

func (r *Repository) PrincipalForToken(ctx context.Context, token string) (access.Principal, error) {
	session, err := r.q.GetSessionByTokenHash(ctx, tokenHash(token))
	if err != nil {
		return access.Principal{}, err
	}
	_ = r.q.TouchSession(ctx, session.ID)
	row, err := r.q.GetPrincipal(ctx, session.PrincipalID)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	return r.q.DeleteSessionByTokenHash(ctx, tokenHash(token))
}

func (r *Repository) ListSessions(ctx context.Context, principalID string) ([]access.Session, error) {
	rows, err := r.q.ListSessionsByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	sessions := make([]access.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, mapSession(row))
	}
	return sessions, nil
}

func (r *Repository) RevokeSession(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("session id is required")
	}
	return r.q.RevokeSession(ctx, id)
}

func (r *Repository) RevokeSessionForPrincipal(ctx context.Context, principalID, id string) error {
	if strings.TrimSpace(principalID) == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("principal id and session id are required")
	}
	_, err := r.q.RevokeSessionForPrincipal(ctx, platformdb.RevokeSessionForPrincipalParams{
		PrincipalID: principalID,
		ID:          id,
	})
	return err
}

func (r *Repository) CreateAPIToken(ctx context.Context, principalID, name string) (string, error) {
	token, _, err := r.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
		PrincipalID: principalID,
		Name:        name,
	})
	return token, err
}

func (r *Repository) CreateAPITokenWithMetadata(ctx context.Context, input access.APITokenInput) (string, access.APIToken, error) {
	if strings.TrimSpace(input.PrincipalID) == "" {
		return "", access.APIToken{}, fmt.Errorf("principal id is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return "", access.APIToken{}, fmt.Errorf("token name is required")
	}
	token := newSecret()
	id := newID("token")
	permissionsJSON, err := json.Marshal(input.Permissions)
	if err != nil {
		return "", access.APIToken{}, err
	}
	expiresAt := sql.NullString{}
	if !input.ExpiresAt.IsZero() {
		expiresAt = sql.NullString{String: input.ExpiresAt.UTC().Format(time.RFC3339), Valid: true}
	}
	if err := r.q.CreateAPIToken(ctx, platformdb.CreateAPITokenParams{
		ID:              id,
		PrincipalID:     input.PrincipalID,
		WorkspaceID:     sql.NullString{String: input.WorkspaceID, Valid: strings.TrimSpace(input.WorkspaceID) != ""},
		Name:            input.Name,
		TokenHash:       tokenHash(token),
		PermissionsJson: string(permissionsJSON),
		ExpiresAt:       expiresAt,
	}); err != nil {
		return "", access.APIToken{}, err
	}
	tokens, err := r.q.ListAPITokensByPrincipal(ctx, input.PrincipalID)
	if err != nil {
		return "", access.APIToken{}, err
	}
	for _, row := range tokens {
		if row.ID == id {
			return token, mapAPIToken(row), nil
		}
	}
	return token, access.APIToken{ID: id, PrincipalID: input.PrincipalID, WorkspaceID: input.WorkspaceID, Name: input.Name, Permissions: input.Permissions, ExpiresAt: nullString(expiresAt)}, nil
}

func (r *Repository) PrincipalForAPIToken(ctx context.Context, token string) (access.Principal, error) {
	credential, err := r.CredentialForAPIToken(ctx, token)
	if err != nil {
		return access.Principal{}, err
	}
	return credential.Principal, nil
}

func (r *Repository) CredentialForAPIToken(ctx context.Context, token string) (access.APICredential, error) {
	apiToken, err := r.q.GetAPITokenByHash(ctx, tokenHash(token))
	if err != nil {
		return access.APICredential{}, err
	}
	_ = r.q.TouchAPIToken(ctx, apiToken.ID)
	row, err := r.q.GetPrincipal(ctx, apiToken.PrincipalID)
	if err != nil {
		return access.APICredential{}, err
	}
	return access.APICredential{
		Principal: mapPrincipal(row),
		Token:     mapAPIToken(apiToken),
	}, nil
}

func (r *Repository) ListAPITokens(ctx context.Context, principalID string) ([]access.APIToken, error) {
	rows, err := r.q.ListAPITokensByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	tokens := make([]access.APIToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, mapAPIToken(row))
	}
	return tokens, nil
}

func (r *Repository) RevokeAPIToken(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("api token id is required")
	}
	return r.q.RevokeAPIToken(ctx, id)
}

func (r *Repository) RevokeAPITokenForPrincipal(ctx context.Context, principalID, id string) error {
	if strings.TrimSpace(principalID) == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("principal id and api token id are required")
	}
	_, err := r.q.RevokeAPITokenForPrincipal(ctx, platformdb.RevokeAPITokenForPrincipalParams{
		PrincipalID: principalID,
		ID:          id,
	})
	return err
}

func (r *Repository) RecordAuditEvent(ctx context.Context, input access.AuditEventInput) error {
	if strings.TrimSpace(input.Action) == "" {
		return fmt.Errorf("audit action is required")
	}
	if strings.TrimSpace(input.MetadataJSON) == "" {
		input.MetadataJSON = "{}"
	}
	return r.q.InsertAuditEvent(ctx, platformdb.InsertAuditEventParams{
		ID:           newID("audit"),
		WorkspaceID:  sql.NullString{String: input.WorkspaceID, Valid: strings.TrimSpace(input.WorkspaceID) != ""},
		PrincipalID:  sql.NullString{String: input.PrincipalID, Valid: strings.TrimSpace(input.PrincipalID) != ""},
		Action:       input.Action,
		TargetType:   input.TargetType,
		TargetID:     input.TargetID,
		MetadataJson: input.MetadataJSON,
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
	from := sqliteAuditTime(filter.From)
	to := sqliteAuditTime(filter.To)
	cursorTime := sqliteAuditTime(filter.CursorTime)
	rows, err := r.q.ListAuditEvents(ctx, platformdb.ListAuditEventsParams{
		Column1:     filter.WorkspaceID,
		WorkspaceID: sql.NullString{String: filter.WorkspaceID, Valid: strings.TrimSpace(filter.WorkspaceID) != ""},
		Column3:     filter.PrincipalID,
		PrincipalID: sql.NullString{String: filter.PrincipalID, Valid: strings.TrimSpace(filter.PrincipalID) != ""},
		Column5:     filter.Action,
		Action:      filter.Action,
		Column7:     filter.TargetType,
		TargetType:  filter.TargetType,
		Column9:     filter.TargetID,
		TargetID:    filter.TargetID,
		Column11:    from,
		CreatedAt:   from,
		Column13:    to,
		CreatedAt_2: to,
		Column15:    cursorTime,
		CreatedAt_3: cursorTime,
		CreatedAt_4: cursorTime,
		ID:          filter.CursorID,
		Limit:       int64(limit),
	})
	if err != nil {
		return nil, err
	}
	events := make([]access.AuditEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, access.AuditEvent{
			ID:           row.ID,
			WorkspaceID:  nullString(row.WorkspaceID),
			PrincipalID:  nullString(row.PrincipalID),
			Action:       row.Action,
			TargetType:   row.TargetType,
			TargetID:     row.TargetID,
			MetadataJSON: row.MetadataJson,
			CreatedAt:    row.CreatedAt,
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
		parsed, err := time.Parse(layout, value)
		if err == nil {
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
	case access.SubjectGroup:
		return role, sql.NullString{}, sql.NullString{String: subjectID, Valid: true}, nil
	default:
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("unsupported subject type %q", input.SubjectType)
	}
}

func mapPrincipal(row platformdb.Principal) access.Principal {
	return access.Principal{
		ID:          row.ID,
		Email:       row.Email,
		DisplayName: row.DisplayName,
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
	var permissions []string
	_ = json.Unmarshal([]byte(row.PermissionsJson), &permissions)
	return access.APIToken{
		ID:          row.ID,
		PrincipalID: row.PrincipalID,
		WorkspaceID: nullString(row.WorkspaceID),
		Name:        row.Name,
		Permissions: permissions,
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

func newID(prefix string) string {
	return prefix + "_" + newSecret()[:24]
}

func newSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
