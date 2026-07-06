package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func projectModelTable(spec projectModelTableSpec) semanticmodel.Table {
	table := semanticmodel.Table{
		Kind:        spec.Kind,
		Source:      spec.Source,
		Sources:     append([]string{}, spec.Sources...),
		SourceReads: copyStringSliceMap(spec.SourceReads),
		SQL:         spec.SQL,
		Transform:   spec.Transform,
		Columns:     copyModelColumns(spec.Columns),
		PrimaryKey:  spec.PrimaryKey,
		Grain:       spec.Grain,
		Dimensions:  map[string]semanticmodel.MetricDimension{},
		Measures:    copyMeasures(spec.Measures),
		Description: spec.Description,
	}
	for name, field := range spec.Fields {
		table.Dimensions[name] = semanticmodel.MetricDimension{
			Label:       field.Label,
			Description: field.Description,
			Type:        field.Type,
			Expr:        field.Expr,
			Expression:  field.Expression,
		}
		if field.Type != "" {
			if table.Columns == nil {
				table.Columns = map[string]semanticmodel.ModelColumn{}
			}
			column := table.Columns[name]
			column.Type = field.Type
			column.Description = firstNonEmpty(column.Description, field.Description)
			table.Columns[name] = column
		}
	}
	return table
}

func projectDashboardPages(pages []projectDashboardPage) []dashboard.Page {
	out := make([]dashboard.Page, 0, len(pages))
	for _, page := range pages {
		out = append(out, dashboard.Page{
			ID:          page.Name,
			Title:       page.Title,
			Description: page.Description,
			Canvas:      page.Canvas,
			Grid:        page.Grid,
			Visuals:     page.Visuals,
		})
	}
	return out
}

func projectWorkspaceGroup(name string, spec workspaceGroupSpec) workspace.WorkspaceGroup {
	group := workspace.WorkspaceGroup{
		ID:          name,
		Name:        name,
		Description: spec.Description,
		Members:     make([]workspace.WorkspaceGroupMember, 0, len(spec.Members)),
	}
	for _, member := range spec.Members {
		group.Members = append(group.Members, workspace.WorkspaceGroupMember{
			PrincipalID: strings.TrimSpace(member.PrincipalID),
			Email:       strings.TrimSpace(member.Email),
			DisplayName: strings.TrimSpace(member.DisplayName),
		})
	}
	sort.SliceStable(group.Members, func(i, j int) bool {
		return accessMemberSortKey(group.Members[i]) < accessMemberSortKey(group.Members[j])
	})
	return group
}

func projectWorkspaceRoleBinding(name string, spec workspaceRoleBindingSpec) workspace.WorkspaceRoleBinding {
	return workspace.WorkspaceRoleBinding{
		ID:   name,
		Name: name,
		Role: strings.TrimSpace(spec.Role),
		Subject: workspace.WorkspaceRoleBindingSubject{
			Kind:        strings.TrimSpace(spec.Subject.Kind),
			PrincipalID: strings.TrimSpace(spec.Subject.PrincipalID),
			Email:       strings.TrimSpace(spec.Subject.Email),
			DisplayName: strings.TrimSpace(spec.Subject.DisplayName),
			Group:       strings.TrimSpace(spec.Subject.Group),
		},
	}
}

func projectWorkspaceGrant(name string, spec workspaceGrantSpec) workspace.WorkspaceGrant {
	return workspace.WorkspaceGrant{
		ID:   name,
		Name: name,
		Object: workspace.WorkspaceSecurableObjectRef{
			Type: strings.TrimSpace(spec.Object.Type),
			ID:   strings.TrimSpace(spec.Object.ID),
		},
		Subject: workspace.WorkspaceRoleBindingSubject{
			Kind:        strings.TrimSpace(spec.Subject.Kind),
			PrincipalID: strings.TrimSpace(spec.Subject.PrincipalID),
			Email:       strings.TrimSpace(spec.Subject.Email),
			DisplayName: strings.TrimSpace(spec.Subject.DisplayName),
			Group:       strings.TrimSpace(spec.Subject.Group),
		},
		Privilege: strings.TrimSpace(spec.Privilege),
	}
}

