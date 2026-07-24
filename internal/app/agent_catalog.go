package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	agenttools "github.com/Yacobolo/leapview/internal/agent/tools"
	analyticsqueryhttp "github.com/Yacobolo/leapview/internal/analytics/query/http"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/cursorsigning"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardhttp "github.com/Yacobolo/leapview/internal/dashboard/http"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	"github.com/Yacobolo/leapview/internal/workspace"
)

const catalogListCursorPrefix = "cl1"

var catalogSearchTypes = []productsearch.Type{
	productsearch.TypeWorkspace,
	productsearch.TypeDashboard,
	productsearch.TypePage,
	productsearch.TypeVisual,
	productsearch.TypeFilter,
	productsearch.TypeSemanticModel,
	productsearch.TypeSemanticTable,
	productsearch.TypeField,
	productsearch.TypeMeasure,
}

type agentCatalogService struct {
	server *Server
}

type activeWorkspaceCatalogRepository interface {
	ListWithActiveMetadata(context.Context, string) ([]workspace.Summary, error)
	ByIDWithActiveMetadata(context.Context, workspace.WorkspaceID, string) (workspace.Summary, error)
}

func (c agentCatalogService) Search(ctx context.Context, scope agenttools.Scope, request agenttools.CatalogSearchRequest) (agenttools.CatalogPage, error) {
	if c.server == nil || c.server.search == nil {
		return agenttools.CatalogPage{}, errors.New("catalog search is not configured")
	}
	types := make([]productsearch.Type, 0, len(request.Types))
	for _, typ := range request.Types {
		types = append(types, productsearch.Type(typ))
	}
	query := productsearch.Query{
		Text:         request.Query,
		Environment:  c.server.defaultEnvironment,
		Workspaces:   append([]string(nil), request.WorkspaceIDs...),
		Types:        types,
		AllowedTypes: append([]productsearch.Type(nil), catalogSearchTypes...),
		Limit:        request.Limit,
		Cursor:       request.Cursor,
	}
	if request.Context != nil {
		query.Context.DashboardID = request.Context.DashboardID
		query.Context.PageID = request.Context.PageID
	}
	if len(request.WorkspaceIDs) == 1 {
		query.Context.WorkspaceID = request.WorkspaceIDs[0]
	} else if scope.WorkspaceID != "" {
		query.Context.WorkspaceID = scope.WorkspaceID
	}
	page, err := c.server.search.Search(ctx, catalogSearchSubject(scope), query)
	if err != nil {
		return agenttools.CatalogPage{}, catalogSearchError(err)
	}
	return agenttools.CatalogPage{Items: catalogItems(page.Items), NextCursor: page.NextCursor}, nil
}

func (c agentCatalogService) List(ctx context.Context, scope agenttools.Scope, request agenttools.CatalogListRequest) (agenttools.CatalogPage, error) {
	items, err := c.listItems(ctx, scope, request)
	if err != nil {
		return agenttools.CatalogPage{}, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left != right {
			return left < right
		}
		if items[i].Ref.Type != items[j].Ref.Type {
			return items[i].Ref.Type < items[j].Ref.Type
		}
		return items[i].Ref.ID < items[j].Ref.ID
	})
	snapshot := catalogItemsSnapshot(items)
	offset, err := decodeCatalogListCursor(request.Cursor, scope, request, snapshot)
	if err != nil {
		return agenttools.CatalogPage{}, err
	}
	if offset > len(items) {
		return agenttools.CatalogPage{}, &agenttools.CatalogError{Code: "invalid_arguments", Message: "catalog cursor offset is invalid"}
	}
	end := offset + request.Limit
	if end > len(items) {
		end = len(items)
	}
	page := agenttools.CatalogPage{Items: append([]agenttools.CatalogItem(nil), items[offset:end]...)}
	if end < len(items) {
		page.NextCursor = encodeCatalogListCursor(scope, request, snapshot, end)
	}
	return page, nil
}

func (c agentCatalogService) Get(ctx context.Context, scope agenttools.Scope, request agenttools.CatalogGetRequest) (agenttools.CatalogGetResult, error) {
	result, err := c.get(ctx, scope, request)
	status := "success"
	if err != nil {
		status = "error"
		var catalogErr *agenttools.CatalogError
		if errors.As(err, &catalogErr) && catalogErr.Code == "catalog_not_found" {
			status = "denied"
		}
	}
	if c.server != nil && !scope.DevAuthBypass {
		if repository, repositoryErr := c.server.accessRepository(); repositoryErr == nil && repository != nil {
			auditScope := agentScopeFromTools(scope)
			auditScope.WorkspaceID = request.Ref.WorkspaceID
			recordAgentToolAudit(ctx, repository, auditScope, access.PrivilegeViewItem, "agent_tool", agenttools.CatalogGetToolName, status, err)
		}
	}
	return result, err
}

