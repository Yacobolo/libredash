package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/workspace"
)

type Privilege string

const (
	PrivilegeUseWorkspace       Privilege = "USE_WORKSPACE"
	PrivilegeViewItem           Privilege = "VIEW_ITEM"
	PrivilegeEditItem           Privilege = "EDIT_ITEM"
	PrivilegeManageItem         Privilege = "MANAGE_ITEM"
	PrivilegeQueryData          Privilege = "QUERY_DATA"
	PrivilegePreviewData        Privilege = "PREVIEW_DATA"
	PrivilegeRefreshData        Privilege = "REFRESH_DATA"
	PrivilegeDeploy             Privilege = "DEPLOY"
	PrivilegeActivateDeployment Privilege = "ACTIVATE_DEPLOYMENT"
	PrivilegeUseAgent           Privilege = "USE_AGENT"
	PrivilegeViewAgent          Privilege = "VIEW_AGENT"
	PrivilegeManageGrants       Privilege = "MANAGE_GRANTS"
	PrivilegeViewAudit          Privilege = "VIEW_AUDIT"
	PrivilegeManageWorkspace    Privilege = "MANAGE_WORKSPACE"
	PrivilegeManagePlatform     Privilege = "MANAGE_PLATFORM"
)

const (
	RoleOwner         = "owner"
	RoleAdmin         = "admin"
	RoleDeployer      = "deployer"
	RoleContributor   = "contributor"
	RoleEditor        = "editor"
	RoleMember        = "member"
	RoleViewer        = "viewer"
	RolePlatformAdmin = "platform_admin"
)

var defaultRoles = []Role{
	{
		Name: RoleOwner,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeManageItem,
			PrivilegeQueryData,
			PrivilegePreviewData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeActivateDeployment,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
			PrivilegeManageGrants,
			PrivilegeViewAudit,
			PrivilegeManageWorkspace,
		},
	},
	{
		Name: RoleAdmin,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeManageItem,
			PrivilegeQueryData,
			PrivilegePreviewData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeActivateDeployment,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
			PrivilegeManageGrants,
			PrivilegeViewAudit,
			PrivilegeManageWorkspace,
		},
	},
	{
		Name: RoleDeployer,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeQueryData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeActivateDeployment,
		},
	},
	{
		Name: RoleContributor,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeQueryData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
		},
	},
	{
		Name: RoleEditor,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeQueryData,
			PrivilegeRefreshData,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
		},
	},
	{
		Name: RoleMember,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeManageItem,
			PrivilegeQueryData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
		},
	},
	{
		Name: RoleViewer,
		Permissions: []Privilege{
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeQueryData,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
		},
	},
	{
		Name: RolePlatformAdmin,
		Permissions: []Privilege{
			PrivilegeManagePlatform,
			PrivilegeUseWorkspace,
			PrivilegeViewItem,
			PrivilegeEditItem,
			PrivilegeManageItem,
			PrivilegeQueryData,
			PrivilegePreviewData,
			PrivilegeRefreshData,
			PrivilegeDeploy,
			PrivilegeActivateDeployment,
			PrivilegeUseAgent,
			PrivilegeViewAgent,
			PrivilegeManageGrants,
			PrivilegeViewAudit,
			PrivilegeManageWorkspace,
		},
	},
}

type Principal struct {
	ID          string
	Kind        PrincipalKind
	Email       string
	DisplayName string
	CreatedAt   string
	UpdatedAt   string
}

type Role struct {
	Name        string
	Permissions []Privilege
}

type PrincipalKind string

const (
	PrincipalKindUser             PrincipalKind = "user"
	PrincipalKindGroup            PrincipalKind = "group"
	PrincipalKindServicePrincipal PrincipalKind = "service_principal"
)

type SecurableType string

const (
	SecurablePlatform      SecurableType = "platform"
	SecurableWorkspace     SecurableType = "workspace"
	SecurableDashboard     SecurableType = "dashboard"
	SecurableSemanticModel SecurableType = "semantic_model"
	SecurableSource        SecurableType = "source"
	SecurableModelTable    SecurableType = "model_table"
	SecurableAgentPolicy   SecurableType = "agent_policy"
	SecurableDataset       SecurableType = "dataset"
	SecurableTable         SecurableType = "table"
	SecurableColumn        SecurableType = "column"
)

type ObjectRef struct {
	Type        SecurableType
	WorkspaceID string
	ObjectID    string
	ParentID    string
}

func PlatformObject() ObjectRef {
	return ObjectRef{Type: SecurablePlatform}
}

func WorkspaceObject(workspaceID string) ObjectRef {
	return ObjectRef{Type: SecurableWorkspace, WorkspaceID: strings.TrimSpace(workspaceID)}
}

func ItemObject(typ SecurableType, workspaceID, objectID string) ObjectRef {
	return ObjectRef{Type: typ, WorkspaceID: strings.TrimSpace(workspaceID), ObjectID: strings.TrimSpace(objectID)}
}

func ItemObjectWithParent(typ SecurableType, workspaceID, objectID string, parent ObjectRef) ObjectRef {
	object := ItemObject(typ, workspaceID, objectID)
	object.ParentID = parent.CanonicalID()
	return object
}

