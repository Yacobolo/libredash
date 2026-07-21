package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/Yacobolo/leapview/internal/access"
	productsearch "github.com/Yacobolo/leapview/internal/search"
)

type Repository struct{ database *sql.DB }

func New(database *sql.DB) *Repository { return &Repository{database: database} }

func (r *Repository) Snapshot(ctx context.Context, query productsearch.RepositoryQuery) (string, error) {
	if r == nil || r.database == nil {
		return "", fmt.Errorf("search repository is not configured")
	}
	if query.NoWorkspaces {
		return snapshotDigest(nil), nil
	}
	statement := `SELECT workspace_id, serving_state_id FROM workspace_active_serving_states WHERE environment = ?`
	args := []any{normalizedEnvironment(query.Environment)}
	statement, args = appendStringFilter(statement, args, "workspace_id", query.Workspaces)
	statement += ` ORDER BY workspace_id, serving_state_id`
	rows, err := r.database.QueryContext(ctx, statement, args...)
	if err != nil {
		return "", fmt.Errorf("read search snapshot: %w", err)
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var workspaceID, servingStateID string
		if err := rows.Scan(&workspaceID, &servingStateID); err != nil {
			return "", fmt.Errorf("scan search snapshot: %w", err)
		}
		values = append(values, workspaceID+"\x00"+servingStateID)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate search snapshot: %w", err)
	}
	return snapshotDigest(values), nil
}

func (r *Repository) Candidates(ctx context.Context, query productsearch.RepositoryQuery, offset, limit int) ([]productsearch.Candidate, bool, error) {
	if r == nil || r.database == nil {
		return nil, false, fmt.Errorf("search repository is not configured")
	}
	if query.NoWorkspaces || query.NoTypes || limit <= 0 {
		return []productsearch.Candidate{}, false, nil
	}
	if offset < 0 {
		offset = 0
	}
	text := strings.TrimSpace(query.Text)
	var statement string
	var args []any
	if expression := matchExpression(text); expression != "" {
		statement, args = r.rankedStatement(query, expression, text, limit+1, offset)
	} else {
		statement, args = r.discoveryStatement(query, limit+1, offset)
	}
	rows, err := r.database.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, false, fmt.Errorf("query global search index: %w", err)
	}
	documents, err := scanDocuments(rows)
	if err != nil {
		return nil, false, err
	}
	more := len(documents) > limit
	if more {
		documents = documents[:limit]
	}
	candidates := make([]productsearch.Candidate, 0, len(documents))
	for _, document := range documents {
		candidate, err := r.hydrateCandidate(ctx, document, query.Context)
		if err != nil {
			return nil, false, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, more, nil
}

func (r *Repository) Resolve(ctx context.Context, environment string, references []productsearch.Reference) ([]productsearch.Candidate, error) {
	if len(references) == 0 {
		return []productsearch.Candidate{}, nil
	}
	workspaces := make([]string, 0, len(references))
	types := make([]productsearch.Type, 0, len(references))
	for _, reference := range references {
		workspaces = append(workspaces, reference.WorkspaceID)
		types = append(types, reference.Type)
	}
	workspaces = uniqueStrings(workspaces)
	types = uniqueTypes(types)
	statement := `
		SELECT id, workspace_id, environment, serving_state_id, asset_id, asset_type,
		       asset_key, parent_asset_id, title, description, workspace_title, terms
		FROM active_search_documents
		WHERE environment = ?`
	args := []any{normalizedEnvironment(environment)}
	statement, args = appendStringFilter(statement, args, "workspace_id", workspaces)
	statement, args = appendTypeFilter(statement, args, "asset_type", types)
	statement += ` ORDER BY workspace_id, asset_type, asset_key`
	rows, err := r.database.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("resolve search references: %w", err)
	}
	documents, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	wanted := make(map[productsearch.Reference]struct{}, len(references))
	for _, reference := range references {
		wanted[reference] = struct{}{}
	}
	out := make([]productsearch.Candidate, 0, len(references))
	for _, document := range documents {
		candidate, err := r.hydrateCandidate(ctx, document, productsearch.SearchContext{})
		if err != nil {
			return nil, err
		}
		if _, ok := wanted[candidate.Result.Reference]; ok {
			out = append(out, candidate)
		}
	}
	return out, nil
}