func (c agentCatalogService) get(ctx context.Context, scope agenttools.Scope, request agenttools.CatalogGetRequest) (agenttools.CatalogGetResult, error) {
	if request.Ref.Type == agenttools.CatalogTypeWorkspace {
		item, summary, ok, err := c.workspaceItem(ctx, scope, request.Ref.WorkspaceID)
		if err != nil {
			return agenttools.CatalogGetResult{}, err
		}
		if !ok {
			return agenttools.CatalogGetResult{}, catalogNotFound()
		}
		return agenttools.CatalogGetResult{
			Item: item,
			Details: map[string]any{
				"type":                 string(agenttools.CatalogTypeWorkspace),
				"activeServingStateId": string(summary.ActiveServingStateID),
			},
		}, nil
	}
	result, ok, err := c.resolveOne(ctx, scope, request.Ref)
	if err != nil {
		return agenttools.CatalogGetResult{}, err
	}
	if !ok {
		return agenttools.CatalogGetResult{}, catalogNotFound()
	}
	item := catalogItem(result)
	location, err := catalogGetLocation(request, item.Locations)
	if err != nil {
		return agenttools.CatalogGetResult{}, err
	}
	details, err := c.details(ctx, scope, request.Ref, location)
	if err != nil {
		return agenttools.CatalogGetResult{}, err
	}
	return agenttools.CatalogGetResult{Item: item, Details: details}, nil
}

func (c agentCatalogService) listItems(ctx context.Context, scope agenttools.Scope, request agenttools.CatalogListRequest) ([]agenttools.CatalogItem, error) {
	if request.Parent == nil {
		return c.listWorkspaceItems(ctx, scope)
	}
	parent := *request.Parent
	if parent.Type == agenttools.CatalogTypeWorkspace {
		if _, _, ok, err := c.workspaceItem(ctx, scope, parent.WorkspaceID); err != nil {
			return nil, err
		} else if !ok {
			return nil, catalogNotFound()
		}
	} else if _, ok, err := c.resolveOne(ctx, scope, parent); err != nil {
		return nil, err
	} else if !ok {
		return nil, catalogNotFound()
	}
	references, err := c.childReferences(parent, request.ChildTypes)
	if err != nil {
		return nil, err
	}
	if len(references) == 0 {
		return []agenttools.CatalogItem{}, nil
	}
	if c.server.search == nil {
		return nil, errors.New("catalog search is not configured")
	}
	results, err := c.server.search.Resolve(ctx, catalogSearchSubject(scope), c.server.defaultEnvironment, references)
	if err != nil {
		return nil, err
	}
	return catalogItems(results), nil
}

func (c agentCatalogService) listWorkspaceItems(ctx context.Context, scope agenttools.Scope) ([]agenttools.CatalogItem, error) {
	repository, err := c.server.workspaceRepository()
	if err != nil {
		return nil, err
	}
	var summaries []workspace.Summary
	if repository != nil {
		if active, ok := repository.(activeWorkspaceCatalogRepository); ok {
			summaries, err = active.ListWithActiveMetadata(ctx, c.server.defaultEnvironment)
		} else {
			summaries, err = repository.List(ctx)
		}
		if err != nil {
			return nil, err
		}
	} else if c.server.metrics != nil {
		catalog := c.server.metrics.Catalog()
		summaries = []workspace.Summary{{
			ID: workspace.WorkspaceID(catalog.Workspace.ID), Title: catalog.Workspace.Title, Description: catalog.Workspace.Description,
		}}
	}
	items := make([]agenttools.CatalogItem, 0, len(summaries))
	for _, summary := range summaries {
		allowed, err := c.canViewWorkspace(ctx, scope, string(summary.ID))
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		items = append(items, catalogWorkspaceItem(summary))
	}
	return items, nil
}

