package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type dataAuthorizationMetrics struct {
	QueryMetrics
	repo               access.Repository
	defaultWorkspaceID string
}

func (m dataAuthorizationMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	provider, ok := m.QueryMetrics.(workspaceMetrics)
	if ok {
		metrics, ok := provider.MetricsForWorkspace(workspaceID)
		if !ok || metrics == nil {
			return nil, ok
		}
		return dataAuthorizationMetrics{QueryMetrics: metrics, repo: m.repo, defaultWorkspaceID: workspaceID}, true
	}
	if m.QueryMetrics == nil {
		return nil, false
	}
	if m.defaultWorkspaceID != "" && workspaceID == m.defaultWorkspaceID {
		return m, true
	}
	catalog := m.QueryMetrics.Catalog()
	if catalog.Workspace.ID == "" || catalog.Workspace.ID == workspaceID {
		return m, true
	}
	return nil, false
}

func (m dataAuthorizationMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if m.QueryMetrics == nil {
		return dataquery.Result{}, errors.New("query metrics are not configured")
	}
	if m.repo == nil {
		return m.QueryMetrics.ExecuteDataQuery(ctx, request)
	}
	governed, transform, err := m.GovernDataQuery(ctx, request)
	if err != nil {
		return rejectedDataQueryResult(err)
	}
	ctx = dataquery.WithGovernanceApplied(ctx)
	result, err := m.QueryMetrics.ExecuteDataQuery(ctx, governed)
	if transform != nil {
		if transformErr := transform(&result, err); transformErr != nil {
			return rejectedDataQueryResult(transformErr)
		}
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (m dataAuthorizationMetrics) GovernDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	request = request.WithMetadata(dataquery.MetadataFromContext(ctx))
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.defaultWorkspaceID
	}
	privilege := dataQueryPrivilege(request)
	objects := dataQueryObjects(request)
	principalID := strings.TrimSpace(request.PrincipalID)
	if principal, ok := principalFromContext(ctx); ok {
		if principal.DevBypass {
			return request, nil, nil
		}
		if principalID == "" {
			principalID = principal.ID
			request.PrincipalID = principal.ID
		}
	}
	if principalID == "" {
		err := dataquery.ErrMissingPrincipal
		m.recordDataAccessAudit(ctx, request, "", objects, "denied", err)
		return request, nil, err
	}
	if credential, ok := apiCredentialFromContext(ctx); ok && !apiTokenAllows(credential.Token, request.WorkspaceID, privilege) {
		err := fmt.Errorf("data query credential lacks %s", privilege)
		m.recordDataAccessAudit(ctx, request, privilege, objects, "denied", err)
		return request, nil, err
	}
	if ok, err := m.authorizeDataQuery(ctx, principalID, privilege, request, objects); err != nil {
		m.recordDataAccessAudit(ctx, request, privilege, objects, "error", err)
		return request, nil, err
	} else if !ok {
		err := fmt.Errorf("principal %q lacks %s on data object", principalID, privilege)
		m.recordDataAccessAudit(ctx, request, privilege, objects, "denied", err)
		return request, nil, err
	}
	governed, err := m.applyDataPolicies(ctx, request, objects)
	if err != nil {
		m.recordDataAccessAudit(ctx, request, privilege, objects, "error", err)
		return request, nil, err
	}
	return governed, func(result *dataquery.Result, executeErr error) error {
		if executeErr != nil {
			m.recordDataAccessAudit(ctx, governed, privilege, objects, "error", executeErr)
			return nil
		}
		status := "success"
		if result != nil && result.Status == dataquery.StatusError {
			status = "error"
		}
		m.recordDataAccessAudit(ctx, governed, privilege, objects, status, nil)
		return nil
	}, nil
}

func (m dataAuthorizationMetrics) authorizeDataQuery(ctx context.Context, principalID string, privilege access.Privilege, request dataquery.Query, objects []access.ObjectRef) (bool, error) {
	decision, err := m.repo.AuthorizeAny(ctx, principalID, privilege, objects)
	if err != nil || decision.Allowed {
		return decision.Allowed, err
	}
	columnObjects := dataQueryColumnObjects(request)
	if len(columnObjects) == 0 {
		return false, nil
	}
	for _, column := range columnObjects {
		columnDecision, err := m.repo.Authorize(ctx, principalID, privilege, column)
		if err != nil {
			return false, err
		}
		if !columnDecision.Allowed {
			return false, nil
		}
	}
	return true, nil
}

