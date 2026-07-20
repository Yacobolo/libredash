package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/queryruntime"
)

type Principal struct {
	ID        string
	DevBypass bool
}

type Options struct {
	Repo                  access.Repository
	DefaultWorkspaceID    string
	PrincipalFromContext  func(context.Context) (Principal, bool)
	CredentialFromContext func(context.Context) (access.APICredential, bool)
	TokenAllows           func(access.APIToken, string, access.Privilege) bool
}

type Metrics struct {
	queryruntime.Metrics
	repo                  access.Repository
	defaultWorkspaceID    string
	principalFromContext  func(context.Context) (Principal, bool)
	credentialFromContext func(context.Context) (access.APICredential, bool)
	tokenAllows           func(access.APIToken, string, access.Privilege) bool
}

type DeniedError struct {
	PrincipalID string
	Privilege   access.Privilege
	Credential  bool
}

func (e DeniedError) Error() string {
	if e.Credential {
		return fmt.Sprintf("data query credential lacks %s", e.Privilege)
	}
	return fmt.Sprintf("principal %q lacks %s on data object", e.PrincipalID, e.Privilege)
}

func IsDenied(err error) bool {
	var denied DeniedError
	return errors.As(err, &denied)
}

func New(metrics queryruntime.Metrics, options Options) Metrics {
	return Metrics{
		Metrics:               metrics,
		repo:                  options.Repo,
		defaultWorkspaceID:    options.DefaultWorkspaceID,
		principalFromContext:  options.PrincipalFromContext,
		credentialFromContext: options.CredentialFromContext,
		tokenAllows:           options.TokenAllows,
	}
}

func (m Metrics) MetricsForWorkspace(workspaceID string) (queryruntime.Metrics, bool) {
	provider, ok := m.Metrics.(queryruntime.WorkspaceMetrics)
	if ok {
		metrics, ok := provider.MetricsForWorkspace(workspaceID)
		if !ok || metrics == nil {
			return nil, ok
		}
		m.Metrics = metrics
		m.defaultWorkspaceID = workspaceID
		return m, true
	}
	if m.Metrics == nil {
		return nil, false
	}
	if m.defaultWorkspaceID != "" && workspaceID == m.defaultWorkspaceID {
		return m, true
	}
	catalog := m.Metrics.Catalog()
	if catalog.Workspace.ID == "" || catalog.Workspace.ID == workspaceID {
		return m, true
	}
	return nil, false
}