func (c agentCatalogService) workspaceItem(ctx context.Context, scope agenttools.Scope, workspaceID string) (agenttools.CatalogItem, workspace.Summary, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	allowed, err := c.canViewWorkspace(ctx, scope, workspaceID)
	if err != nil || !allowed {
		return agenttools.CatalogItem{}, workspace.Summary{}, false, err
	}
	repository, err := c.server.workspaceRepository()
	if err != nil {
		return agenttools.CatalogItem{}, workspace.Summary{}, false, err
	}
	if repository != nil {
		var summary workspace.Summary
		if active, ok := repository.(activeWorkspaceCatalogRepository); ok {
			summary, err = active.ByIDWithActiveMetadata(ctx, workspace.WorkspaceID(workspaceID), c.server.defaultEnvironment)
		} else {
			summary, err = repository.ByID(ctx, workspace.WorkspaceID(workspaceID))
		}
		if err != nil {
			if errors.Is(err, workspace.ErrNotFound) {
				return agenttools.CatalogItem{}, workspace.Summary{}, false, nil
			}
			return agenttools.CatalogItem{}, workspace.Summary{}, false, err
		}
		return catalogWorkspaceItem(summary), summary, true, nil
	}
	metrics, ok := c.server.metricsForWorkspace(workspaceID)
	if !ok || metrics == nil {
		return agenttools.CatalogItem{}, workspace.Summary{}, false, nil
	}
	value := metrics.Catalog().Workspace
	summary := workspace.Summary{ID: workspace.WorkspaceID(value.ID), Title: value.Title, Description: value.Description}
	return catalogWorkspaceItem(summary), summary, true, nil
}

func (c agentCatalogService) canViewWorkspace(ctx context.Context, scope agenttools.Scope, workspaceID string) (bool, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(scope.PrincipalID) == "" {
		return false, nil
	}
	if scope.Credential.WorkspaceID != "" && scope.Credential.WorkspaceID != workspaceID {
		return false, nil
	}
	if scope.Credential.Restricted && !containsSearchPrivilege(scope.Credential.Privileges, access.PrivilegeViewItem) {
		return false, nil
	}
	if scope.DevAuthBypass || c.server.auth == nil {
		return true, nil
	}
	repository, err := c.server.accessRepository()
	if err != nil || repository == nil {
		return false, err
	}
	decision, err := repository.Authorize(ctx, scope.PrincipalID, access.PrivilegeViewItem, access.WorkspaceObject(workspaceID))
	return err == nil && decision.Allowed, err
}

func (c agentCatalogService) resolveOne(ctx context.Context, scope agenttools.Scope, ref agenttools.CatalogRef) (productsearch.Result, bool, error) {
	if c.server.search == nil {
		return productsearch.Result{}, false, errors.New("catalog search is not configured")
	}
	results, err := c.server.search.Resolve(ctx, catalogSearchSubject(scope), c.server.defaultEnvironment, []productsearch.Reference{catalogSearchReference(ref)})
	if err != nil {
		return productsearch.Result{}, false, err
	}
	if len(results) != 1 {
		return productsearch.Result{}, false, nil
	}
	return results[0], true, nil
}

func (c agentCatalogService) childReferences(parent agenttools.CatalogRef, requested []agenttools.CatalogType) ([]productsearch.Reference, error) {
	metrics, ok := c.server.metricsForWorkspace(parent.WorkspaceID)
	if !ok || metrics == nil {
		return []productsearch.Reference{}, nil
	}
	children := catalogRequestedChildren(parent.Type, requested)
	allows := func(typ agenttools.CatalogType) bool {
		for _, child := range children {
			if child == typ {
				return true
			}
		}
		return false
	}
	references := make([]productsearch.Reference, 0)
	switch parent.Type {
	case agenttools.CatalogTypeWorkspace:
		catalog := metrics.Catalog()
		if allows(agenttools.CatalogTypeDashboard) {
			for _, value := range catalog.Dashboards {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeDashboard, ID: value.ID})
			}
		}
		if allows(agenttools.CatalogTypeSemanticModel) {
			for _, value := range catalog.Models {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeSemanticModel, ID: value.ID})
			}
		}
	case agenttools.CatalogTypeDashboard:
		if allows(agenttools.CatalogTypePage) {
			for _, page := range metrics.Pages(parent.ID) {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypePage, ID: parent.ID + "." + page.ID})
			}
		}
	case agenttools.CatalogTypePage:
		dashboardID, pageID, ok := catalogPageIDs(parent.ID)
		if !ok {
			return nil, catalogNotFound()
		}
		report, _, ok := metrics.Report(dashboardID)
		if !ok {
			return nil, catalogNotFound()
		}
		page, ok := catalogPage(metrics.Pages(dashboardID), pageID)
		if !ok {
			return nil, catalogNotFound()
		}
		seen := map[productsearch.Reference]struct{}{}
		for _, component := range page.Visuals {
			if allows(agenttools.CatalogTypeVisual) {
				id := component.Visual
				if id != "" {
					reference := productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeVisual, ID: report.ID + "." + id}
					if _, duplicate := seen[reference]; !duplicate {
						seen[reference] = struct{}{}
						references = append(references, reference)
					}
				}
			}
			if allows(agenttools.CatalogTypeFilter) && component.Filter != "" {
				reference := productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeFilter, ID: report.ID + "." + component.Filter}
				if _, duplicate := seen[reference]; !duplicate {
					seen[reference] = struct{}{}
					references = append(references, reference)
				}
			}
		}
	case agenttools.CatalogTypeSemanticModel:
		model, ok := metrics.SemanticModel(parent.ID)
		if !ok || model == nil {
			return nil, catalogNotFound()
		}
		if allows(agenttools.CatalogTypeSemanticTable) {
			for _, id := range sortedCatalogKeys(model.Tables) {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeSemanticTable, ID: parent.ID + "." + id})
			}
		}
		if allows(agenttools.CatalogTypeMeasure) {
			for _, id := range append(sortedCatalogKeys(model.Measures), sortedCatalogKeys(model.Metrics)...) {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeMeasure, ID: parent.ID + "." + id})
			}
		}
	case agenttools.CatalogTypeSemanticTable:
		modelID, tableID, ok := catalogSemanticTableIDs(parent.ID)
		if !ok {
			return nil, catalogNotFound()
		}
		model, ok := metrics.SemanticModel(modelID)
		if !ok || model == nil {
			return nil, catalogNotFound()
		}
		table, ok := model.Tables[tableID]
		if !ok {
			return nil, catalogNotFound()
		}
		if allows(agenttools.CatalogTypeField) {
			for _, id := range sortedCatalogKeys(table.Dimensions) {
				references = append(references, productsearch.Reference{WorkspaceID: parent.WorkspaceID, Type: productsearch.TypeField, ID: modelID + "." + tableID + "." + id})
			}
		}
	default:
		return nil, &agenttools.CatalogError{Code: "invalid_arguments", Message: fmt.Sprintf("catalog type %q cannot have children", parent.Type)}
	}
	return references, nil
}