func (r ObjectRef) CanonicalID() string {
	switch r.Type {
	case SecurablePlatform:
		return "platform"
	case SecurableWorkspace:
		return "workspace:" + r.WorkspaceID
	default:
		return string(r.Type) + ":" + r.WorkspaceID + ":" + r.ObjectID
	}
}

func (r ObjectRef) Parent() (ObjectRef, bool) {
	switch r.Type {
	case SecurablePlatform:
		return ObjectRef{}, false
	case SecurableWorkspace:
		return PlatformObject(), true
	default:
		return WorkspaceObject(r.WorkspaceID), true
	}
}

type SecurableObject struct {
	ID               string
	Type             SecurableType
	WorkspaceID      string
	ParentID         string
	OwnerPrincipalID string
	DisplayName      string
	CreatedAt        string
	UpdatedAt        string
}

type Grant struct {
	ID          string
	ObjectID    string
	ObjectType  SecurableType
	WorkspaceID string
	SubjectType SubjectType
	SubjectID   string
	Privilege   Privilege
	CreatedAt   string
}

type GrantView struct {
	Grant
	Inherited    bool
	ParentID     string
	ParentType   SecurableType
	ParentObject string
}

type GrantInput struct {
	Object      ObjectRef
	SubjectType SubjectType
	SubjectID   string
	Privilege   Privilege
}

type AuthorizationDecision struct {
	Allowed       bool
	Privilege     Privilege
	Object        ObjectRef
	Reason        string
	GrantID       string
	GrantObjectID string
	SubjectType   SubjectType
	SubjectID     string
	Inherited     bool
	Owner         bool
	Platform      bool
}

type SubjectType string

const (
	SubjectPrincipal        SubjectType = "principal"
	SubjectGroup            SubjectType = "group"
	SubjectServicePrincipal SubjectType = "service_principal"
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
	Kind        PrincipalKind
	Email       string
	DisplayName string
}

type ServicePrincipalInput struct {
	ID          string
	DisplayName string
}

type ServicePrincipalSecretInput struct {
	Name      string
	ExpiresAt time.Time
}

type ServicePrincipalSecret struct {
	ID                 string
	ServicePrincipalID string
	Name               string
	Secret             string
	ExpiresAt          string
	CreatedAt          string
	RevokedAt          string
}

type DataPolicy struct {
	ID             string
	WorkspaceID    string
	ObjectID       string
	PolicyType     string
	ExpressionJSON string
	CreatedAt      string
	UpdatedAt      string
}

type DataPolicyInput struct {
	ID             string
	Object         ObjectRef
	PolicyType     string
	ExpressionJSON string
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
	Permissions []Privilege
	ExpiresAt   time.Time
}

type APIToken struct {
	ID          string
	PrincipalID string
	WorkspaceID string
	Name        string
	Permissions []Privilege
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
	WorkspaceID   string
	PrincipalID   string
	Action        string
	TargetType    string
	TargetID      string
	Privilege     Privilege
	Status        string
	RequestID     string
	CorrelationID string
	MetadataJSON  string
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
	ID            string
	WorkspaceID   string
	PrincipalID   string
	Action        string
	TargetType    string
	TargetID      string
	Privilege     Privilege
	Status        string
	RequestID     string
	CorrelationID string
	MetadataJSON  string
	CreatedAt     string
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
	Authorize(ctx context.Context, principalID string, privilege Privilege, object ObjectRef) (AuthorizationDecision, error)
	AuthorizeAny(ctx context.Context, principalID string, privilege Privilege, objects []ObjectRef) (AuthorizationDecision, error)
	EffectivePrivileges(ctx context.Context, principalID string, object ObjectRef) ([]Privilege, error)
	EffectiveAccess(ctx context.Context, principalID string, object ObjectRef) ([]AuthorizationDecision, error)
	CreateGrant(ctx context.Context, input GrantInput) (Grant, error)
	DeleteGrant(ctx context.Context, workspaceID, id string) error
	ListGrants(ctx context.Context, object ObjectRef) ([]Grant, error)
	ListGrantsWithOptions(ctx context.Context, object ObjectRef, includeInherited bool) ([]GrantView, error)
	CreateServicePrincipal(ctx context.Context, input ServicePrincipalInput) (Principal, error)
	ListServicePrincipals(ctx context.Context) ([]Principal, error)
	UpdateServicePrincipal(ctx context.Context, id string, input ServicePrincipalInput) (Principal, error)
	DeleteServicePrincipal(ctx context.Context, id string) error
	CreateServicePrincipalSecret(ctx context.Context, servicePrincipalID, name string) (string, ServicePrincipalSecret, error)
	RevokeServicePrincipalSecret(ctx context.Context, servicePrincipalID, secretID string) error
	PrincipalForServicePrincipalSecret(ctx context.Context, servicePrincipalID, secret string) (Principal, error)
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

type WorkspacePolicyReconciler interface {
	ReconcileWorkspacePolicy(ctx context.Context, workspaceID string, policy workspace.AccessPolicy) error
}

func DefaultRoles() []Role {
	roles := make([]Role, len(defaultRoles))
	for i, role := range defaultRoles {
		roles[i] = Role{
			Name:        role.Name,
			Permissions: append([]Privilege(nil), role.Permissions...),
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
