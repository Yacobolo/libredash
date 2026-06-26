package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	PermissionWorkspaceRead      = "workspace:read"
	PermissionAssetRead          = "asset:read"
	PermissionDeploymentRead     = "deployment:read"
	PermissionDeploymentWrite    = "deployment:write"
	PermissionDeploymentActivate = "deployment:activate"
	PermissionRBACRead           = "rbac:read"
	PermissionRBACWrite          = "rbac:write"
	PermissionAgentUse           = "agent:use"
	PermissionAgentRead          = "agent:read"
	PermissionMaterializationRun = "materialization:run"
	PermissionAuditRead          = "audit:read"
	PermissionTokenManage        = "token:manage"

	PermissionDashboardView           = PermissionWorkspaceRead
	PermissionDeploymentCreate        = PermissionDeploymentWrite
	PermissionDeploymentRollback      = PermissionDeploymentActivate
	PermissionMaterializationsRefresh = PermissionMaterializationRun
	PermissionRBACManage              = PermissionRBACWrite
)

const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleDeployer = "deployer"
	RoleEditor   = "editor"
	RoleViewer   = "viewer"
)

var defaultRoles = []Role{
	{
		Name: RoleOwner,
		Permissions: []string{
			PermissionWorkspaceRead,
			PermissionAssetRead,
			PermissionDeploymentRead,
			PermissionDeploymentWrite,
			PermissionDeploymentActivate,
			PermissionRBACRead,
			PermissionRBACWrite,
			PermissionAgentUse,
			PermissionAgentRead,
			PermissionMaterializationRun,
			PermissionAuditRead,
			PermissionTokenManage,
		},
	},
	{
		Name: RoleAdmin,
		Permissions: []string{
			PermissionWorkspaceRead,
			PermissionAssetRead,
			PermissionDeploymentRead,
			PermissionDeploymentWrite,
			PermissionDeploymentActivate,
			PermissionRBACRead,
			PermissionRBACWrite,
			PermissionAgentUse,
			PermissionAgentRead,
			PermissionMaterializationRun,
			PermissionAuditRead,
			PermissionTokenManage,
		},
	},
	{
		Name: RoleDeployer,
		Permissions: []string{
			PermissionWorkspaceRead,
			PermissionAssetRead,
			PermissionDeploymentRead,
			PermissionDeploymentWrite,
			PermissionDeploymentActivate,
			PermissionMaterializationRun,
		},
	},
	{
		Name: RoleEditor,
		Permissions: []string{
			PermissionWorkspaceRead,
			PermissionAssetRead,
			PermissionAgentUse,
			PermissionAgentRead,
			PermissionMaterializationRun,
		},
	},
	{
		Name: RoleViewer,
		Permissions: []string{
			PermissionWorkspaceRead,
			PermissionAssetRead,
			PermissionAgentUse,
			PermissionAgentRead,
		},
	},
}

type Principal struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   string
	UpdatedAt   string
}

type Role struct {
	Name        string
	Permissions []string
}

type SubjectType string

const (
	SubjectPrincipal SubjectType = "principal"
	SubjectGroup     SubjectType = "group"
)

type RoleBinding struct {
	ID          string
	WorkspaceID string
	SubjectType SubjectType
	SubjectID   string
	PrincipalID string
	GroupID     string
	Email       string
	DisplayName string
	GroupName   string
	Role        string
	CreatedAt   string
}

type RoleBindingInput struct {
	ID          string
	WorkspaceID string
	SubjectType SubjectType
	SubjectID   string
	Role        string
}

type PrincipalRoleInput struct {
	WorkspaceID string
	Email       string
	DisplayName string
	Role        string
}

type PlatformRoleInput struct {
	PrincipalID string
	Email       string
	DisplayName string
	Role        string
}

type PrincipalInput struct {
	ID          string
	Email       string
	DisplayName string
}

type ExternalIdentityInput struct {
	Provider    string
	TenantID    string
	Subject     string
	Email       string
	DisplayName string
}

type Group struct {
	ID          string
	WorkspaceID string
	Provider    string
	ExternalID  string
	Name        string
	CreatedAt   string
}

type GroupInput struct {
	ID          string
	WorkspaceID string
	Provider    string
	ExternalID  string
	Name        string
}

type GroupMember struct {
	GroupID     string
	WorkspaceID string
	PrincipalID string
	Email       string
	DisplayName string
	CreatedAt   string
}