func projectWorkspaceDataPolicy(name string, spec workspaceDataPolicySpec) (workspace.WorkspaceDataPolicy, error) {
	expressionJSON := "{}"
	if spec.Expression.Kind != 0 {
		var expression any
		if err := spec.Expression.Decode(&expression); err != nil {
			return workspace.WorkspaceDataPolicy{}, err
		}
		expression = normalizeYAMLValue(expression)
		bytes, err := json.Marshal(expression)
		if err != nil {
			return workspace.WorkspaceDataPolicy{}, err
		}
		expressionJSON = string(bytes)
	}
	return workspace.WorkspaceDataPolicy{
		ID:   name,
		Name: name,
		Object: workspace.WorkspaceSecurableObjectRef{
			Type: strings.TrimSpace(spec.Object.Type),
			ID:   strings.TrimSpace(spec.Object.ID),
		},
		Subject: workspace.WorkspaceRoleBindingSubject{
			Kind:        strings.TrimSpace(spec.Subject.Kind),
			PrincipalID: strings.TrimSpace(spec.Subject.PrincipalID),
			Email:       strings.TrimSpace(spec.Subject.Email),
			DisplayName: strings.TrimSpace(spec.Subject.DisplayName),
			Group:       strings.TrimSpace(spec.Subject.Group),
		},
		PolicyType:     strings.TrimSpace(spec.PolicyType),
		ExpressionJSON: expressionJSON,
	}, nil
}

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeYAMLValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = normalizeYAMLValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeYAMLValue(item)
		}
		return out
	default:
		return value
	}
}

func projectWorkspaceAgentPolicy(name string, spec workspaceAgentPolicySpec) workspace.AgentPolicy {
	return workspace.AgentPolicy{
		ID:      name,
		Name:    name,
		Enabled: spec.Enabled,
		Tools: workspace.AgentPolicyTools{
			Allow: sortedUniqueTrimmed(spec.Tools.Allow),
			Deny:  sortedUniqueTrimmed(spec.Tools.Deny),
		},
		Instructions: strings.TrimSpace(spec.Instructions),
	}
}

