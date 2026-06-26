package app

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/go-chi/chi/v5"
)

type pageResponse struct {
	NextCursor string `json:"nextCursor"`
}

func pagedResponse(items any) map[string]any {
	return pagedResponseWithCursor(items, "")
}

func pagedResponseWithCursor(items any, nextCursor string) map[string]any {
	return map[string]any{"items": items, "page": pageResponse{NextCursor: nextCursor}}
}

func writePagedJSON[T any](w http.ResponseWriter, r *http.Request, items []T) bool {
	page, nextCursor, ok := pageSliceForRequest(w, r, items)
	if !ok {
		return false
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(page, nextCursor))
	return true
}

func pageSliceForRequest[T any](w http.ResponseWriter, r *http.Request, items []T) ([]T, string, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	start, ok := apiCursorOffsetForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	nextCursor := ""
	if end < len(items) {
		nextCursor = encodeIndexCursor(end)
	}
	return append([]T(nil), items[start:end]...), nextCursor, true
}

func (s *Server) apiGetCurrentPrincipal(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, currentPrincipalDTO(principal))
}

func (s *Server) apiListCurrentPermissions(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	workspaceID := s.workspaceID(r.URL.Query().Get("workspace"))
	permissions := knownPermissions()
	allowed := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if principal.DevBypass {
			allowed = append(allowed, permission)
			continue
		}
		if credential, ok := currentAPICredential(s, r); ok && !apiTokenAllows(credential.Token, workspaceID, permission) {
			continue
		}
		ok, err := repo.HasPermission(r.Context(), workspaceID, principal.ID, permission)
		if err != nil {
			writeJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if ok {
			allowed = append(allowed, permission)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaceId": workspaceID, "permissions": allowed})
}

func (s *Server) apiListCurrentAPITokens(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	rows, err := repo.ListAPITokens(r.Context(), principal.ID)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiTokenDTO(row))
	}
	_ = writePagedJSON(w, r, out)
}

func (s *Server) apiCreateCurrentAPIToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	var input struct {
		Name        string   `json:"name"`
		WorkspaceID string   `json:"workspaceId"`
		Permissions []string `json:"permissions"`
		ExpiresAt   string   `json:"expiresAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	var expiresAt time.Time
	if strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, input.ExpiresAt)
		if err != nil {
			writeJSONError(w, err, http.StatusBadRequest)
			return
		}
		expiresAt = parsed
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	token, row, err := repo.CreateAPITokenWithMetadata(r.Context(), access.APITokenInput{
		PrincipalID: principal.ID,
		WorkspaceID: input.WorkspaceID,
		Name:        input.Name,
		Permissions: input.Permissions,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "apiToken": apiTokenDTO(row)})
}

func (s *Server) apiRevokeCurrentAPIToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := repo.RevokeAPITokenForPrincipal(r.Context(), principal.ID, chi.URLParam(r, "token")); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) apiListCurrentSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	rows, err := repo.ListSessions(r.Context(), principal.ID)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionDTO(row))
	}
	_ = writePagedJSON(w, r, out)
}

func (s *Server) apiRevokeCurrentSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := repo.RevokeSessionForPrincipal(r.Context(), principal.ID, chi.URLParam(r, "session")); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) apiListPrincipals(w http.ResponseWriter, r *http.Request) {
	if _, ok := apiLimitForRequest(w, r); !ok {
		return
	}
	if _, ok := apiCursorOffsetForRequest(w, r); !ok {
		return
	}
	rows, err := s.queryPrincipals(r)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	_ = writePagedJSON(w, r, rows)
}

func (s *Server) apiGetPrincipal(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	principal, err := repo.PrincipalByID(r.Context(), chi.URLParam(r, "principal"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, principalDTO(principal))
}

func (s *Server) apiUpdatePrincipal(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	existing, err := repo.PrincipalByID(r.Context(), chi.URLParam(r, "principal"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if strings.TrimSpace(input.DisplayName) != "" {
		existing.DisplayName = input.DisplayName
	}
	principal, err := repo.UpsertPrincipal(r.Context(), access.PrincipalInput{ID: existing.ID, Email: existing.Email, DisplayName: existing.DisplayName})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, principalDTO(principal))
}

func (s *Server) apiListGroups(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	rows, err := repo.ListGroups(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, groupDTO(row))
	}
	_ = writePagedJSON(w, r, out)
}

func (s *Server) apiCreateGroup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	name := firstNonEmpty(input.DisplayName, input.Name)
	group, err := repo.UpsertGroup(r.Context(), access.GroupInput{WorkspaceID: s.workspaceID(chi.URLParam(r, "workspace")), Provider: "local", ExternalID: input.Name, Name: name})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, groupDTO(group))
}

func (s *Server) apiGetGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := s.groupByID(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, groupDTO(group))
}

func (s *Server) apiUpdateGroup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	group, ok := s.groupByID(w, r)
	if !ok {
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	updated, err := repo.UpsertGroup(r.Context(), access.GroupInput{ID: group.ID, WorkspaceID: group.WorkspaceID, Provider: group.Provider, ExternalID: group.ExternalID, Name: firstNonEmpty(input.DisplayName, group.Name)})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, groupDTO(updated))
}

func (s *Server) apiDeleteGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := s.groupByID(w, r)
	if !ok {
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := repo.DeleteGroup(r.Context(), group.WorkspaceID, group.ID); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) apiListGroupMembers(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	rows, err := repo.ListGroupMembers(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")), chi.URLParam(r, "group"))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, groupMemberPrincipalDTO(row))
	}
	_ = writePagedJSON(w, r, out)
}

func (s *Server) apiAddGroupMember(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := repo.AddGroupMember(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")), chi.URLParam(r, "group"), chi.URLParam(r, "principal")); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *Server) apiRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if err := repo.RemoveGroupMember(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")), chi.URLParam(r, "group"), chi.URLParam(r, "principal")); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) apiCreateRoleBinding(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeRoleBindingInput(w, r)
	if !ok {
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	row, err := repo.CreateRoleBinding(r.Context(), input)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, apiRoleBindingDTO(row))
}

func (s *Server) apiUpdateRoleBinding(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeRoleBindingInput(w, r)
	if !ok {
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	row, err := repo.UpdateRoleBinding(r.Context(), input.WorkspaceID, chi.URLParam(r, "binding"), input)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, apiRoleBindingDTO(row))
}

func (s *Server) apiListAuditEvents(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return
	}
	cursorTime, cursorID := decodeCursor(r.URL.Query().Get("pageToken"))
	rows, err := repo.ListAuditEvents(r.Context(), access.AuditEventFilter{
		WorkspaceID: s.workspaceID(chi.URLParam(r, "workspace")),
		PrincipalID: r.URL.Query().Get("actor"),
		Action:      r.URL.Query().Get("action"),
		TargetType:  r.URL.Query().Get("targetType"),
		TargetID:    r.URL.Query().Get("targetId"),
		From:        r.URL.Query().Get("from"),
		To:          r.URL.Query().Get("to"),
		CursorTime:  cursorTime,
		CursorID:    cursorID,
		Limit:       limit + 1,
	})
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	nextCursor := ""
	if len(rows) > limit {
		last := rows[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, auditEventDTO(row))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func currentPrincipal(s *Server, r *http.Request) (Principal, bool) {
	if s.auth == nil {
		return Principal{ID: "dev", Email: "dev@localhost", DisplayName: "Local Developer", DevBypass: true}, true
	}
	return s.auth.Principal(r)
}

func currentAPICredential(s *Server, r *http.Request) (access.APICredential, bool) {
	if s.auth == nil {
		return access.APICredential{}, false
	}
	return s.auth.APICredential(r)
}

func principalDTO(row access.Principal) map[string]any {
	return map[string]any{"id": row.ID, "email": row.Email, "displayName": row.DisplayName, "createdAt": row.CreatedAt, "updatedAt": row.UpdatedAt}
}

func currentPrincipalDTO(row Principal) map[string]any {
	return map[string]any{"id": row.ID, "email": row.Email, "displayName": row.DisplayName, "createdAt": "", "updatedAt": ""}
}

func groupDTO(row access.Group) map[string]any {
	return map[string]any{"id": row.ID, "name": row.ExternalID, "displayName": row.Name, "createdAt": row.CreatedAt, "updatedAt": row.CreatedAt}
}

func groupMemberPrincipalDTO(row access.GroupMember) map[string]any {
	return map[string]any{"id": row.PrincipalID, "email": row.Email, "displayName": row.DisplayName, "createdAt": row.CreatedAt, "updatedAt": row.CreatedAt}
}

func apiRoleBindingDTO(row access.RoleBinding) map[string]any {
	return map[string]any{"id": row.ID, "workspaceId": row.WorkspaceID, "subjectType": string(row.SubjectType), "subjectId": row.SubjectID, "email": row.Email, "displayName": firstNonEmpty(row.DisplayName, row.GroupName), "role": row.Role, "createdAt": row.CreatedAt}
}

func apiTokenDTO(row access.APIToken) map[string]any {
	return map[string]any{"id": row.ID, "name": row.Name, "workspaceId": row.WorkspaceID, "permissions": row.Permissions, "expiresAt": emptyToNil(row.ExpiresAt), "revokedAt": emptyToNil(row.RevokedAt), "createdAt": row.CreatedAt, "lastUsedAt": emptyToNil(row.LastUsedAt)}
}

func sessionDTO(row access.Session) map[string]any {
	return map[string]any{"id": row.ID, "createdAt": row.CreatedAt, "expiresAt": row.ExpiresAt, "lastSeenAt": emptyToNil(row.LastSeenAt), "revokedAt": emptyToNil(row.RevokedAt)}
}

func auditEventDTO(row access.AuditEvent) map[string]any {
	var metadata map[string]any
	if strings.TrimSpace(row.MetadataJSON) != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON), &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return map[string]any{"id": row.ID, "workspaceId": row.WorkspaceID, "principalId": row.PrincipalID, "action": row.Action, "targetType": row.TargetType, "targetId": row.TargetID, "metadata": metadata, "createdAt": row.CreatedAt}
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func knownPermissions() []string {
	return []string{
		access.PermissionWorkspaceRead,
		access.PermissionAssetRead,
		access.PermissionDeploymentRead,
		access.PermissionDeploymentWrite,
		access.PermissionDeploymentActivate,
		access.PermissionRBACRead,
		access.PermissionRBACWrite,
		access.PermissionAgentUse,
		access.PermissionAgentRead,
		access.PermissionMaterializationRun,
		access.PermissionAuditRead,
		access.PermissionTokenManage,
	}
}

func (s *Server) queryPrincipals(r *http.Request) ([]map[string]any, error) {
	if s.store == nil {
		return []map[string]any{}, nil
	}
	query := `
SELECT id, email, display_name, created_at, updated_at
FROM principals
WHERE (? = '' OR lower(email) = lower(?))
  AND (? = '' OR lower(email) LIKE '%' || lower(?) || '%' OR lower(display_name) LIKE '%' || lower(?) || '%')
ORDER BY email, id
`
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.store.SQLDB().QueryContext(r.Context(), query, email, email, q, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, principalEmail, displayName, createdAt, updatedAt string
		if err := rows.Scan(&id, &principalEmail, &displayName, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "email": principalEmail, "displayName": displayName, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	return out, rows.Err()
}

func (s *Server) groupByID(w http.ResponseWriter, r *http.Request) (access.Group, bool) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return access.Group{}, false
	}
	rows, err := repo.ListGroups(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return access.Group{}, false
	}
	for _, row := range rows {
		if row.ID == chi.URLParam(r, "group") {
			return row, true
		}
	}
	writeJSONError(w, sql.ErrNoRows, http.StatusNotFound)
	return access.Group{}, false
}

func decodeRoleBindingInput(w http.ResponseWriter, r *http.Request) (access.RoleBindingInput, bool) {
	var input struct {
		SubjectType string `json:"subjectType"`
		SubjectID   string `json:"subjectId"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return access.RoleBindingInput{}, false
	}
	return access.RoleBindingInput{
		WorkspaceID: chi.URLParam(r, "workspace"),
		SubjectType: access.SubjectType(input.SubjectType),
		SubjectID:   input.SubjectID,
		Role:        input.Role,
	}, true
}

const (
	defaultAPILimit = 50
	maxAPILimit     = 100
)

func apiLimitForRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit, err := parseAPILimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return 0, false
	}
	return limit, true
}

func parseAPILimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAPILimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if value <= 0 {
		return defaultAPILimit, nil
	}
	if value > maxAPILimit {
		return maxAPILimit, nil
	}
	return value, nil
}

func apiCursorOffsetForRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	offset, err := decodeIndexCursor(r.URL.Query().Get("pageToken"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return 0, false
	}
	return offset, true
}

func encodeIndexCursor(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("idx:%d", offset)))
}

func decodeIndexCursor(token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("pageToken is invalid")
	}
	text := string(raw)
	if !strings.HasPrefix(text, "idx:") {
		return 0, fmt.Errorf("pageToken is invalid")
	}
	value, err := strconv.Atoi(strings.TrimPrefix(text, "idx:"))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("pageToken is invalid")
	}
	return value, nil
}

func encodeCursor(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func decodeCursor(token string) (string, string) {
	if strings.TrimSpace(token) == "" {
		return "", ""
	}
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
