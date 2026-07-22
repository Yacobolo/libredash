package httpauth

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
)

type ObjectResolver func(r *http.Request, workspaceID string) []access.ObjectRef

func ObjectsForRequest(privilege access.Privilege, r *http.Request, workspaceID string) []access.ObjectRef {
	if privilege == access.PrivilegeManagePlatform {
		return []access.ObjectRef{access.PlatformObject()}
	}
	return []access.ObjectRef{ObjectForWorkspace(workspaceID)}
}

func CanDeferDataAuth(privilege access.Privilege) bool {
	return privilege == access.PrivilegeQueryData || privilege == access.PrivilegePreviewData
}

func RouteCanDeferGrantManagement(privilege access.Privilege, r *http.Request) bool {
	return privilege == access.PrivilegeManageGrants && (strings.Contains(r.URL.Path, "/grants") || strings.Contains(r.URL.Path, "/data-policies"))
}

func ObjectForWorkspace(workspaceID string) access.ObjectRef {
	if strings.TrimSpace(workspaceID) == "" {
		return access.PlatformObject()
	}
	return access.WorkspaceObject(workspaceID)
}
