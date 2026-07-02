package compiler

import (
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func catalogPayload(definition *workspace.Definition) catalogPayloadV1 {
	return catalogPayloadV1{
		Workspace: catalogWorkspacePayloadV1{
			ID:          definition.Catalog.Workspace.ID,
			Title:       workspaceTitle(definition.Catalog.Workspace.Title, definition.Catalog.Workspace.ID),
			Description: definition.Catalog.Workspace.Description,
		},
		SemanticModels: catalogModelsPayload(definition.Catalog.SemanticModels),
		Dashboards:     catalogDashboardsPayload(definition.Catalog.Dashboards),
	}
}

func catalogModelsPayload(models []workspace.CatalogModel) []catalogModelPayloadV1 {
	out := make([]catalogModelPayloadV1, 0, len(models))
	for _, model := range models {
		out = append(out, catalogModelPayloadV1{
			ID:          model.ID,
			Title:       model.Title,
			Path:        model.Path,
			Description: model.Description,
		})
	}
	return out
}

func catalogDashboardsPayload(dashboards []workspace.CatalogDashboard) []catalogDashboardPayloadV1 {
	out := make([]catalogDashboardPayloadV1, 0, len(dashboards))
	for _, dashboard := range dashboards {
		out = append(out, catalogDashboardPayloadV1{
			ID:          dashboard.ID,
			Title:       dashboard.Title,
			Path:        dashboard.Path,
			Description: dashboard.Description,
			Tags:        dashboard.Tags,
		})
	}
	return out
}

func connectionPayload(connection semanticmodel.Connection) connectionPayloadV1 {
	return connectionPayloadV1{
		Kind:                  connection.Kind,
		Path:                  connection.Path,
		Root:                  connection.Root,
		Scope:                 connection.Scope,
		Host:                  connection.Host,
		Port:                  connection.Port,
		Database:              connection.Database,
		Username:              connection.Username,
		SSLMode:               connection.SSLMode,
		Options:               connection.Options,
		Defaults:              connectionDefaultsPayloadV1{Options: connection.Defaults.Options},
		CredentialsConfigured: semanticmodel.ConnectionCredentialsConfigured(connection),
	}
}

func sourcePayload(source semanticmodel.Source) sourcePayloadV1 {
	return sourcePayloadV1{
		Format:     source.Format,
		Path:       source.Path,
		Connection: source.Connection,
		Object:     source.Object,
		Options:    source.Options,
		Fields:     sourceFieldsPayload(source.Fields),
		Schema:     schemaPayload(source.Schema),
	}
}

func sourceFieldsPayload(fields map[string]semanticmodel.SourceField) map[string]sourceFieldPayloadV1 {
	out := map[string]sourceFieldPayloadV1{}
	for _, name := range sortedMapKeys(fields) {
		field := fields[name]
		out[name] = sourceFieldPayloadV1{Name: field.Name, Field: field.Field, Table: field.Table, Type: field.Type, Description: field.Description}
	}
	return out
}

func modelTablePayload(table semanticmodel.Table) modelTablePayloadV1 {
	dimensions := dimensionsPayload(table.Dimensions)
	return modelTablePayloadV1{
		Kind:               table.Kind,
		Source:             table.Source,
		Sources:            table.Sources,
		SourceDependencies: table.SourceDependencies,
		ModelDependencies:  table.ModelDependencies,
		Transform:          transformPayloadV1{SQL: table.Transform.SQL},
		SQL:                table.SQL,
		PrimaryKey:         table.PrimaryKey,
		Grain:              table.Grain,
		Dimensions:         dimensions,
		Fields:             dimensions,
		Measures:           measuresPayload(table.Measures),
		Columns:            columnsPayload(table.Columns),
		Schema:             schemaPayload(table.Schema),
	}
}

func semanticModelPayload(model *semanticmodel.Model) semanticModelPayloadV1 {
	connections := map[string]connectionPayloadV1{}
	for _, name := range sortedMapKeys(model.Connections) {
		connections[name] = connectionPayload(model.Connections[name])
	}
	sources := map[string]sourcePayloadV1{}
	for _, name := range sortedMapKeys(model.Sources) {
		sources[name] = sourcePayload(model.Sources[name])
	}
	tables := map[string]modelTablePayloadV1{}
	for _, name := range sortedMapKeys(model.Tables) {
		tables[name] = modelTablePayload(model.Tables[name])
	}
	return semanticModelPayloadV1{
		Name:          model.Name,
		Title:         model.Title,
		Description:   model.Description,
		BaseTable:     model.BaseTable,
		Connections:   connections,
		Sources:       sources,
		Tables:        tables,
		Models:        tables,
		Measures:      measuresPayload(model.Measures),
		Relationships: relationshipsPayload(model.Relationships),
	}
}

func semanticTablePayload(name string, table semanticmodel.Table) semanticTablePayloadV1 {
	return semanticTablePayloadV1{Table: name, modelTablePayloadV1: modelTablePayload(table)}
}

func fieldPayload(field semanticmodel.MetricDimension) fieldPayloadV1 {
	return fieldPayloadV1{
		Field:       field.Field,
		Table:       field.Table,
		Name:        field.Name,
		Label:       field.Label,
		Description: field.Description,
		Type:        field.Type,
		Expr:        field.Expr,
		Expression:  field.Expression,
	}
}

func dimensionsPayload(fields map[string]semanticmodel.MetricDimension) map[string]fieldPayloadV1 {
	out := map[string]fieldPayloadV1{}
	for _, name := range sortedMapKeys(fields) {
		out[name] = fieldPayload(fields[name])
	}
	return out
}

func measurePayload(measure semanticmodel.MetricMeasure) measurePayloadV1 {
	return measurePayloadV1{
		Field:       measure.Field,
		Table:       measure.Table,
		Name:        measure.Name,
		Label:       measure.Label,
		Description: measure.Description,
		Expr:        measure.Expr,
		Expression:  measure.SQLExpression(),
		Unit:        measure.Unit,
		Format:      measure.Format,
		Grain:       measure.Grain,
		Time:        measure.Time,
		Grains:      measure.Grains,
	}
}

func measuresPayload(measures map[string]semanticmodel.MetricMeasure) map[string]measurePayloadV1 {
	out := map[string]measurePayloadV1{}
	for _, name := range sortedMapKeys(measures) {
		out[name] = measurePayload(measures[name])
	}
	return out
}

func relationshipPayload(relationship semanticmodel.Relationship) relationshipPayloadV1 {
	return relationshipPayloadV1{
		ID:          relationship.ID,
		Description: relationship.Description,
		From:        relationship.From,
		To:          relationship.To,
		Cardinality: relationship.Cardinality,
		Active:      relationship.Active,
	}
}

func relationshipsPayload(relationships []semanticmodel.Relationship) []relationshipPayloadV1 {
	out := make([]relationshipPayloadV1, 0, len(relationships))
	for _, relationship := range relationships {
		out = append(out, relationshipPayload(relationship))
	}
	return out
}

func columnsPayload(columns map[string]semanticmodel.ModelColumn) map[string]modelColumnPayloadV1 {
	out := map[string]modelColumnPayloadV1{}
	for _, name := range sortedMapKeys(columns) {
		column := columns[name]
		out[name] = modelColumnPayloadV1{
			Field:       column.Field,
			Name:        column.Name,
			SourceField: column.SourceField,
			Description: column.Description,
			Type:        column.Type,
		}
	}
	return out
}

func schemaPayload(schema semanticmodel.TableSchema) schemaPayloadV1 {
	columns := make([]schemaColumnPayloadV1, 0, len(schema.Columns))
	for _, column := range schema.Columns {
		columns = append(columns, schemaColumnPayloadV1{
			Name:         column.Name,
			Ordinal:      column.Ordinal,
			PhysicalType: column.PhysicalType,
			Nullable:     column.Nullable,
			Default:      column.Default,
			Comment:      column.Comment,
			PrimaryKey:   column.PrimaryKey,
		})
	}
	return schemaPayloadV1{Columns: columns}
}

func dashboardPayload(report reportdef.Dashboard, tags []string) dashboardPayloadV1 {
	return dashboardPayloadV1{ID: report.ID, Title: report.Title, Description: report.Description, SemanticModel: report.SemanticModel, Tags: tags}
}

func filterPayload(filter reportdef.FilterDefinition) filterPayloadV1 {
	return filterPayloadV1{
		Type:             filter.Type,
		Label:            filter.Label,
		Description:      filter.Description,
		Dimension:        filter.Dimension,
		Default:          filter.Default,
		Custom:           filter.Custom,
		Presets:          filterPresetsPayload(filter.Presets),
		Operator:         filter.Operator,
		Values:           filterValuesPayload(filter.Values),
		DefaultOperator:  filter.DefaultOperator,
		Operators:        filter.Operators,
		Options:          filterOptionsPayload(filter.Options),
		URLParam:         filter.URLParam,
		FromURLParam:     filter.FromURLParam,
		ToURLParam:       filter.ToURLParam,
		OperatorURLParam: filter.OperatorURLParam,
		Targets:          filterTargetsPayload(filter.Targets),
	}
}

func filterPresetsPayload(presets []reportdef.FilterPreset) []filterPresetPayloadV1 {
	out := make([]filterPresetPayloadV1, 0, len(presets))
	for _, preset := range presets {
		out = append(out, filterPresetPayloadV1{
			Value:        preset.Value,
			Label:        preset.Label,
			From:         preset.From,
			To:           preset.To,
			RelativeDays: preset.RelativeDays,
		})
	}
	return out
}

func filterValuesPayload(values reportdef.FilterValues) filterValuesPayloadV1 {
	return filterValuesPayloadV1{Source: values.Source, Limit: values.Limit}
}

func filterOptionsPayload(options []reportdef.FilterOption) []filterOptionPayloadV1 {
	out := make([]filterOptionPayloadV1, 0, len(options))
	for _, option := range options {
		out = append(out, filterOptionPayloadV1{Value: option.Value, Label: option.Label})
	}
	return out
}

func filterTargetsPayload(targets reportdef.FilterTargets) filterTargetsPayloadV1 {
	return filterTargetsPayloadV1{Visuals: targets.Visuals, Tables: targets.Tables}
}

func visualPayload(visual reportdef.Visual) visualPayloadV1 {
	return visualPayloadV1{
		Title:           visual.Title,
		Description:     visual.Description,
		Kind:            visual.KindOrDefault(),
		Shape:           visual.ShapeOrDefault(),
		Renderer:        visual.RendererOrDefault(),
		Type:            visual.Type,
		Query:           visualQueryPayload(visual.Query),
		Options:         visual.CoreOptions(),
		RendererOptions: visual.RendererOptions,
		Encode:          visual.Encode,
	}
}

func visualQueryPayload(query reportdef.VisualQuery) visualQueryPayloadV1 {
	return visualQueryPayloadV1{
		Table:      query.Table,
		Dimensions: fieldRefStrings(query.Dimensions),
		Series:     query.Series.Field,
		Measures:   fieldRefStrings(query.Measures),
		Time:       queryTimePayload(query.Time),
		Sort:       sortPayload(query.Sort),
		Limit:      query.Limit,
	}
}

func queryTimePayload(time reportdef.QueryTime) queryTimePayloadV1 {
	return queryTimePayloadV1{Field: time.Field, Grain: time.Grain, Alias: time.Alias}
}

func sortPayload(sort []reportdef.Sort) []sortPayloadV1 {
	out := make([]sortPayloadV1, 0, len(sort))
	for _, entry := range sort {
		out = append(out, sortPayloadV1{Field: entry.Field, Direction: entry.Direction, Expr: entry.Expr})
	}
	return out
}

func fieldRefStrings(refs []reportdef.FieldRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Field)
	}
	return out
}