func (m Metrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if m.Metrics == nil {
		return dataquery.Result{}, errors.New("query metrics are not configured")
	}
	if m.repo == nil {
		return m.Metrics.ExecuteDataQuery(ctx, request)
	}
	governed, transform, err := m.GovernDataQuery(ctx, request)
	if err != nil {
		return rejectedDataQueryResult(err)
	}
	ctx = dataquery.WithGovernanceApplied(ctx)
	result, err := m.Metrics.ExecuteDataQuery(ctx, governed)
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

func (m Metrics) GovernDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Query, dataquery.ResultTransformer, error) {
	request = request.WithMetadata(dataquery.MetadataFromContext(ctx))
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.defaultWorkspaceID
	}
	privilege := dataQueryPrivilege(request)
	objects := dataQueryObjects(request)
	semanticObjects, physicalObjects, err := m.resolvedDependencyObjects(request)
	if err != nil {
		return request, nil, err
	}
	objects = append(objects, semanticObjects...)
	objects = append(objects, physicalObjects...)
	principalID := strings.TrimSpace(request.PrincipalID)
	if principal, ok := m.currentPrincipal(ctx); ok {
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
	if credential, ok := m.currentCredential(ctx); ok && !m.allowsToken(credential.Token, request.WorkspaceID, privilege) {
		err := DeniedError{PrincipalID: principalID, Privilege: privilege, Credential: true}
		m.recordDataAccessAudit(ctx, request, privilege, objects, "denied", err)
		return request, nil, err
	}
	if ok, err := m.authorizeDataQuery(ctx, principalID, privilege, request, objects); err != nil {
		m.recordDataAccessAudit(ctx, request, privilege, objects, "error", err)
		return request, nil, err
	} else if !ok {
		err := DeniedError{PrincipalID: principalID, Privilege: privilege}
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

func (m Metrics) authorizeDataQuery(ctx context.Context, principalID string, privilege access.Privilege, request dataquery.Query, objects []access.ObjectRef) (bool, error) {
	if request.Kind == dataquery.KindSemanticAggregate && request.Target == "" {
		modelObject := access.ItemObject(access.SecurableSemanticModel, request.WorkspaceID, request.ModelID)
		decision, err := m.repo.Authorize(ctx, principalID, privilege, modelObject)
		if err != nil || !decision.Allowed {
			return decision.Allowed, err
		}
		for _, object := range objects {
			if object.Type != access.SecurableSemanticField {
				continue
			}
			decision, err := m.repo.Authorize(ctx, principalID, privilege, object)
			if err != nil || !decision.Allowed {
				return decision.Allowed, err
			}
		}
		return true, nil
	}
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

func (m Metrics) resolvedDependencyObjects(request dataquery.Query) ([]access.ObjectRef, []access.ObjectRef, error) {
	if request.Kind != dataquery.KindSemanticAggregate {
		return nil, nil, nil
	}
	model, ok := m.Metrics.SemanticModel(request.ModelID)
	if !ok || model == nil {
		return nil, nil, fmt.Errorf("unknown semantic model %q", request.ModelID)
	}
	queryRequest := semanticquery.Request{
		Table:      request.Target,
		Dimensions: dataFieldsToSemanticFields(request.Fields),
		Measures:   dataFieldsToSemanticFields(request.Measures),
		Time:       semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:    dataFiltersToSemanticFilters(request.Filters),
	}
	dependencies, err := semanticquery.ResolveDependencies(model, queryRequest)
	if err != nil {
		return nil, nil, err
	}
	modelObject := access.ItemObject(access.SecurableSemanticModel, request.WorkspaceID, request.ModelID)
	semanticObjects := make([]access.ObjectRef, 0, len(dependencies.LogicalFields))
	for _, field := range dependencies.LogicalFields {
		if !isSemanticField(model, field) {
			continue
		}
		semanticObjects = append(semanticObjects, access.ItemObjectWithParent(access.SecurableSemanticField, request.WorkspaceID, request.ModelID+"/"+field, modelObject))
	}
	physicalObjects := make([]access.ObjectRef, 0, len(dependencies.Facts)+len(dependencies.PhysicalFields))
	datasets := map[string]access.ObjectRef{}
	for _, fact := range dependencies.Facts {
		dataset := access.ItemObjectWithParent(access.SecurableDataset, request.WorkspaceID, request.ModelID+"/"+fact, modelObject)
		datasets[fact] = dataset
		physicalObjects = append(physicalObjects, dataset)
	}
	for _, field := range dependencies.PhysicalFields {
		table, column, ok := splitFieldRef(field)
		if !ok {
			continue
		}
		tableObject, ok := datasets[table]
		if !ok {
			tableObject = access.ItemObjectWithParent(access.SecurableDataset, request.WorkspaceID, request.ModelID+"/"+table, modelObject)
			datasets[table] = tableObject
			physicalObjects = append(physicalObjects, tableObject)
		}
		physicalObjects = append(physicalObjects, access.ItemObjectWithParent(access.SecurableColumn, request.WorkspaceID, request.ModelID+"/"+table+"/"+column, tableObject))
	}
	return semanticObjects, physicalObjects, nil
}

func isSemanticField(model *semanticmodel.Model, field string) bool {
	if _, ok := model.Dimensions[field]; ok {
		return true
	}
	if _, ok := model.Measures[field]; ok {
		return true
	}
	_, ok := model.Metrics[field]
	return ok
}

func dataFieldsToSemanticFields(fields []dataquery.Field) []semanticquery.Field {
	out := make([]semanticquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, semanticquery.Field{Field: field.Field, Alias: field.Alias})
	}
	return out
}

func dataFiltersToSemanticFilters(filters []dataquery.Filter) []semanticquery.Filter {
	out := make([]semanticquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]semanticquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, semanticquery.FilterGroup{Filters: dataFiltersToSemanticFilters(group.Filters)})
		}
		out = append(out, semanticquery.Filter{Field: filter.Field, Fact: filter.Fact, Operator: filter.Operator, Values: append([]any{}, filter.Values...), Groups: groups})
	}
	return out
}