func (c agentCatalogService) details(ctx context.Context, scope agenttools.Scope, ref agenttools.CatalogRef, location agenttools.CatalogLocation) (map[string]any, error) {
	metrics, ok := c.server.metricsForWorkspace(ref.WorkspaceID)
	if !ok || metrics == nil {
		return nil, catalogNotFound()
	}
	switch ref.Type {
	case agenttools.CatalogTypeDashboard:
		report, model, ok := metrics.Report(ref.ID)
		if !ok {
			return nil, catalogNotFound()
		}
		manifest := dashboardhttp.DashboardManifestProjection(report, model, metrics.Pages(ref.ID))
		return map[string]any{
			"type":             string(ref.Type),
			"semanticModelRef": catalogRefValue(ref.WorkspaceID, agenttools.CatalogTypeSemanticModel, report.SemanticModel),
			"pageCount":        manifest.Counts.Pages,
			"visualCount":      manifest.Counts.Visuals,
			"filterCount":      manifest.Counts.Filters,
		}, nil
	case agenttools.CatalogTypePage:
		dashboardID, pageID, ok := catalogPageIDs(ref.ID)
		if !ok {
			return nil, catalogNotFound()
		}
		report, _, ok := metrics.Report(dashboardID)
		if !ok {
			return nil, catalogNotFound()
		}
		page, ok := catalogPage(metrics.Pages(dashboardID), pageID)
		if !ok {
			return nil, catalogNotFound()
		}
		projection := dashboardhttp.DashboardPageProjection(report, page)
		return map[string]any{"type": string(ref.Type), "components": projection.Components}, nil
	case agenttools.CatalogTypeVisual:
		return catalogVisualDetails(metrics, ref, location)
	case agenttools.CatalogTypeFilter:
		return catalogFilterDetails(metrics, ref, location)
	case agenttools.CatalogTypeSemanticModel:
		return c.semanticModelDetails(ctx, scope, metrics, ref)
	case agenttools.CatalogTypeSemanticTable:
		return catalogSemanticTableDetails(metrics, ref)
	case agenttools.CatalogTypeField:
		return catalogFieldDetails(metrics, ref)
	case agenttools.CatalogTypeMeasure:
		return catalogMeasureDetails(metrics, ref)
	default:
		return nil, catalogNotFound()
	}
}

func catalogSearchSubject(scope agenttools.Scope) productsearch.Subject {
	subject := productsearch.Subject{
		ID:                   scope.PrincipalID,
		DevBypass:            scope.DevAuthBypass,
		CredentialRestricted: scope.Credential.Restricted,
		Privileges:           append([]string(nil), scope.Credential.Privileges...),
	}
	if workspaceID := strings.TrimSpace(scope.Credential.WorkspaceID); workspaceID != "" {
		subject.Restricted = true
		subject.WorkspaceIDs = []string{workspaceID}
	}
	return subject
}

func catalogSearchReference(ref agenttools.CatalogRef) productsearch.Reference {
	return productsearch.Reference{WorkspaceID: ref.WorkspaceID, Type: productsearch.Type(ref.Type), ID: ref.ID}
}