func fieldRefsPayload(refs []reportdef.FieldRef) []fieldRefPayloadV1 {
	out := make([]fieldRefPayloadV1, 0, len(refs))
	for _, ref := range refs {
		out = append(out, fieldRefPayloadV1{Field: ref.Field, Alias: ref.Alias})
	}
	return out
}

func tableVisualPayload(table reportdef.TableVisual) tablePayloadV1 {
	return tablePayloadV1{
		Title:       table.Title,
		Description: table.Description,
		Kind:        table.KindOrDefault(),
		Query: tableQueryPayloadV1{
			Table:    table.Query.Table,
			Measures: fieldRefStrings(table.Query.Measures),
		},
		Rows:        table.Rows,
		ColumnDims:  table.ColumnDims,
		DataColumns: fieldRefsPayload(table.DataColumns),
		Style:       tableStylePayload(table.Style),
		DefaultSort: tableSortPayload(table.DefaultSort),
	}
}

func tableStylePayload(style dashboard.TableStyle) tableStylePayloadV1 {
	return tableStylePayloadV1{Density: style.Density, Zebra: style.Zebra, Grid: style.Grid}
}

func tableSortPayload(sort dashboard.TableSort) tableSortPayloadV1 {
	return tableSortPayloadV1{Key: sort.Key, Direction: sort.Direction}
}