func (m Metrics) recordDataAccessAudit(ctx context.Context, request dataquery.Query, privilege access.Privilege, objects []access.ObjectRef, status string, cause error) {
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
	_ = access.PersistAuditEvent(ctx, m.repo, access.AuditEventInput{
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

func (m Metrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	result, err := m.ExecuteDataQuery(ctx, semanticAggregateDataQuery(modelID, request))
	return queryRowsFromDataResult(result.Rows), err
}

func (m Metrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
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

func queryFieldsToDataFields(fields []reportdef.QueryField) []dataquery.Field {
	out := make([]dataquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, dataquery.Field{
			Field: field.Field,
			Alias: field.Alias,
		})
	}
	return out
}

func queryFiltersToDataFilters(filters []reportdef.QueryFilter) []dataquery.Filter {
	out := make([]dataquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]dataquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, dataquery.FilterGroup{Filters: queryFiltersToDataFilters(group.Filters)})
		}
		out = append(out, dataquery.Filter{
			Field:    filter.Field,
			Fact:     filter.Fact,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   groups,
		})
	}
	return out
}

func querySortToDataSort(sort []reportdef.QuerySort) []dataquery.Sort {
	out := make([]dataquery.Sort, 0, len(sort))
	for _, item := range sort {
		out = append(out, dataquery.Sort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func queryRowsFromDataResult(rows []dataquery.Row) reportdef.QueryRows {
	out := make(reportdef.QueryRows, 0, len(rows))
	for _, row := range rows {
		converted := reportdef.QueryRow{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
}

func (m Metrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m Metrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.Metrics.QueryDashboardPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters)
}

func (m Metrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m Metrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.Metrics.QueryTablePage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
}

func (m Metrics) applyDataPolicies(ctx context.Context, request dataquery.Query, objects []access.ObjectRef) (dataquery.Query, error) {
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
			request.Filters = append(request.Filters, m.resolvePolicyFilterFacts(request, filters)...)
		case "column_mask":
			mask, err := columnMaskFromPolicy(policy)
			if err != nil {
				return request, err
			}
			maskedFields := selectedMaskedFields(request, mask)
			if request.Kind == dataquery.KindSemanticAggregate {
				maskedFields = append(maskedFields, mask.Fields...)
			}
			for _, field := range uniqueStrings(maskedFields) {
				request.ColumnMasks = append(request.ColumnMasks, dataquery.ColumnMask{Field: field, Mask: mask.Mask})
			}
		}
	}
	return request, nil
}

func (m Metrics) resolvePolicyFilterFacts(request dataquery.Query, filters []dataquery.Filter) []dataquery.Filter {
	if request.Kind != dataquery.KindSemanticAggregate || request.Target != "" {
		return filters
	}
	model, ok := m.Metrics.SemanticModel(request.ModelID)
	if !ok || model == nil {
		return filters
	}
	dependencies, err := semanticquery.ResolveDependencies(model, semanticquery.Request{
		Dimensions: dataFieldsToSemanticFields(request.Fields), Measures: dataFieldsToSemanticFields(request.Measures),
		Time: semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
	})
	if err != nil {
		return filters
	}
	out := []dataquery.Filter{}
	for _, filter := range filters {
		if filter.Field == "" || filter.Fact != "" || len(filter.Groups) > 0 {
			out = append(out, filter)
			continue
		}
		if _, conformed := model.Dimensions[filter.Field]; conformed {
			out = append(out, filter)
			continue
		}
		physical, err := model.ResolveDimension(filter.Field)
		if err != nil {
			out = append(out, filter)
			continue
		}
		for _, fact := range dependencies.Facts {
			if _, err := model.SafeRelationshipPath(fact, physical.Table); err == nil {
				copy := filter
				copy.Fact = fact
				out = append(out, copy)
			}
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func (m Metrics) effectiveDataPolicies(ctx context.Context, request dataquery.Query, objects []access.ObjectRef) ([]access.DataPolicy, error) {
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
	fields := make([]string, 0, len(request.Fields)+len(request.Measures)+len(request.AuthorizationFields)+1)
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
	for _, field := range request.AuthorizationFields {
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

func (m Metrics) currentPrincipal(ctx context.Context) (Principal, bool) {
	if m.principalFromContext == nil {
		return Principal{}, false
	}
	return m.principalFromContext(ctx)
}

func (m Metrics) currentCredential(ctx context.Context) (access.APICredential, bool) {
	if m.credentialFromContext == nil {
		return access.APICredential{}, false
	}
	return m.credentialFromContext(ctx)
}

func (m Metrics) allowsToken(token access.APIToken, workspaceID string, privilege access.Privilege) bool {
	if m.tokenAllows == nil {
		return true
	}
	return m.tokenAllows(token, workspaceID, privilege)
}