func sortedUniqueTrimmed(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (workspaceProject *WorkspaceProject) definition(project Project) (*workspace.Definition, error) {
	sourceAliases := map[string]string{}
	sourceIDs := map[string]string{}
	for source := range workspaceProject.AllowedSources {
		alias := localSourceName(source)
		if existing, exists := sourceIDs[alias]; exists && existing != source {
			return nil, fmt.Errorf("workspace %q sources %q and %q resolve to duplicate runtime alias %q", workspaceProject.ID, existing, source, alias)
		}
		sourceAliases[source] = alias
		sourceIDs[alias] = source
	}
	catalog := workspace.Catalog{
		Workspace: workspace.CatalogWorkspace{
			ID:          workspaceProject.ID,
			Title:       workspaceProject.Title,
			Description: workspaceProject.Description,
		},
		SemanticModels: []workspace.CatalogModel{},
		Dashboards:     []workspace.CatalogDashboard{},
	}
	definition := &workspace.Definition{
		Catalog:    catalog,
		Models:     map[string]*semanticmodel.Model{},
		Dashboards: workspaceProject.Dashboards,
		Access: workspace.AccessPolicy{
			Groups:       copyWorkspaceGroups(workspaceProject.AccessGroups),
			RoleBindings: copyWorkspaceRoleBindings(workspaceProject.AccessRoleBindings),
			Grants:       copyWorkspaceGrants(workspaceProject.AccessGrants),
			DataPolicies: copyWorkspaceDataPolicies(workspaceProject.AccessDataPolicies),
		},
		AgentPolicies: copyAgentPolicies(workspaceProject.AgentPolicies),
		AgentPolicy:   effectiveAgentPolicy(workspaceProject.AgentPolicies),
		BaseDir:       project.BaseDir,
		SourceIDs:     sourceIDs,
		SourceFiles:   workspaceProject.sourceFiles(project),
	}
	for _, modelName := range sortedMapKeys(workspaceProject.SemanticModels) {
		semanticSpec := workspaceProject.SemanticModels[modelName]
		model, err := workspaceProject.semanticModel(project, modelName, semanticSpec, sourceAliases)
		if err != nil {
			return nil, err
		}
		definition.Models[modelName] = model
		definition.Catalog.SemanticModels = append(definition.Catalog.SemanticModels, workspace.CatalogModel{
			ID:          modelName,
			Title:       model.Title,
			Description: model.Description,
		})
	}
	for name := range workspaceProject.Dashboards {
		dashboard := workspaceProject.Dashboards[name]
		if err := ValidateDashboard(dashboard, definition.Models); err != nil {
			return nil, resourceError(workspaceProject.DashboardPaths[name], "dashboard:"+workspaceProject.ID+"."+name, "spec", "loading dashboard %q: %s", name, err.Error())
		}
		definition.Catalog.Dashboards = append(definition.Catalog.Dashboards, workspace.CatalogDashboard{
			ID:          name,
			Title:       firstNonEmpty(workspaceProject.DashboardTitles[name], dashboard.Title),
			Description: workspaceProject.DashboardDescriptions[name],
			Tags:        append([]string{}, workspaceProject.DashboardTags[name]...),
		})
	}
	sort.Slice(definition.Catalog.Dashboards, func(i, j int) bool {
		return definition.Catalog.Dashboards[i].ID < definition.Catalog.Dashboards[j].ID
	})
	return definition, nil
}

func (workspaceProject *WorkspaceProject) sourceFiles(project Project) map[string]string {
	sourceFiles := map[string]string{}
	workspaceKey := func(name string) string {
		return workspaceProject.ID + "." + name
	}
	sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeCatalog, workspaceProject.ID))] = workspaceProject.Path
	for name, path := range project.ConnectionPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeConnection, name))] = path
	}
	for name, path := range project.SourcePaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeSource, name))] = path
	}
	for name, path := range workspaceProject.ModelPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeModelTable, workspaceKey(name)))] = path
	}
	for name, path := range workspaceProject.SemanticModelPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeSemanticModel, workspaceKey(name)))] = path
	}
	for name, path := range workspaceProject.DashboardPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeDashboard, workspaceKey(name)))] = path
	}
	for accessKey, path := range workspaceProject.AccessPaths {
		kind, name, ok := strings.Cut(accessKey, ":")
		if !ok {
			continue
		}
		switch kind {
		case "WorkspaceGroup":
			sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeWorkspaceGroup, workspaceKey(name)))] = path
		case "WorkspaceRoleBinding":
			sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeWorkspaceRoleBinding, workspaceKey(name)))] = path
		}
	}
	for name, path := range workspaceProject.AgentPolicyPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeWorkspaceAgentPolicy, workspaceKey(name)))] = path
	}
	return sourceFiles
}