type APITokenInput struct {
	PrincipalID string
	WorkspaceID string
	Name        string
	Permissions []string
	ExpiresAt   time.Time
}

type APIToken struct {
	ID          string
	PrincipalID string
	WorkspaceID string
	Name        string
	Permissions []string
	ExpiresAt   string
	CreatedAt   string
	LastUsedAt  string
	RevokedAt   string
}

type APICredential struct {
	Principal Principal
	Token     APIToken
}

type Session struct {
	ID          string
	PrincipalID string
	ExpiresAt   string
	CreatedAt   string
	LastSeenAt  string
	RevokedAt   string
}

type AuditEventInput struct {
	WorkspaceID  string
	PrincipalID  string
	Action       string
	TargetType   string
	TargetID     string
	MetadataJSON string
}

type AuditEventFilter struct {
	WorkspaceID string
	PrincipalID string
	Action      string
	TargetType  string
	TargetID    string
	From        string
	To          string
	PageToken   string
	CursorTime  string
	CursorID    string
	Limit       int
}

type AuditEvent struct {
	ID           string
	WorkspaceID  string
	PrincipalID  string
	Action       string
	TargetType   string
	TargetID     string
	MetadataJSON string
	CreatedAt    string
}

type Repository interface {
	PrincipalByID(ctx context.Context, id string) (Principal, error)
	UpsertPrincipal(ctx context.Context, input PrincipalInput) (Principal, error)
	SetPrincipalRole(ctx context.Context, input PrincipalRoleInput) (Principal, error)
	SetPlatformRole(ctx context.Context, input PlatformRoleInput) (Principal, error)
	RemovePrincipalRoles(ctx context.Context, workspaceID, principalID string) error
	CreateRoleBinding(ctx context.Context, input RoleBindingInput) (RoleBinding, error)
	GetRoleBinding(ctx context.Context, workspaceID, id string) (RoleBinding, error)
	UpdateRoleBinding(ctx context.Context, workspaceID, id string, input RoleBindingInput) (RoleBinding, error)
	DeleteRoleBinding(ctx context.Context, workspaceID, id string) error
	ListRoleBindings(ctx context.Context, workspaceID string) ([]RoleBinding, error)
	ListRoles(ctx context.Context) ([]Role, error)
	HasPermission(ctx context.Context, workspaceID, principalID, permission string) (bool, error)
	BootstrapAdmin(ctx context.Context, workspaceID, email string) error
	ResolveExternalPrincipal(ctx context.Context, input ExternalIdentityInput) (Principal, error)
	UpsertGroup(ctx context.Context, input GroupInput) (Group, error)
	ListGroups(ctx context.Context, workspaceID string) ([]Group, error)
	DeleteGroup(ctx context.Context, workspaceID, groupID string) error
	AddGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error
	RemoveGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error
	ListGroupMembers(ctx context.Context, workspaceID, groupID string) ([]GroupMember, error)
	CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error)
	PrincipalForToken(ctx context.Context, token string) (Principal, error)
	DeleteSession(ctx context.Context, token string) error
	ListSessions(ctx context.Context, principalID string) ([]Session, error)
	RevokeSession(ctx context.Context, id string) error
	RevokeSessionForPrincipal(ctx context.Context, principalID, id string) error
	CreateAPIToken(ctx context.Context, principalID, name string) (string, error)
	CreateAPITokenWithMetadata(ctx context.Context, input APITokenInput) (string, APIToken, error)
	PrincipalForAPIToken(ctx context.Context, token string) (Principal, error)
	CredentialForAPIToken(ctx context.Context, token string) (APICredential, error)
	ListAPITokens(ctx context.Context, principalID string) ([]APIToken, error)
	RevokeAPIToken(ctx context.Context, id string) error
	RevokeAPITokenForPrincipal(ctx context.Context, principalID, id string) error
	RecordAuditEvent(ctx context.Context, input AuditEventInput) error
	ListAuditEvents(ctx context.Context, filter AuditEventFilter) ([]AuditEvent, error)
}

func DefaultRoles() []Role {
	roles := make([]Role, len(defaultRoles))
	for i, role := range defaultRoles {
		roles[i] = Role{
			Name:        role.Name,
			Permissions: append([]string(nil), role.Permissions...),
		}
	}
	return roles
}

func PrincipalIDForEmail(email string) string {
	return "email_" + stableID(NormalizeEmail(email))
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}