func pagePayload(page dashboard.Page) pagePayloadV1 {
	return pagePayloadV1{
		ID:          page.ID,
		Title:       page.Title,
		Description: page.Description,
		Canvas:      pageCanvasPayload(page.Canvas),
		Grid:        pageGridPayload(page.Grid),
	}
}

func workspaceGroupPayload(group workspace.WorkspaceGroup) workspaceGroupPayloadV1 {
	members := make([]workspaceGroupMemberPayloadV1, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, workspaceGroupMemberPayloadV1{
			PrincipalID: member.PrincipalID,
			Email:       member.Email,
			DisplayName: member.DisplayName,
		})
	}
	return workspaceGroupPayloadV1{
		ID:          group.ID,
		Name:        group.Name,
		Description: group.Description,
		Members:     members,
	}
}

func workspaceRoleBindingPayload(binding workspace.WorkspaceRoleBinding) workspaceRoleBindingPayloadV1 {
	return workspaceRoleBindingPayloadV1{
		ID:   binding.ID,
		Name: binding.Name,
		Role: binding.Role,
		Subject: workspaceRoleBindingSubjectPayloadV1{
			Kind:        binding.Subject.Kind,
			PrincipalID: binding.Subject.PrincipalID,
			Email:       binding.Subject.Email,
			DisplayName: binding.Subject.DisplayName,
			Group:       binding.Subject.Group,
		},
	}
}