func (workspaceProject *WorkspaceProject) semanticModel(project Project, modelName string, semanticSpec projectSemanticModelSpec, sourceAliases map[string]string) (*semanticmodel.Model, error) {
	model := &semanticmodel.Model{
		Name:          modelName,
		Title:         firstNonEmpty(workspaceProject.ModelTitles[modelName], modelName),
		Description:   workspaceProject.ModelDescriptions[modelName],
		Connections:   workspaceConnections(project, workspaceProject),
		Sources:       map[string]semanticmodel.Source{},
		Tables:        copyTables(workspaceProject.Models),
		BaseTable:     semanticSpec.BaseTable,
		Relationships: append([]semanticmodel.Relationship{}, semanticSpec.Relationships...),
		Measures:      map[string]semanticmodel.MetricMeasure{},
	}
	model.DefaultConnection = firstConnectionName(model.Connections)
	for source, alias := range sourceAliases {
		model.Sources[alias] = project.Sources[source]
	}
	model.Tables = translatedTablesForRuntime(model.Tables, sourceAliases)
	if err := applySemanticModelSpec(model, semanticSpec); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec", "%s", err.Error())
	}
	if err := model.Validate(); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec", "%s", err.Error())
	}
	if _, err := analyticsmaterialize.ModelTableOrder(model); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec.tables", "%s", err.Error())
	}
	return model, nil
}

func translatedTablesForRuntime(in map[string]semanticmodel.Table, sourceAliases map[string]string) map[string]semanticmodel.Table {
	out := make(map[string]semanticmodel.Table, len(in))
	for name, table := range in {
		if alias, ok := sourceAliases[table.Source]; ok {
			table.Source = alias
		}
		for index, source := range table.Sources {
			if alias, ok := sourceAliases[source]; ok {
				table.Sources[index] = alias
			}
		}
		table.SQL = rewriteSourceSQLForRuntime(table.SQL, sourceAliases)
		table.Transform.SQL = rewriteSourceSQLForRuntime(table.Transform.SQL, sourceAliases)
		out[name] = table
	}
	return out
}

func rewriteSourceSQLForRuntime(sql string, sourceAliases map[string]string) string {
	for global, local := range sourceAliases {
		sql = strings.ReplaceAll(sql, `source."`+global+`"`, "source."+local)
		sql = strings.ReplaceAll(sql, "source."+global, "source."+local)
	}
	return sql
}

func localSourceName(sourceID string) string {
	var builder strings.Builder
	for index, char := range sourceID {
		valid := char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9'
		if valid {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('_')
	}
	out := builder.String()
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "source_" + out
	}
	return out
}

func workspaceConnections(project Project, workspaceProject *WorkspaceProject) map[string]semanticmodel.Connection {
	out := map[string]semanticmodel.Connection{}
	for sourceID := range workspaceProject.AllowedSources {
		source, ok := project.Sources[sourceID]
		if !ok {
			continue
		}
		connection, ok := project.Connections[source.Connection]
		if !ok {
			continue
		}
		out[source.Connection] = connection
	}
	return out
}

func applySemanticModelSpec(model *semanticmodel.Model, spec projectSemanticModelSpec) error {
	if spec.BaseTable == "" {
		return fmt.Errorf("SemanticModel %q requires baseTable", model.Name)
	}
	if len(spec.Tables) == 0 {
		return fmt.Errorf("SemanticModel %q requires tables", model.Name)
	}
	tables := map[string]semanticmodel.Table{}
	for _, tableName := range spec.Tables {
		table, ok := model.Tables[tableName]
		if !ok {
			return fmt.Errorf("SemanticModel %q references unknown ModelTable %q", model.Name, tableName)
		}
		tables[tableName] = table
	}
	defaults := spec.Measures.Defaults
	measures := map[string]semanticmodel.MetricMeasure{}
	for name, measure := range spec.Measures.Items {
		if measure.Expression == "" {
			measure.Expression = measure.Expr
		}
		measure.Table = firstNonEmpty(measure.Table, defaults.Table)
		measure.Grain = firstNonEmpty(measure.Grain, defaults.Grain)
		measure.Time = firstNonEmpty(measure.Time, defaults.Time)
		if len(measure.Grains) == 0 {
			measure.Grains = append([]string{}, defaults.Grains...)
		}
		measure.Field = name
		measure.Name = name
		measures[name] = measure
	}
	model.Tables = tables
	model.BaseTable = spec.BaseTable
	model.Relationships = append([]semanticmodel.Relationship{}, spec.Relationships...)
	model.Measures = measures
	return nil
}