func (m dataAuthorizationMetrics) recordDataAccessAudit(ctx context.Context, request dataquery.Query, privilege access.Privilege, objects []access.ObjectRef, status string, cause error) {
	if m.repo == nil {
		return
	}
	action := "data_query.executed"
	if privilege == access.PrivilegePreviewData {
		action = "data_preview.executed"
	}
	targetType := strings.TrimSpace(request.ObjectType)
	targetID := strings.TrimSpace(request.ObjectID)
	if targetType == "" || targetID == "" {
		for _, object := range objects {
			if object.Type == "" {
				continue
			}
			if targetType == "" {
				targetType = string(object.Type)
			}
			if targetID == "" {
				targetID = object.CanonicalID()
			}
			break
		}
	}
	metadata := map[string]any{
		"kind":      string(request.Kind),
		"surface":   request.Surface,
		"operation": request.Operation,
		"modelId":   request.ModelID,
		"target":    request.Target,
	}
	if cause != nil {
		metadata["error"] = cause.Error()
	}
	bytes, _ := json.Marshal(metadata)
	_ = m.repo.RecordAuditEvent(ctx, access.AuditEventInput{
		WorkspaceID:   request.WorkspaceID,
		PrincipalID:   request.PrincipalID,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		Privilege:     privilege,
		Status:        status,
		RequestID:     request.RequestID,
		CorrelationID: request.CorrelationID,
		MetadataJSON:  string(bytes),
	})
}

func (m dataAuthorizationMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	result, err := m.ExecuteDataQuery(ctx, semanticAggregateDataQuery(modelID, request))
	return queryRowsFromDataResult(result.Rows), err
}

func (m dataAuthorizationMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	result, err := m.ExecuteDataQuery(ctx, semanticRowsDataQuery(modelID, request))
	return queryRowsFromDataResult(result.Rows), err
}

func semanticAggregateDataQuery(modelID string, request reportdef.AggregateQuery) dataquery.Query {
	return dataquery.Query{
		ModelID:  modelID,
		Kind:     dataquery.KindSemanticAggregate,
		Target:   request.Table,
		Fields:   queryFieldsToDataFields(request.Dimensions),
		Measures: queryFieldsToDataFields(request.Measures),
		Time:     dataquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:  queryFiltersToDataFilters(request.Filters),
		Sort:     querySortToDataSort(request.Sort),
		Limit:    request.Limit,
		Offset:   request.Offset,
	}
}

func semanticRowsDataQuery(modelID string, request reportdef.RowQuery) dataquery.Query {
	return dataquery.Query{
		ModelID:  modelID,
		Kind:     dataquery.KindSemanticRows,
		Target:   request.Table,
		Fields:   queryFieldsToDataFields(request.Dimensions),
		Measures: queryFieldsToDataFields(request.Measures),
		Filters:  queryFiltersToDataFilters(request.Filters),
		Sort:     querySortToDataSort(request.Sort),
		Limit:    request.Limit,
		Offset:   request.Offset,
	}
}

func (m dataAuthorizationMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m dataAuthorizationMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryMetrics.QueryDashboardPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters)
}

func (m dataAuthorizationMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m dataAuthorizationMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryMetrics.QueryTablePage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
}

func (m dataAuthorizationMetrics) RefreshModelTables(ctx context.Context, modelID string, tableNames []string) error {
	if port, ok := m.QueryMetrics.(modelTableRefreshMetrics); ok {
		return port.RefreshModelTables(ctx, modelID, tableNames)
	}
	return errors.New("model table refresh is not configured")
}

func (m dataAuthorizationMetrics) RefreshTables(ctx context.Context, modelID string, tableNames []string) error {
	if port, ok := m.QueryMetrics.(modelTableRefreshRuntimeMetrics); ok {
		return port.RefreshTables(ctx, modelID, tableNames)
	}
	return errors.New("model table refresh is not configured")
}