func catalogItems(results []productsearch.Result) []agenttools.CatalogItem {
	items := make([]agenttools.CatalogItem, 0, len(results))
	for _, result := range results {
		items = append(items, catalogItem(result))
	}
	return items
}

func catalogItem(result productsearch.Result) agenttools.CatalogItem {
	ref := agenttools.CatalogRef{WorkspaceID: result.Reference.WorkspaceID, Type: agenttools.CatalogType(result.Reference.Type), ID: result.Reference.ID}
	hierarchy := make([]agenttools.CatalogHierarchyItem, 0, len(result.Hierarchy))
	for _, ancestor := range result.Hierarchy {
		hierarchy = append(hierarchy, agenttools.CatalogHierarchyItem{
			Ref:  agenttools.CatalogRef{WorkspaceID: result.Reference.WorkspaceID, Type: agenttools.CatalogType(ancestor.Type), ID: ancestor.ID},
			Name: ancestor.Name,
		})
	}
	locations := make([]agenttools.CatalogLocation, 0, len(result.Locations))
	for _, location := range result.Locations {
		if location.DashboardID == "" || location.PageID == "" {
			continue
		}
		locations = append(locations, agenttools.CatalogLocation{DashboardID: location.DashboardID, PageID: location.PageID})
	}
	return agenttools.CatalogItem{
		Ref: ref, Name: result.Name, Description: result.Description,
		Workspace: agenttools.CatalogWorkspace{
			Ref:  agenttools.CatalogRef{WorkspaceID: result.Workspace.ID, Type: agenttools.CatalogTypeWorkspace, ID: result.Workspace.ID},
			Name: result.Workspace.Name,
		},
		Hierarchy: hierarchy, Locations: locations, Href: result.Href, Capabilities: catalogCapabilities(ref.Type),
	}
}

func catalogWorkspaceItem(summary workspace.Summary) agenttools.CatalogItem {
	id := string(summary.ID)
	name := firstNonEmpty(summary.Title, id)
	ref := agenttools.CatalogRef{WorkspaceID: id, Type: agenttools.CatalogTypeWorkspace, ID: id}
	return agenttools.CatalogItem{
		Ref: ref, Name: name, Description: summary.Description,
		Workspace: agenttools.CatalogWorkspace{Ref: ref, Name: name},
		Hierarchy: []agenttools.CatalogHierarchyItem{},
		Href:      "/workspaces/" + url.PathEscape(id),
		Capabilities: []string{
			agenttools.CatalogGetToolName,
			agenttools.CatalogListToolName,
		},
	}
}

func catalogCapabilities(typ agenttools.CatalogType) []string {
	switch typ {
	case agenttools.CatalogTypeWorkspace, agenttools.CatalogTypeDashboard, agenttools.CatalogTypePage, agenttools.CatalogTypeSemanticTable:
		return []string{agenttools.CatalogGetToolName, agenttools.CatalogListToolName}
	case agenttools.CatalogTypeSemanticModel:
		return []string{agenttools.CatalogGetToolName, agenttools.CatalogListToolName, "query_semantic_model", agenttools.QueryVisualToolName}
	case agenttools.CatalogTypeVisual:
		return []string{agenttools.CatalogGetToolName, "query_dashboard_visual"}
	case agenttools.CatalogTypeField, agenttools.CatalogTypeMeasure:
		return []string{agenttools.CatalogGetToolName, "query_semantic_model", agenttools.QueryVisualToolName}
	default:
		return []string{agenttools.CatalogGetToolName}
	}
}

func catalogRequestedChildren(parent agenttools.CatalogType, requested []agenttools.CatalogType) []agenttools.CatalogType {
	if len(requested) > 0 {
		return requested
	}
	switch parent {
	case agenttools.CatalogTypeWorkspace:
		return []agenttools.CatalogType{agenttools.CatalogTypeDashboard, agenttools.CatalogTypeSemanticModel}
	case agenttools.CatalogTypeDashboard:
		return []agenttools.CatalogType{agenttools.CatalogTypePage}
	case agenttools.CatalogTypePage:
		return []agenttools.CatalogType{agenttools.CatalogTypeVisual, agenttools.CatalogTypeFilter}
	case agenttools.CatalogTypeSemanticModel:
		return []agenttools.CatalogType{agenttools.CatalogTypeSemanticTable, agenttools.CatalogTypeMeasure}
	case agenttools.CatalogTypeSemanticTable:
		return []agenttools.CatalogType{agenttools.CatalogTypeField}
	default:
		return nil
	}
}