func firstConnectionName(connections map[string]semanticmodel.Connection) string {
	names := make([]string, 0, len(connections))
	for name := range connections {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func copyConnections(in map[string]semanticmodel.Connection) map[string]semanticmodel.Connection {
	out := make(map[string]semanticmodel.Connection, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyTables(in map[string]semanticmodel.Table) map[string]semanticmodel.Table {
	out := make(map[string]semanticmodel.Table, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = append([]string{}, value...)
	}
	return out
}

func copyModelColumns(in map[string]semanticmodel.ModelColumn) map[string]semanticmodel.ModelColumn {
	if in == nil {
		return nil
	}
	out := make(map[string]semanticmodel.ModelColumn, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyMeasures(in map[string]semanticmodel.MetricMeasure) map[string]semanticmodel.MetricMeasure {
	if in == nil {
		return nil
	}
	out := make(map[string]semanticmodel.MetricMeasure, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyWorkspaceGroups(in map[string]workspace.WorkspaceGroup) map[string]workspace.WorkspaceGroup {
	out := make(map[string]workspace.WorkspaceGroup, len(in))
	for key, value := range in {
		value.Members = append([]workspace.WorkspaceGroupMember{}, value.Members...)
		out[key] = value
	}
	return out
}

func copyWorkspaceRoleBindings(in map[string]workspace.WorkspaceRoleBinding) map[string]workspace.WorkspaceRoleBinding {
	out := make(map[string]workspace.WorkspaceRoleBinding, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyWorkspaceGrants(in map[string]workspace.WorkspaceGrant) map[string]workspace.WorkspaceGrant {
	out := make(map[string]workspace.WorkspaceGrant, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyWorkspaceDataPolicies(in map[string]workspace.WorkspaceDataPolicy) map[string]workspace.WorkspaceDataPolicy {
	out := make(map[string]workspace.WorkspaceDataPolicy, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyAgentPolicies(in map[string]workspace.AgentPolicy) map[string]workspace.AgentPolicy {
	out := make(map[string]workspace.AgentPolicy, len(in))
	for key, value := range in {
		value.Tools.Allow = append([]string{}, value.Tools.Allow...)
		value.Tools.Deny = append([]string{}, value.Tools.Deny...)
		out[key] = value
	}
	return out
}

func effectiveAgentPolicy(policies map[string]workspace.AgentPolicy) workspace.AgentPolicy {
	effective := workspace.DefaultAgentPolicy()
	included := sortedMapKeys(policies)
	denySet := map[string]struct{}{}
	var allowSet map[string]struct{}
	var instructions []string
	for _, name := range included {
		policy := policies[name]
		if !policy.Enabled {
			effective.Enabled = false
		}
		if len(policy.Tools.Allow) > 0 {
			next := stringSet(policy.Tools.Allow)
			if allowSet == nil {
				allowSet = next
			} else {
				allowSet = intersectStringSets(allowSet, next)
			}
		}
		for _, tool := range policy.Tools.Deny {
			denySet[tool] = struct{}{}
		}
		if policy.Instructions != "" {
			instructions = append(instructions, policy.Instructions)
		}
	}
	if allowSet != nil {
		for tool := range denySet {
			delete(allowSet, tool)
		}
		effective.Tools.Allow = sortedSetKeys(allowSet)
	}
	effective.Tools.Deny = sortedSetKeys(denySet)
	effective.Instructions = strings.Join(instructions, "\n\n")
	return effective
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func intersectStringSets(left, right map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for value := range left {
		if _, ok := right[value]; ok {
			out[value] = struct{}{}
		}
	}
	return out
}

func accessMemberSortKey(member workspace.WorkspaceGroupMember) string {
	return member.Email + "\x00" + member.PrincipalID + "\x00" + member.DisplayName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStringList(left []string, right []string) bool {
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