func (m dataAuthorizationMetrics) applyDataPolicies(ctx context.Context, request dataquery.Query, objects []access.ObjectRef) (dataquery.Query, error) {
	policies, err := m.effectiveDataPolicies(ctx, request, objects)
	if err != nil {
		return request, err
	}
	for _, policy := range policies {
		switch policy.PolicyType {
		case "row_filter":
			filters, err := rowFiltersFromPolicy(policy)
			if err != nil {
				return request, err
			}
			request.Filters = append(request.Filters, filters...)
		case "column_mask":
			mask, err := columnMaskFromPolicy(policy)
			if err != nil {
				return request, err
			}
			for _, field := range selectedMaskedFields(request, mask) {
				request.ColumnMasks = append(request.ColumnMasks, dataquery.ColumnMask{Field: field, Mask: mask.Mask})
			}
		}
	}
	return request, nil
}

func (m dataAuthorizationMetrics) effectiveDataPolicies(ctx context.Context, request dataquery.Query, objects []access.ObjectRef) ([]access.DataPolicy, error) {
	seenObjects := map[string]struct{}{}
	seenPolicies := map[string]struct{}{}
	out := []access.DataPolicy{}
	addObject := func(object access.ObjectRef) error {
		if object.Type == "" {
			return nil
		}
		key := object.CanonicalID()
		if _, ok := seenObjects[key]; ok {
			return nil
		}
		seenObjects[key] = struct{}{}
		policies, err := m.repo.ListEffectiveDataPolicies(ctx, request.PrincipalID, object, true)
		if err != nil {
			return err
		}
		for _, policy := range policies {
			if _, ok := seenPolicies[policy.ID]; ok {
				continue
			}
			seenPolicies[policy.ID] = struct{}{}
			out = append(out, policy)
		}
		return nil
	}
	for _, object := range objects {
		if err := addObject(object); err != nil {
			return nil, err
		}
	}
	for _, object := range dataQueryColumnObjects(request) {
		if err := addObject(object); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func dataQueryPrivilege(request dataquery.Query) access.Privilege {
	switch request.Operation {
	case dataquery.OperationAPIPreview, dataquery.OperationPreviewWindow:
		return access.PrivilegePreviewData
	}
	switch request.Kind {
	case dataquery.KindModelTableRows, dataquery.KindSourceRows:
		return access.PrivilegePreviewData
	case dataquery.KindSemanticRows:
		if request.Surface == dataquery.SurfaceDashboard {
			return access.PrivilegeQueryData
		}
		return access.PrivilegePreviewData
	default:
		return access.PrivilegeQueryData
	}
}

func dataQueryObjects(request dataquery.Query) []access.ObjectRef {
	workspaceID := request.WorkspaceID
	modelID := request.ModelID
	objects := []access.ObjectRef{}
	switch request.Kind {
	case dataquery.KindSourceRows:
		objects = append(objects, access.ItemObject(access.SecurableSource, workspaceID, request.Target))
	case dataquery.KindModelTableRows:
		objects = append(objects, access.ItemObjectWithParent(access.SecurableModelTable, workspaceID, modelID+"/"+request.Target, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID)))
	default:
		if request.Target != "" {
			objects = append(objects, access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+request.Target, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID)))
		}
	}
	if modelID != "" {
		objects = append(objects, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID))
	}
	if workspaceID != "" {
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	return objects
}

func dataQueryColumnObjects(request dataquery.Query) []access.ObjectRef {
	objects := []access.ObjectRef{}
	for _, field := range dataQuerySelectedFields(request) {
		table, column, ok := splitFieldRef(field)
		if !ok {
			continue
		}
		parent := access.ItemObjectWithParent(access.SecurableDataset, request.WorkspaceID, request.ModelID+"/"+table, access.ItemObject(access.SecurableSemanticModel, request.WorkspaceID, request.ModelID))
		objects = append(objects, access.ItemObjectWithParent(access.SecurableColumn, request.WorkspaceID, request.ModelID+"/"+table+"/"+column, parent))
	}
	return objects
}

func dataQuerySelectedFields(request dataquery.Query) []string {
	fields := make([]string, 0, len(request.Fields)+len(request.Measures)+1)
	for _, field := range request.Fields {
		if field.Field != "" {
			fields = append(fields, field.Field)
		}
	}
	for _, field := range request.Measures {
		if field.Field != "" {
			fields = append(fields, field.Field)
		}
	}
	if request.Value.Field != "" {
		fields = append(fields, request.Value.Field)
	}
	return fields
}

