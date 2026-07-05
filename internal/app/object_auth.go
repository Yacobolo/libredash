package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/go-chi/chi/v5"
)

func authObjectsForRequest(privilege access.Privilege, r *http.Request, workspaceID string) []access.ObjectRef {
	if privilege == access.PrivilegeManagePlatform {
		return []access.ObjectRef{access.PlatformObject()}
	}
	objects := routeObjectRefs(r, workspaceID)
	if len(objects) == 0 {
		objects = append(objects, authObjectForWorkspace(workspaceID))
	}
	return objects
}

func routeCanDeferDashboardDataAuth(privilege access.Privilege, r *http.Request) bool {
	return privilege == access.PrivilegeQueryData && strings.TrimSpace(chi.URLParam(r, "dashboard")) != ""
}

func routeObjectRefs(r *http.Request, workspaceID string) []access.ObjectRef {
	workspaceID = strings.TrimSpace(workspaceID)
	objects := []access.ObjectRef{}
	if dashboardID := strings.TrimSpace(chi.URLParam(r, "dashboard")); dashboardID != "" {
		objects = append(objects, access.ItemObject(access.SecurableDashboard, workspaceID, dashboardID))
	}
	modelID := strings.TrimSpace(chi.URLParam(r, "model"))
	if modelID != "" {
		model := access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID)
		if datasetID := strings.TrimSpace(chi.URLParam(r, "dataset")); datasetID != "" {
			objects = append(objects, access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+datasetID, model))
		}
		objects = append(objects, model)
	}
	if deploymentID := strings.TrimSpace(chi.URLParam(r, "deployment")); deploymentID != "" {
		objects = append(objects, access.ItemObject(access.SecurableTable, workspaceID, "deployment/"+deploymentID))
	}
	if conversationID := strings.TrimSpace(chi.URLParam(r, "conversation")); conversationID != "" {
		objects = append(objects, access.ItemObject(access.SecurableAgentPolicy, workspaceID, "conversation/"+conversationID))
	}
	if workspaceID != "" {
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	return objects
}

func dashboardObjectFromRequest(r *http.Request) access.ObjectRef {
	return access.ItemObject(access.SecurableDashboard, chi.URLParam(r, "workspace"), chi.URLParam(r, "dashboard"))
}

func semanticModelObjectFromRequest(r *http.Request) access.ObjectRef {
	return access.ItemObject(access.SecurableSemanticModel, chi.URLParam(r, "workspace"), chi.URLParam(r, "model"))
}

func semanticDatasetObjectFromRequest(r *http.Request) access.ObjectRef {
	model := semanticModelObjectFromRequest(r)
	return access.ItemObjectWithParent(access.SecurableDataset, chi.URLParam(r, "workspace"), chi.URLParam(r, "model")+"/"+chi.URLParam(r, "dataset"), model)
}

func objectWithInferredParent(typ access.SecurableType, workspaceID, objectID string) access.ObjectRef {
	parts := strings.Split(objectID, "/")
	switch typ {
	case access.SecurableDataset, access.SecurableTable:
		if len(parts) >= 2 && strings.TrimSpace(parts[0]) != "" {
			return access.ItemObjectWithParent(typ, workspaceID, objectID, access.ItemObject(access.SecurableSemanticModel, workspaceID, parts[0]))
		}
	case access.SecurableColumn:
		if len(parts) >= 3 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			parent := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, parts[0]+"/"+parts[1], access.ItemObject(access.SecurableSemanticModel, workspaceID, parts[0]))
			return access.ItemObjectWithParent(typ, workspaceID, objectID, parent)
		}
	}
	return access.ItemObject(typ, workspaceID, objectID)
}

func dashboardQueryObjects(metrics QueryMetrics, r *http.Request) []access.ObjectRef {
	workspaceID := chi.URLParam(r, "workspace")
	dashboardID := chi.URLParam(r, "dashboard")
	objects := []access.ObjectRef{access.ItemObject(access.SecurableDashboard, workspaceID, dashboardID)}
	if modelID := strings.TrimSpace(metrics.ModelIDForDashboard(dashboardID)); modelID != "" {
		objects = append(objects, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID))
	}
	objects = append(objects, access.WorkspaceObject(workspaceID))
	return objects
}

func (s *Server) authorizeCurrentObject(w http.ResponseWriter, r *http.Request, privilege access.Privilege, object access.ObjectRef) bool {
	return s.authorizeCurrentAny(w, r, privilege, []access.ObjectRef{object})
}

func (s *Server) authorizeCurrentAny(w http.ResponseWriter, r *http.Request, privilege access.Privilege, objects []access.ObjectRef) bool {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		writeJSONError(w, fmt.Errorf("authenticated principal is required"), http.StatusUnauthorized)
		return false
	}
	if credential, ok := currentAPICredential(s, r); ok && !apiTokenAllows(credential.Token, firstObjectWorkspace(objects), privilege) {
		writeJSONError(w, errForbidden, http.StatusForbidden)
		return false
	}
	if principal.DevBypass || s.auth == nil {
		return true
	}
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return false
	}
	decision, err := repo.AuthorizeAny(r.Context(), principal.ID, privilege, objects)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return false
	}
	if !decision.Allowed {
		writeJSONError(w, errForbidden, http.StatusForbidden)
		return false
	}
	return true
}

func firstObjectWorkspace(objects []access.ObjectRef) string {
	for _, object := range objects {
		if strings.TrimSpace(object.WorkspaceID) != "" {
			return object.WorkspaceID
		}
	}
	return ""
}