func workspaceAgentPolicyPayload(policy workspace.AgentPolicy) workspaceAgentPolicyPayloadV1 {
	return workspaceAgentPolicyPayloadV1{
		ID:      policy.ID,
		Name:    policy.Name,
		Enabled: policy.Enabled,
		Tools: workspaceAgentPolicyToolsPayloadV1{
			Allow: append([]string{}, policy.Tools.Allow...),
			Deny:  append([]string{}, policy.Tools.Deny...),
		},
		Instructions: policy.Instructions,
	}
}

func pageCanvasPayload(canvas dashboard.PageCanvas) pageCanvasV1 {
	return pageCanvasV1{Width: canvas.Width, Height: canvas.Height}
}

func pageGridPayload(grid dashboard.PageGrid) pageGridV1 {
	return pageGridV1{Columns: grid.Columns, RowHeight: grid.RowHeight, Gap: grid.Gap, Padding: grid.Padding}
}

func pageItemPayload(item dashboard.PageVisual) pageItemPayloadV1 {
	return pageItemPayloadV1{
		ID:          item.ID,
		Kind:        item.Kind,
		Visual:      item.Visual,
		Table:       item.Table,
		Filter:      item.Filter,
		Description: item.Description,
		Placement:   pagePlacementPayload(item.Placement),
		Title:       item.Title,
		Subtitle:    item.Subtitle,
		Badges:      item.Badges,
	}
}

func pagePlacementPayload(placement dashboard.PagePlacement) pagePlacementV1 {
	return pagePlacementV1{
		Col:     placement.Col,
		Row:     placement.Row,
		ColSpan: placement.ColSpan,
		RowSpan: placement.RowSpan,
	}
}