type dataPolicyExpression struct {
	Field    string             `json:"field"`
	Columns  []string           `json:"columns"`
	Operator string             `json:"operator"`
	Values   []any              `json:"values"`
	Value    any                `json:"value"`
	Filters  []dataquery.Filter `json:"filters"`
	Mask     string             `json:"mask"`
}

func rowFiltersFromPolicy(policy access.DataPolicy) ([]dataquery.Filter, error) {
	var expression dataPolicyExpression
	if err := json.Unmarshal([]byte(policy.ExpressionJSON), &expression); err != nil {
		return nil, fmt.Errorf("data policy %q expression is invalid: %w", policy.ID, err)
	}
	if len(expression.Filters) > 0 {
		return expression.Filters, nil
	}
	if strings.TrimSpace(expression.Field) == "" {
		return nil, fmt.Errorf("row_filter data policy %q requires field or filters", policy.ID)
	}
	operator := strings.TrimSpace(expression.Operator)
	if operator == "" {
		operator = "equals"
	}
	values := append([]any{}, expression.Values...)
	if len(values) == 0 && expression.Value != nil {
		values = append(values, expression.Value)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("row_filter data policy %q requires values", policy.ID)
	}
	return []dataquery.Filter{{Field: expression.Field, Operator: operator, Values: values}}, nil
}

type columnMaskPolicy struct {
	PolicyID string
	Fields   []string
	Mask     string
}

func columnMaskFromPolicy(policy access.DataPolicy) (columnMaskPolicy, error) {
	var expression dataPolicyExpression
	if err := json.Unmarshal([]byte(policy.ExpressionJSON), &expression); err != nil {
		return columnMaskPolicy{}, fmt.Errorf("data policy %q expression is invalid: %w", policy.ID, err)
	}
	fields := append([]string{}, expression.Columns...)
	if strings.TrimSpace(expression.Field) != "" {
		fields = append(fields, strings.TrimSpace(expression.Field))
	}
	if len(fields) == 0 {
		return columnMaskPolicy{}, fmt.Errorf("column_mask data policy %q requires field or columns", policy.ID)
	}
	mask := strings.TrimSpace(expression.Mask)
	if mask == "" {
		mask = "null"
	}
	return columnMaskPolicy{PolicyID: policy.ID, Fields: fields, Mask: mask}, nil
}

func selectedMaskedFields(request dataquery.Query, mask columnMaskPolicy) []string {
	selected := map[string]string{}
	leafSelected := map[string]string{}
	ambiguousLeaf := map[string]bool{}
	for _, field := range dataQuerySelectedFields(request) {
		normalized := strings.ToLower(strings.TrimSpace(field))
		selected[normalized] = field
		leaf := strings.ToLower(strings.TrimSpace(fieldNameLeaf(field)))
		if existing, ok := leafSelected[leaf]; ok && existing != field {
			ambiguousLeaf[leaf] = true
		} else {
			leafSelected[leaf] = field
		}
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, field := range mask.Fields {
		key := strings.ToLower(strings.TrimSpace(field))
		selectedField, ok := selected[key]
		if !ok {
			leaf := strings.ToLower(strings.TrimSpace(fieldNameLeaf(field)))
			if ambiguousLeaf[leaf] {
				continue
			}
			selectedField, ok = leafSelected[leaf]
			if !ok {
				continue
			}
		}
		seenKey := strings.ToLower(strings.TrimSpace(selectedField))
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		out = append(out, selectedField)
	}
	return out
}

func fieldNameLeaf(field string) string {
	_, column, ok := splitFieldRef(field)
	if !ok {
		return field
	}
	return column
}

func splitFieldRef(field string) (string, string, bool) {
	table, column, ok := strings.Cut(strings.TrimSpace(field), ".")
	return table, column, ok && table != "" && column != ""
}

func rejectedDataQueryResult(err error) (dataquery.Result, error) {
	return dataquery.Result{Status: dataquery.StatusError, ExecutionState: dataquery.ExecutionRejected, Error: err.Error()}, err
}

func apiCredentialFromContext(ctx context.Context) (access.APICredential, bool) {
	credential, ok := ctx.Value(apiCredentialContextKey{}).(access.APICredential)
	return credential, ok
}