func catalogGetLocation(request agenttools.CatalogGetRequest, locations []agenttools.CatalogLocation) (agenttools.CatalogLocation, error) {
	if request.Ref.Type != agenttools.CatalogTypeVisual && request.Ref.Type != agenttools.CatalogTypeFilter {
		return agenttools.CatalogLocation{}, nil
	}
	if request.Location == nil {
		if len(locations) == 1 {
			return locations[0], nil
		}
		if len(locations) > 1 {
			return agenttools.CatalogLocation{}, &agenttools.CatalogError{
				Code: "catalog_location_required", Message: "this resource is shared; pass one of its dashboard/page locations",
			}
		}
		return agenttools.CatalogLocation{}, catalogNotFound()
	}
	for _, location := range locations {
		if location == *request.Location {
			return location, nil
		}
	}
	return agenttools.CatalogLocation{}, catalogNotFound()
}

func catalogSearchError(err error) error {
	switch {
	case errors.Is(err, productsearch.ErrInvalidCursor):
		return &agenttools.CatalogError{Code: "invalid_arguments", Message: err.Error()}
	case errors.Is(err, productsearch.ErrSnapshotChanged):
		return &agenttools.CatalogError{Code: "catalog_snapshot_changed", Message: err.Error()}
	default:
		return err
	}
}

func catalogNotFound() error {
	return &agenttools.CatalogError{Code: "catalog_not_found", Message: "catalog resource was not found"}
}

type catalogListCursor struct {
	Scope      string                   `json:"scope"`
	Parent     *agenttools.CatalogRef   `json:"parent,omitempty"`
	ChildTypes []agenttools.CatalogType `json:"childTypes,omitempty"`
	Snapshot   string                   `json:"snapshot"`
	Offset     int                      `json:"offset"`
}

func encodeCatalogListCursor(scope agenttools.Scope, request agenttools.CatalogListRequest, snapshot string, offset int) string {
	payload, _ := json.Marshal(catalogListCursor{
		Scope: catalogScopeDigest(scope), Parent: request.Parent,
		ChildTypes: append([]agenttools.CatalogType(nil), request.ChildTypes...),
		Snapshot:   snapshot, Offset: offset,
	})
	return cursorsigning.Sign(catalogListCursorPrefix, payload)
}

func decodeCatalogListCursor(value string, scope agenttools.Scope, request agenttools.CatalogListRequest, snapshot string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	payload, err := cursorsigning.Verify(catalogListCursorPrefix, value)
	if err != nil {
		return 0, &agenttools.CatalogError{Code: "invalid_arguments", Message: "catalog cursor is invalid"}
	}
	var cursor catalogListCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Offset < 0 {
		return 0, &agenttools.CatalogError{Code: "invalid_arguments", Message: "catalog cursor is invalid"}
	}
	if cursor.Scope != catalogScopeDigest(scope) || !catalogRefsEqual(cursor.Parent, request.Parent) || !catalogTypesEqual(cursor.ChildTypes, request.ChildTypes) {
		return 0, &agenttools.CatalogError{Code: "invalid_arguments", Message: "catalog cursor does not match this request"}
	}
	if cursor.Snapshot != snapshot {
		return 0, &agenttools.CatalogError{Code: "catalog_snapshot_changed", Message: "catalog changed while browsing; restart from the first page"}
	}
	return cursor.Offset, nil
}

func catalogScopeDigest(scope agenttools.Scope) string {
	values := append([]string{scope.PrincipalID, scope.WorkspaceID, scope.Credential.WorkspaceID, fmt.Sprint(scope.Credential.Restricted)}, scope.Credential.Privileges...)
	sort.Strings(values[4:])
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(sum[:])
}