type document struct {
	rowID          int64
	workspaceID    string
	environment    string
	servingStateID string
	assetID        string
	assetType      string
	assetKey       string
	parentAssetID  string
	title          string
	description    string
	workspaceTitle string
	terms          string
}

func scanDocuments(rows *sql.Rows) ([]document, error) {
	defer rows.Close()
	documents := make([]document, 0)
	for rows.Next() {
		var value document
		if err := rows.Scan(
			&value.rowID, &value.workspaceID, &value.environment, &value.servingStateID,
			&value.assetID, &value.assetType, &value.assetKey, &value.parentAssetID,
			&value.title, &value.description, &value.workspaceTitle, &value.terms,
		); err != nil {
			return nil, fmt.Errorf("scan global search document: %w", err)
		}
		documents = append(documents, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate global search documents: %w", err)
	}
	return documents, nil
}

const documentColumns = `d.id, d.workspace_id, d.environment, d.serving_state_id, d.asset_id,
	d.asset_type, d.asset_key, d.parent_asset_id, d.title, d.description, d.workspace_title, d.terms`

func (r *Repository) rankedStatement(query productsearch.RepositoryQuery, expression, text string, limit, offset int) (string, []any) {
	contextRank, contextArgs := searchContextRank("d", query.Context)
	statement := `SELECT ` + documentColumns + `
		FROM active_search_documents_fts
		JOIN active_search_documents d ON d.id = active_search_documents_fts.rowid
		WHERE active_search_documents_fts MATCH ? AND d.environment = ?`
	args := []any{expression, normalizedEnvironment(query.Environment)}
	statement, args = appendStringFilter(statement, args, "d.workspace_id", query.Workspaces)
	statement, args = appendTypeFilter(statement, args, "d.asset_type", query.Types)
	statement += ` ORDER BY ` + contextRank + `,
		CASE WHEN lower(trim(d.title)) = lower(trim(?)) THEN 0 ELSE 1 END,
		CASE WHEN lower(trim(d.asset_key)) = lower(trim(?))
		          OR lower(trim(substr(d.asset_key, length(d.workspace_id) + 2))) = lower(trim(?))
		     THEN 0 ELSE 1 END,
		bm25(active_search_documents_fts, 8.0, 12.0, 5.0, 1.5, 1.0),
		lower(d.title), d.asset_type, d.workspace_id, d.asset_id
		LIMIT ? OFFSET ?`
	args = append(args, contextArgs...)
	args = append(args, text, text, text, limit, offset)
	return statement, args
}

func (r *Repository) discoveryStatement(query productsearch.RepositoryQuery, limit, offset int) (string, []any) {
	contextRank, contextArgs := searchContextRank("d", query.Context)
	statement := `WITH discovery AS (
		SELECT ` + documentColumns + `,
		       ` + contextRank + ` AS context_rank,
		       ROW_NUMBER() OVER (
		         PARTITION BY CASE WHEN d.asset_type = 'catalog' THEN 'workspace' ELSE d.asset_type END
		         ORDER BY lower(d.title), d.workspace_id, d.asset_id
		       ) AS type_rank,
		       CASE d.asset_type
		         WHEN 'catalog' THEN 0 WHEN 'dashboard' THEN 1 WHEN 'semantic_model' THEN 2
		         WHEN 'visual' THEN 3 WHEN 'page' THEN 4 WHEN 'measure' THEN 5
		         WHEN 'field' THEN 6 WHEN 'connection' THEN 7 WHEN 'source' THEN 8
		         WHEN 'model_table' THEN 9 WHEN 'semantic_table' THEN 10
		         WHEN 'filter' THEN 11 WHEN 'refresh_pipeline' THEN 12 ELSE 99 END AS type_priority
		FROM active_search_documents d
		WHERE d.environment = ?`
	args := append([]any(nil), contextArgs...)
	args = append(args, normalizedEnvironment(query.Environment))
	statement, args = appendStringFilter(statement, args, "d.workspace_id", query.Workspaces)
	statement, args = appendTypeFilter(statement, args, "d.asset_type", query.Types)
	statement += `)
		SELECT id, workspace_id, environment, serving_state_id, asset_id, asset_type,
		       asset_key, parent_asset_id, title, description, workspace_title, terms
		FROM discovery
		ORDER BY context_rank, type_rank, type_priority, lower(title), workspace_id, asset_id
		LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	return statement, args
}

func searchContextRank(alias string, context productsearch.SearchContext) (string, []any) {
	if strings.TrimSpace(context.WorkspaceID) == "" {
		return "3", nil
	}
	workspaceID := strings.TrimSpace(context.WorkspaceID)
	dashboardPrefix := workspaceID + "." + strings.TrimSpace(context.DashboardID)
	pageAssetID := "page:" + dashboardPrefix + "." + strings.TrimSpace(context.PageID)
	currentPage := `(` + alias + `.workspace_id = ? AND (` +
		`(` + alias + `.asset_type = 'dashboard' AND ` + alias + `.asset_key = ?) OR ` +
		`(` + alias + `.asset_type = 'page' AND ` + alias + `.asset_id = ?) OR ` +
		`(` + alias + `.asset_type IN ('visual','filter') AND EXISTS (` +
		`SELECT 1 FROM asset_edges edge JOIN assets item ON item.logical_asset_id = edge.from_logical_asset_id AND item.serving_state_id = edge.serving_state_id ` +
		`WHERE edge.serving_state_id = ` + alias + `.serving_state_id AND edge.to_logical_asset_id = ` + alias + `.asset_id AND item.parent_logical_asset_id = ?` +
		`))))`
	currentDashboard := `(` + alias + `.workspace_id = ? AND (` + alias + `.asset_key = ? OR substr(` + alias + `.asset_key, 1, length(?)+1) = ? || '.'))`
	expression := `CASE WHEN ` + currentPage + ` THEN 0 WHEN ` + currentDashboard + ` THEN 1 WHEN ` + alias + `.workspace_id = ? THEN 2 ELSE 3 END`
	return expression, []any{
		workspaceID, dashboardPrefix, pageAssetID, pageAssetID,
		workspaceID, dashboardPrefix, dashboardPrefix, dashboardPrefix,
		workspaceID,
	}
}

func (r *Repository) hydrateCandidate(ctx context.Context, document document, context productsearch.SearchContext) (productsearch.Candidate, error) {
	typ := publicType(document.assetType)
	reference := productsearch.Reference{WorkspaceID: document.workspaceID, Type: typ, ID: publicID(document)}
	ancestors, err := r.ancestors(ctx, document)
	if err != nil {
		return productsearch.Candidate{}, err
	}
	locations, err := r.locations(ctx, document, ancestors)
	if err != nil {
		return productsearch.Candidate{}, err
	}
	contextTags := contextTagsFor(document.workspaceID, locations, context)
	orderLocations(locations, context)
	href := genericAssetHref(document)
	if len(locations) > 0 {
		href = locations[0].Href
	}
	result := productsearch.Result{
		Reference: reference, Name: firstNonEmpty(document.title, reference.ID), Description: document.description,
		VisualType: visualTypeFromTerms(typ, document.terms),
		Workspace:  productsearch.Workspace{ID: document.workspaceID, Name: firstNonEmpty(document.workspaceTitle, document.workspaceID)},
		Hierarchy:  ancestorHierarchy(ancestors),
		Href:       href, Locations: locations, Context: contextTags,
	}
	object := securityObject(document, ancestors)
	locationObjects := make([]access.ObjectRef, 0, len(locations))
	for _, location := range locations {
		if location.DashboardID == "" {
			locationObjects = append(locationObjects, object)
			continue
		}
		locationObjects = append(locationObjects, access.ItemObjectWithParent(
			access.SecurableDashboard, document.workspaceID, location.DashboardID, access.WorkspaceObject(document.workspaceID),
		))
	}
	return productsearch.Candidate{
		Result: result, Object: object, LocationObjects: locationObjects,
		RequireLocation: document.assetType == "visual" || document.assetType == "filter",
	}, nil
}

func visualTypeFromTerms(typ productsearch.Type, terms string) string {
	if typ != productsearch.TypeVisual {
		return ""
	}
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(terms), &payload); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(payload.Type))
}

func ancestorHierarchy(ancestors []document) []productsearch.HierarchyItem {
	out := make([]productsearch.HierarchyItem, 0, len(ancestors))
	for index := len(ancestors) - 1; index >= 0; index-- {
		ancestor := ancestors[index]
		if ancestor.assetType == "catalog" {
			continue
		}
		out = append(out, productsearch.HierarchyItem{
			Type: publicType(ancestor.assetType), ID: publicID(ancestor),
			Name: firstNonEmpty(ancestor.title, publicID(ancestor)),
		})
	}
	return out
}

func (r *Repository) ancestors(ctx context.Context, value document) ([]document, error) {
	if value.parentAssetID == "" {
		return nil, nil
	}
	rows, err := r.database.QueryContext(ctx, `
		WITH RECURSIVE ancestors(id, workspace_id, environment, serving_state_id, asset_id, asset_type, asset_key, parent_asset_id, title, description, workspace_title, terms) AS (
		  SELECT id, workspace_id, environment, serving_state_id, asset_id, asset_type, asset_key, parent_asset_id, title, description, workspace_title, terms
		  FROM active_search_documents WHERE workspace_id = ? AND environment = ? AND asset_id = ?
		  UNION ALL
		  SELECT parent.id, parent.workspace_id, parent.environment, parent.serving_state_id, parent.asset_id, parent.asset_type, parent.asset_key,
		         parent.parent_asset_id, parent.title, parent.description, parent.workspace_title, parent.terms
		  FROM active_search_documents parent JOIN ancestors child ON parent.asset_id = child.parent_asset_id
		  WHERE parent.workspace_id = child.workspace_id AND parent.environment = child.environment
		)
		SELECT id, workspace_id, environment, serving_state_id, asset_id, asset_type, asset_key, parent_asset_id, title, description, workspace_title, terms FROM ancestors`,
		value.workspaceID, value.environment, value.parentAssetID)
	if err != nil {
		return nil, fmt.Errorf("read search result ancestors: %w", err)
	}
	return scanDocuments(rows)
}

func (r *Repository) locations(ctx context.Context, value document, ancestors []document) ([]productsearch.Location, error) {
	dashboard := ancestorOfType(append([]document{value}, ancestors...), "dashboard")
	switch value.assetType {
	case "catalog":
		return []productsearch.Location{{Href: "/workspaces/" + url.PathEscape(value.workspaceID)}}, nil
	case "dashboard":
		dashboardID := publicID(value)
		return []productsearch.Location{{DashboardID: dashboardID, DashboardName: value.title, Href: dashboardHref(value.workspaceID, dashboardID)}}, nil
	case "page":
		if dashboard.assetID == "" {
			return []productsearch.Location{{Href: genericAssetHref(value)}}, nil
		}
		dashboardID, pageID := publicID(dashboard), lastIDPart(publicID(value))
		return []productsearch.Location{{DashboardID: dashboardID, DashboardName: dashboard.title, PageID: pageID, PageName: value.title, Href: pageHref(value.workspaceID, dashboardID, pageID)}}, nil
	case "visual", "filter":
		rows, err := r.database.QueryContext(ctx, `
			SELECT page.asset_key, page.title, dashboard.asset_key, dashboard.title
			FROM asset_edges edge
			JOIN assets item ON item.logical_asset_id = edge.from_logical_asset_id AND item.serving_state_id = edge.serving_state_id
			JOIN assets page ON page.logical_asset_id = item.parent_logical_asset_id AND page.serving_state_id = edge.serving_state_id
			JOIN assets dashboard ON dashboard.logical_asset_id = page.parent_logical_asset_id AND dashboard.serving_state_id = edge.serving_state_id
			WHERE edge.serving_state_id = ? AND edge.to_logical_asset_id = ? AND edge.edge_type IN ('uses_visual','uses_filter')
			ORDER BY lower(dashboard.title), lower(page.title), page.asset_key`, value.servingStateID, value.assetID)
		if err != nil {
			return nil, fmt.Errorf("read search result locations: %w", err)
		}
		defer rows.Close()
		locations := make([]productsearch.Location, 0)
		for rows.Next() {
			var pageKey, pageName, dashboardKey, dashboardName string
			if err := rows.Scan(&pageKey, &pageName, &dashboardKey, &dashboardName); err != nil {
				return nil, fmt.Errorf("scan search result location: %w", err)
			}
			dashboardID := stripWorkspacePrefix(value.workspaceID, dashboardKey)
			pageID := lastIDPart(stripWorkspacePrefix(value.workspaceID, pageKey))
			locations = append(locations, productsearch.Location{
				DashboardID: dashboardID, DashboardName: dashboardName, PageID: pageID, PageName: pageName,
				Href: pageHref(value.workspaceID, dashboardID, pageID),
			})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate search result locations: %w", err)
		}
		if len(locations) > 0 {
			return locations, nil
		}
	}
	return []productsearch.Location{{Href: genericAssetHref(value)}}, nil
}

func contextTagsFor(workspaceID string, locations []productsearch.Location, context productsearch.SearchContext) []productsearch.ContextTag {
	if context.WorkspaceID == "" || workspaceID != context.WorkspaceID {
		return []productsearch.ContextTag{}
	}
	for _, location := range locations {
		if location.DashboardID == context.DashboardID && location.PageID == context.PageID && context.PageID != "" {
			return []productsearch.ContextTag{productsearch.ContextCurrentPage, productsearch.ContextCurrentDashboard, productsearch.ContextCurrentWorkspace}
		}
	}
	for _, location := range locations {
		if location.DashboardID == context.DashboardID && context.DashboardID != "" {
			return []productsearch.ContextTag{productsearch.ContextCurrentDashboard, productsearch.ContextCurrentWorkspace}
		}
	}
	return []productsearch.ContextTag{productsearch.ContextCurrentWorkspace}
}

func orderLocations(locations []productsearch.Location, context productsearch.SearchContext) {
	sort.SliceStable(locations, func(i, j int) bool {
		leftCurrent := locations[i].DashboardID == context.DashboardID && locations[i].PageID == context.PageID
		rightCurrent := locations[j].DashboardID == context.DashboardID && locations[j].PageID == context.PageID
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		if locations[i].DashboardName != locations[j].DashboardName {
			return locations[i].DashboardName < locations[j].DashboardName
		}
		return locations[i].PageName < locations[j].PageName
	})
}

func securityObject(value document, ancestors []document) access.ObjectRef {
	workspaceObject := access.WorkspaceObject(value.workspaceID)
	if value.assetType == "visual" || value.assetType == "filter" {
		return workspaceObject
	}
	dashboard := ancestorOfType(append([]document{value}, ancestors...), "dashboard")
	if dashboard.assetID != "" {
		return access.ItemObjectWithParent(access.SecurableDashboard, value.workspaceID, publicID(dashboard), workspaceObject)
	}
	switch value.assetType {
	case "semantic_model":
		return access.ItemObjectWithParent(access.SecurableSemanticModel, value.workspaceID, publicID(value), workspaceObject)
	case "semantic_table":
		model := ancestorOfType(ancestors, "semantic_model")
		modelID := publicID(model)
		modelObject := access.ItemObjectWithParent(access.SecurableSemanticModel, value.workspaceID, modelID, workspaceObject)
		return access.ItemObjectWithParent(access.SecurableDataset, value.workspaceID, modelID+"/"+lastIDPart(publicID(value)), modelObject)
	case "field":
		table := ancestorOfType(ancestors, "semantic_table")
		model := ancestorOfType(ancestors, "semantic_model")
		modelID, tableID := publicID(model), lastIDPart(publicID(table))
		modelObject := access.ItemObjectWithParent(access.SecurableSemanticModel, value.workspaceID, modelID, workspaceObject)
		tableObject := access.ItemObjectWithParent(access.SecurableDataset, value.workspaceID, modelID+"/"+tableID, modelObject)
		return access.ItemObjectWithParent(access.SecurableColumn, value.workspaceID, modelID+"/"+tableID+"/"+lastIDPart(publicID(value)), tableObject)
	case "measure":
		model := ancestorOfType(ancestors, "semantic_model")
		modelID := publicID(model)
		modelObject := access.ItemObjectWithParent(access.SecurableSemanticModel, value.workspaceID, modelID, workspaceObject)
		return access.ItemObjectWithParent(access.SecurableSemanticField, value.workspaceID, modelID+"/"+lastIDPart(publicID(value)), modelObject)
	case "source":
		return access.ItemObjectWithParent(access.SecurableSource, value.workspaceID, publicID(value), workspaceObject)
	case "model_table":
		return access.ItemObjectWithParent(access.SecurableModelTable, value.workspaceID, publicID(value), workspaceObject)
	}
	return workspaceObject
}

func ancestorOfType(values []document, typ string) document {
	for _, value := range values {
		if value.assetType == typ {
			return value
		}
	}
	return document{}
}

func publicType(assetType string) productsearch.Type {
	if assetType == "catalog" {
		return productsearch.TypeWorkspace
	}
	return productsearch.Type(assetType)
}

func publicID(value document) string {
	if value.assetType == "catalog" {
		return value.workspaceID
	}
	return stripWorkspacePrefix(value.workspaceID, value.assetKey)
}

func stripWorkspacePrefix(workspaceID, key string) string {
	return strings.TrimPrefix(key, workspaceID+".")
}

func lastIDPart(value string) string {
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}

func genericAssetHref(value document) string {
	return "/workspaces/" + url.PathEscape(value.workspaceID) + "/assets/" + url.PathEscape(value.assetID) + "/details"
}

func dashboardHref(workspaceID, dashboardID string) string {
	return "/workspaces/" + url.PathEscape(workspaceID) + "/dashboards/" + url.PathEscape(dashboardID)
}

func pageHref(workspaceID, dashboardID, pageID string) string {
	return dashboardHref(workspaceID, dashboardID) + "/pages/" + url.PathEscape(pageID)
}

func appendStringFilter(statement string, args []any, column string, values []string) (string, []any) {
	values = uniqueStrings(values)
	if len(values) == 0 {
		return statement, args
	}
	statement += " AND " + column + " IN (" + placeholders(len(values)) + ")"
	for _, value := range values {
		args = append(args, value)
	}
	return statement, args
}

func appendTypeFilter(statement string, args []any, column string, columnTypes []productsearch.Type) (string, []any) {
	values := make([]string, 0, len(columnTypes))
	for _, typ := range uniqueTypes(columnTypes) {
		if typ == productsearch.TypeWorkspace {
			values = append(values, "catalog")
		} else {
			values = append(values, string(typ))
		}
	}
	return appendStringFilter(statement, args, column, values)
}

func placeholders(count int) string { return strings.TrimSuffix(strings.Repeat("?,", count), ",") }

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueTypes(values []productsearch.Type) []productsearch.Type {
	seen := map[productsearch.Type]struct{}{}
	out := make([]productsearch.Type, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizedEnvironment(value string) string {
	if strings.TrimSpace(value) == "" {
		return "dev"
	}
	return strings.TrimSpace(value)
}

func snapshotDigest(values []string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x01")))
	return hex.EncodeToString(sum[:])
}

func matchExpression(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	stopWords := map[string]struct{}{"a": {}, "an": {}, "and": {}, "by": {}, "for": {}, "in": {}, "of": {}, "on": {}, "the": {}, "to": {}}
	expressions := make([]string, 0)
	seen := map[string]struct{}{}
	appendTerms := func(value string) {
		for _, term := range searchTokens(value) {
			if _, stop := stopWords[term]; stop {
				continue
			}
			key := "term:" + term
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			expressions = append(expressions, `"`+term+`"*`)
		}
	}
	for len(query) > 0 {
		start := strings.IndexByte(query, '"')
		if start < 0 {
			appendTerms(query)
			break
		}
		appendTerms(query[:start])
		query = query[start+1:]
		end := strings.IndexByte(query, '"')
		if end < 0 {
			appendTerms(query)
			break
		}
		phraseTerms := searchTokens(query[:end])
		if len(phraseTerms) == 1 {
			appendTerms(phraseTerms[0])
		} else if len(phraseTerms) > 1 {
			phrase := strings.Join(phraseTerms, " ")
			key := "phrase:" + phrase
			if _, duplicate := seen[key]; !duplicate {
				seen[key] = struct{}{}
				expressions = append(expressions, `"`+phrase+`"`)
			}
		}
		query = query[end+1:]
	}
	return strings.Join(expressions, " AND ")
}

func searchTokens(value string) []string {
	return strings.FieldsFunc(value, func(character rune) bool {
		return character != '_' && !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