func catalogItemsSnapshot(items []agenttools.CatalogItem) string {
	encoded, _ := json.Marshal(items)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func catalogRefsEqual(left, right *agenttools.CatalogRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func catalogTypesEqual(left, right []agenttools.CatalogType) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func catalogPageIDs(id string) (string, string, bool) {
	index := strings.LastIndex(id, ".")
	if index <= 0 || index == len(id)-1 {
		return "", "", false
	}
	return id[:index], id[index+1:], true
}

func catalogSemanticTableIDs(id string) (string, string, bool) {
	return catalogPageIDs(id)
}

func catalogPage(pages []dashboard.Page, id string) (dashboard.Page, bool) {
	for _, page := range pages {
		if page.ID == id {
			return page, true
		}
	}
	return dashboard.Page{}, false
}

func catalogComponent(component dashboard.PageVisual, report dashboarddefinition.Definition) map[string]any {
	kind, ref := component.Kind, ""
	switch {
	case component.Visual != "":
		kind, ref = "visual", component.Visual
	case component.Filter != "":
		kind, ref = "filter", component.Filter
	}
	title := component.Title
	if title == "" {
		if value, ok := report.Visualizations[ref]; ok {
			title = dashboarddefinition.SpecTitle(value.Spec)
		} else if value, ok := report.Filters[ref]; ok {
			title = value.Label
		}
	}
	return map[string]any{
		"id": component.ID, "kind": kind, "ref": ref, "title": title,
		"description": component.Description, "placement": catalogPlacement(component.Placement),
	}
}

func catalogVisualDetails(metrics QueryMetrics, ref agenttools.CatalogRef, location agenttools.CatalogLocation) (map[string]any, error) {
	dashboardID, visualID, ok := catalogPageIDs(ref.ID)
	if !ok || (location.DashboardID != "" && location.DashboardID != dashboardID) {
		return nil, catalogNotFound()
	}
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		return nil, catalogNotFound()
	}
	page, ok := catalogPage(metrics.Pages(dashboardID), location.PageID)
	if !ok {
		return nil, catalogNotFound()
	}
	component, ok := catalogComponentForRef(page, visualID, false)
	if !ok {
		return nil, catalogNotFound()
	}
	if visual, exists := report.Visualizations[visualID]; exists {
		projection := dashboardhttp.DashboardVisualProjection(visual, component)
		columns := make([]api.DashboardTableColumn, 0)
		for _, column := range dashboarddefinition.TableColumns(visual.Spec) {
			columns = append(columns, api.DashboardTableColumn{
				Key: column.Key, Label: column.Label, Role: column.Role, Format: column.Format,
			})
		}
		return map[string]any{
			"type": string(ref.Type), "visualType": catalogVisualizationType(visual),
			"shape": string(visual.Query.ResultShape), "renderer": projection.RendererID,
			"query": catalogJSONMap(visual.Query), "columns": columns,
			"placement": catalogProjectedPlacement(projection.Placement, projection.X, projection.Y, projection.Width, projection.Height),
		}, nil
	}
	return nil, catalogNotFound()
}

func catalogVisualizationType(visual visualizationdefinition.Definition) string {
	spec := catalogJSONMap(visual.Spec)
	if mark, _ := spec["mark"].(string); mark != "" {
		return mark
	}
	kind, _ := spec["kind"].(string)
	return kind
}

func catalogFilterDetails(metrics QueryMetrics, ref agenttools.CatalogRef, location agenttools.CatalogLocation) (map[string]any, error) {
	dashboardID, filterID, ok := catalogPageIDs(ref.ID)
	if !ok || (location.DashboardID != "" && location.DashboardID != dashboardID) {
		return nil, catalogNotFound()
	}
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		return nil, catalogNotFound()
	}
	filter, exists := report.Filters[filterID]
	if !exists {
		return nil, catalogNotFound()
	}
	page, ok := catalogPage(metrics.Pages(dashboardID), location.PageID)
	if !ok {
		return nil, catalogNotFound()
	}
	component, ok := catalogComponentForRef(page, filterID, true)
	if !ok {
		return nil, catalogNotFound()
	}
	projection := dashboardhttp.DashboardFilterProjection(filterID, filter, component)
	return map[string]any{
		"type": string(ref.Type), "field": projection.Field,
		"configuration": catalogJSONMap(filter), "placement": catalogProjectedPlacement(projection.Placement, component.X, component.Y, component.Width, component.Height),
	}, nil
}

func (c agentCatalogService) semanticModelDetails(ctx context.Context, scope agenttools.Scope, metrics QueryMetrics, ref agenttools.CatalogRef) (map[string]any, error) {
	model, ok := metrics.SemanticModel(ref.ID)
	if !ok || model == nil {
		return nil, catalogNotFound()
	}
	projection, ok := analyticsqueryhttp.SemanticModelProjection(metrics, ref.ID)
	if !ok {
		return nil, catalogNotFound()
	}
	references := make([]productsearch.Reference, 0, len(projection.Dashboards))
	for _, dashboardUsage := range projection.Dashboards {
		references = append(references, productsearch.Reference{
			WorkspaceID: ref.WorkspaceID,
			Type:        productsearch.TypeDashboard,
			ID:          dashboardUsage.ID,
		})
	}
	authorized, err := c.server.search.Resolve(ctx, catalogSearchSubject(scope), c.server.defaultEnvironment, references)
	if err != nil {
		return nil, err
	}
	usage := make([]agenttools.CatalogRef, 0, len(authorized))
	for _, dashboardUsage := range authorized {
		usage = append(usage, catalogRefValue(ref.WorkspaceID, agenttools.CatalogTypeDashboard, dashboardUsage.Reference.ID))
	}
	fieldCount := 0
	if projection.Counts != nil {
		fieldCount = projection.Counts.Fields
	}
	return map[string]any{
		"type": string(ref.Type), "semanticTableCount": len(projection.Tables), "fieldCount": fieldCount,
		"measureCount": len(model.Measures) + len(model.Metrics), "dashboardCount": len(usage), "dashboardUsage": usage,
	}, nil
}

func catalogSemanticTableDetails(metrics QueryMetrics, ref agenttools.CatalogRef) (map[string]any, error) {
	modelID, tableID, ok := catalogSemanticTableIDs(ref.ID)
	if !ok {
		return nil, catalogNotFound()
	}
	model, ok := metrics.SemanticModel(modelID)
	if !ok || model == nil {
		return nil, catalogNotFound()
	}
	table, ok := model.Tables[tableID]
	if !ok {
		return nil, catalogNotFound()
	}
	projection := analyticsqueryhttp.SemanticTableProjection(model, tableID, table)
	keys := []string{}
	if projection.PrimaryKey != "" {
		keys = append(keys, projection.PrimaryKey)
	}
	return map[string]any{
		"type": string(ref.Type), "source": projection.Source, "sources": projection.Sources, "grain": projection.Grain,
		"primaryKey": projection.PrimaryKey, "keys": keys, "fieldCount": projection.FieldCount, "measureCount": projection.MeasureCount,
	}, nil
}

func catalogFieldDetails(metrics QueryMetrics, ref agenttools.CatalogRef) (map[string]any, error) {
	parts := strings.Split(ref.ID, ".")
	if len(parts) < 2 {
		return nil, catalogNotFound()
	}
	model, ok := metrics.SemanticModel(parts[0])
	if !ok || model == nil {
		return nil, catalogNotFound()
	}
	if len(parts) == 2 {
		field, ok := model.Dimensions[parts[1]]
		if !ok {
			return nil, catalogNotFound()
		}
		return map[string]any{
			"type": string(ref.Type), "kind": "dimension", "label": field.Label, "dataType": field.Type,
			"timeGrains": append([]string(nil), field.Grains...),
		}, nil
	}
	table, ok := model.Tables[parts[len(parts)-2]]
	if !ok {
		return nil, catalogNotFound()
	}
	field, ok := table.Dimensions[parts[len(parts)-1]]
	if !ok {
		return nil, catalogNotFound()
	}
	return map[string]any{
		"type": string(ref.Type), "kind": "dimension", "table": parts[len(parts)-2],
		"label": field.Label, "dataType": field.Type, "grain": table.Grain,
	}, nil
}

func catalogMeasureDetails(metrics QueryMetrics, ref agenttools.CatalogRef) (map[string]any, error) {
	modelID, measureID, ok := catalogPageIDs(ref.ID)
	if !ok {
		return nil, catalogNotFound()
	}
	model, ok := metrics.SemanticModel(modelID)
	if !ok || model == nil {
		return nil, catalogNotFound()
	}
	if measure, ok := model.Measures[measureID]; ok {
		return map[string]any{
			"type": string(ref.Type), "kind": "measure", "table": measure.Fact, "label": measure.Label,
			"aggregation": measure.Aggregation, "unit": measure.Unit, "format": measure.Format, "hidden": measure.Hidden,
		}, nil
	}
	if metric, ok := model.Metrics[measureID]; ok {
		return map[string]any{
			"type": string(ref.Type), "kind": "metric", "label": metric.Label,
			"unit": metric.Unit, "format": metric.Format, "hidden": metric.Hidden,
		}, nil
	}
	return nil, catalogNotFound()
}

func catalogComponentForRef(page dashboard.Page, id string, filter bool) (dashboard.PageVisual, bool) {
	for _, component := range page.Visuals {
		if filter && component.Filter == id {
			return component, true
		}
		if !filter && component.Visual == id {
			return component, true
		}
	}
	return dashboard.PageVisual{}, false
}

func catalogPlacement(value dashboard.PagePlacement) map[string]any {
	return map[string]any{"col": value.Col, "row": value.Row, "colSpan": value.ColSpan, "rowSpan": value.RowSpan}
}

func catalogProjectedPlacement(value *api.DashboardComponentPlacement, x, y, width, height float64) map[string]any {
	if value != nil {
		return map[string]any{"col": value.Col, "row": value.Row, "colSpan": value.ColSpan, "rowSpan": value.RowSpan}
	}
	return map[string]any{"x": x, "y": y, "width": width, "height": height}
}

func catalogJSONMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(encoded, &out)
	return out
}

func catalogRefValue(workspaceID string, typ agenttools.CatalogType, id string) agenttools.CatalogRef {
	return agenttools.CatalogRef{WorkspaceID: workspaceID, Type: typ, ID: id}
}

func sortedCatalogKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
