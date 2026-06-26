package compiler

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace"
)

var lineageFieldRefPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\b`)

func ExtractLineage(workspaceID workspace.WorkspaceID, deploymentID workspace.DeploymentID, definition *workspace.Definition) (workspace.AssetGraph, error) {
	graph := workspace.AssetGraph{}
	byKey := map[string]workspace.AssetID{}
	seenEdges := map[string]struct{}{}
	add := func(typ workspace.AssetType, key string, parentID workspace.AssetID, title, description string, payload any) (workspace.AssetID, error) {
		asset, err := workspace.NewAsset(workspaceID, deploymentID, typ, key, parentID, title, description, workspace.PayloadSchemaForAssetType(typ), payload)
		if err != nil {
			return "", err
		}
		graph.Assets = append(graph.Assets, asset)
		byKey[string(typ)+":"+key] = asset.ID
		return asset.ID, nil
	}
	edge := func(fromID, toID workspace.AssetID, typ workspace.AssetEdgeType) {
		if fromID == "" || toID == "" {
			return
		}
		key := string(fromID) + "|" + string(toID) + "|" + string(typ)
		if _, ok := seenEdges[key]; ok {
			return
		}
		seenEdges[key] = struct{}{}
		graph.Edges = append(graph.Edges, workspace.NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ))
	}
	assetID := func(typ workspace.AssetType, key string) (workspace.AssetID, error) {
		id := byKey[string(typ)+":"+key]
		if id == "" {
			return "", fmt.Errorf("missing extracted %s asset %q", typ, key)
		}
		return id, nil
	}

	catalogID, err := add(workspace.AssetTypeCatalog, string(workspaceID), "", workspaceTitle(definition.Catalog.Workspace.Title), definition.Catalog.Workspace.Description, catalogPayload(definition))
	if err != nil {
		return workspace.AssetGraph{}, err
	}
	for _, modelEntry := range definition.Catalog.SemanticModels {
		model := definition.Models[modelEntry.ID]
		for _, connectionName := range sortedMapKeys(model.Connections) {
			connection := model.Connections[connectionName]
			id, err := add(workspace.AssetTypeConnection, modelEntry.ID+"."+connectionName, catalogID, connectionName, connection.Description, connectionPayload(connection))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
		}
		for _, sourceName := range sortedMapKeys(model.Sources) {
			source := model.Sources[sourceName]
			id, err := add(workspace.AssetTypeSource, modelEntry.ID+"."+sourceName, catalogID, sourceName, source.Description, sourcePayload(source))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
			connectionID, err := assetID(workspace.AssetTypeConnection, modelEntry.ID+"."+source.Connection)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, connectionID, workspace.AssetEdgeUsesConnection)
		}
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			id, err := add(workspace.AssetTypeModelTable, modelEntry.ID+"."+tableName, catalogID, tableName, table.Description, modelTablePayload(table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
		}
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			id, err := assetID(workspace.AssetTypeModelTable, modelEntry.ID+"."+tableName)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			for _, sourceName := range table.SourceDependencies {
				sourceID, err := assetID(workspace.AssetTypeSource, modelEntry.ID+"."+sourceName)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, sourceID, workspace.AssetEdgeReadsSource)
			}
			for _, dependency := range table.ModelDependencies {
				dependencyID, err := assetID(workspace.AssetTypeModelTable, modelEntry.ID+"."+dependency)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, dependencyID, workspace.AssetEdgeUsesModelTable)
			}
		}
		modelID, err := add(workspace.AssetTypeSemanticModel, modelEntry.ID, catalogID, modelEntry.Title, modelEntry.Description, semanticModelPayload(model))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, modelID, workspace.AssetEdgeContains)
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			semanticTableID, err := add(workspace.AssetTypeSemanticTable, modelEntry.ID+"."+tableName, modelID, tableName, table.Description, semanticTablePayload(tableName, table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, semanticTableID, workspace.AssetEdgeContains)
			modelTableID, err := assetID(workspace.AssetTypeModelTable, modelEntry.ID+"."+tableName)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(semanticTableID, modelTableID, workspace.AssetEdgeUsesModelTable)
			if table.PrimaryKey != "" {
				field := table.Dimensions[table.PrimaryKey]
				field.Field = tableName + "." + table.PrimaryKey
				field.Table = tableName
				field.Name = table.PrimaryKey
				fieldID, err := add(workspace.AssetTypeField, modelEntry.ID+"."+tableName+"."+table.PrimaryKey, semanticTableID, dimensionLabel(table.PrimaryKey, field.Label), field.Description, fieldPayload(field))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(semanticTableID, fieldID, workspace.AssetEdgeContains)
			}
			for _, fieldName := range sortedMapKeys(table.Dimensions) {
				field := table.Dimensions[fieldName]
				if fieldName == table.PrimaryKey {
					continue
				}
				fieldID, err := add(workspace.AssetTypeField, modelEntry.ID+"."+tableName+"."+fieldName, semanticTableID, dimensionLabel(fieldName, field.Label), field.Description, fieldPayload(field))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(semanticTableID, fieldID, workspace.AssetEdgeContains)
			}
		}
		for _, relationship := range model.Relationships {
			id, err := add(workspace.AssetTypeRelationship, modelEntry.ID+"."+relationship.ID, modelID, relationship.ID, relationship.Description, relationshipPayload(relationship))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			for _, endpoint := range []string{relationship.From, relationship.To} {
				fieldID, err := fieldAssetID(modelEntry.ID, model, endpoint, assetID)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
		for _, measureName := range sortedMapKeys(model.Measures) {
			measure := model.Measures[measureName]
			id, err := add(workspace.AssetTypeMeasure, modelEntry.ID+"."+measureName, modelID, measureLabel(measureName, measure.Label), measure.Description, measurePayload(measure))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			tableID, err := assetID(workspace.AssetTypeSemanticTable, modelEntry.ID+"."+measure.Table)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, tableID, workspace.AssetEdgeUsesSemanticTable)
			for _, ref := range lineageMeasureFieldRefs(model, measure) {
				fieldID, err := assetID(workspace.AssetTypeField, modelEntry.ID+"."+ref)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
	}
	for _, reportEntry := range definition.Catalog.Dashboards {
		report := definition.Dashboards[reportEntry.ID]
		reportID, err := add(workspace.AssetTypeDashboard, reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, dashboardPayload(*report, reportEntry.Tags))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, reportID, workspace.AssetEdgeContains)
		modelID, err := assetID(workspace.AssetTypeSemanticModel, report.SemanticModel)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(reportID, modelID, workspace.AssetEdgeUsesSemanticModel)
		model := definition.Models[report.SemanticModel]
		addSemanticTableUse := func(fromID workspace.AssetID, tableName string) error {
			if tableName == "" {
				return nil
			}
			tableID, err := assetID(workspace.AssetTypeSemanticTable, report.SemanticModel+"."+tableName)
			if err != nil {
				return err
			}
			edge(fromID, tableID, workspace.AssetEdgeUsesSemanticTable)
			return nil
		}
		addMeasureUse := func(fromID workspace.AssetID, ref reportdef.FieldRef) error {
			if ref.Measure.Expression != "" || ref.Measure.Expr != "" {
				if err := addSemanticTableUse(fromID, ref.Measure.Table); err != nil {
					return err
				}
				for _, fieldRef := range lineageExpressionFieldRefs(model, ref.Measure.SQLExpression()) {
					fieldID, err := assetID(workspace.AssetTypeField, report.SemanticModel+"."+fieldRef)
					if err != nil {
						return err
					}
					edge(fromID, fieldID, workspace.AssetEdgeUsesField)
				}
				return nil
			}
			measure, err := model.ResolveMeasure(ref.Field)
			if err != nil {
				return err
			}
			measureID, err := assetID(workspace.AssetTypeMeasure, report.SemanticModel+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, workspace.AssetEdgeUsesMeasure)
			return nil
		}
		addFieldUse := func(fromID workspace.AssetID, ref string, edgeType workspace.AssetEdgeType) error {
			if ref == "" {
				return nil
			}
			if dimension, err := model.ResolveDimension(ref); err == nil {
				fieldID, err := assetID(workspace.AssetTypeField, report.SemanticModel+"."+dimension.Field)
				if err != nil {
					return err
				}
				edge(fromID, fieldID, edgeType)
				return nil
			}
			measure, err := model.ResolveMeasure(ref)
			if err != nil {
				return err
			}
			measureID, err := assetID(workspace.AssetTypeMeasure, report.SemanticModel+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, workspace.AssetEdgeUsesMeasure)
			return nil
		}
		for _, filterName := range sortedMapKeys(report.Filters) {
			filter := report.Filters[filterName]
			filterID, err := add(workspace.AssetTypeFilter, report.ID+"."+filterName, reportID, filter.Label, filter.Description, filterPayload(filter))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, filterID, workspace.AssetEdgeContains)
			if err := addFieldUse(filterID, filter.Dimension, workspace.AssetEdgeFiltersField); err != nil {
				return workspace.AssetGraph{}, err
			}
		}
		for _, visualName := range sortedMapKeys(report.Visuals) {
			visual := report.Visuals[visualName]
			visualID, err := add(workspace.AssetTypeVisual, report.ID+"."+visualName, reportID, visual.Title, visual.Description, visualPayload(visual))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, visualID, workspace.AssetEdgeContains)
			for _, measure := range visual.Query.Measures {
				if err := addMeasureUse(visualID, measure); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, dimension := range visual.Query.Dimensions {
				if err := addFieldUse(visualID, dimension.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			if !visual.Query.Series.IsZero() {
				if err := addFieldUse(visualID, visual.Query.Series.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			if visual.Query.Time.Field != "" {
				if err := addFieldUse(visualID, visual.Query.Time.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
		}
		for _, tableName := range sortedMapKeys(report.Tables) {
			table := report.Tables[tableName]
			tableID, err := add(workspace.AssetTypeTable, report.ID+"."+tableName, reportID, table.Title, table.Description, tableVisualPayload(table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, tableID, workspace.AssetEdgeContains)
			for _, column := range table.DataColumns {
				if err := addFieldUse(tableID, column.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, row := range table.Rows {
				if err := addFieldUse(tableID, row, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, dimension := range table.ColumnDims {
				if err := addFieldUse(tableID, dimension, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, measure := range table.Query.Measures {
				if err := addMeasureUse(tableID, measure); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
		}
		for _, page := range report.Pages {
			pageID, err := add(workspace.AssetTypePage, report.ID+"."+page.ID, reportID, page.Title, page.Description, pagePayload(page))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, pageID, workspace.AssetEdgeContains)
			for index, item := range page.Visuals {
				itemKey := item.ID
				if itemKey == "" {
					itemKey = strconv.Itoa(index)
				}
				itemID, err := add(workspace.AssetTypePageItem, report.ID+"."+page.ID+"."+itemKey, pageID, pageItemTitle(item), item.Description, pageItemPayload(item))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(pageID, itemID, workspace.AssetEdgeContains)
				if item.Visual != "" {
					visualID, err := assetID(workspace.AssetTypeVisual, report.ID+"."+item.Visual)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, visualID, workspace.AssetEdgeUsesVisual)
				}
				if item.Table != "" {
					tableID, err := assetID(workspace.AssetTypeTable, report.ID+"."+item.Table)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, tableID, workspace.AssetEdgeUsesTable)
				}
				if item.Filter != "" {
					filterID, err := assetID(workspace.AssetTypeFilter, report.ID+"."+item.Filter)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, filterID, workspace.AssetEdgeUsesFilter)
				}
			}
		}
	}
	return graph, nil
}

type catalogPayloadV1 struct {
	Workspace      catalogWorkspacePayloadV1    `json:"Workspace"`
	SemanticModels []workspace.CatalogModel     `json:"SemanticModels"`
	Dashboards     []workspace.CatalogDashboard `json:"Dashboards"`
}

type catalogWorkspacePayloadV1 struct {
	ID          string `json:"ID"`
	Title       string `json:"Title"`
	Description string `json:"Description"`
}

type connectionPayloadV1 struct {
	Kind                  string                      `json:"Kind"`
	Path                  string                      `json:"Path"`
	Root                  string                      `json:"Root"`
	Scope                 string                      `json:"Scope"`
	Options               map[string]any              `json:"Options"`
	Defaults              connectionDefaultsPayloadV1 `json:"Defaults"`
	CredentialsConfigured bool                        `json:"credentials_configured"`
}

type connectionDefaultsPayloadV1 struct {
	Options map[string]any `json:"Options"`
}

type sourcePayloadV1 struct {
	Format     string                          `json:"Format"`
	Path       string                          `json:"Path"`
	Connection string                          `json:"Connection"`
	Object     string                          `json:"Object"`
	Options    map[string]any                  `json:"Options"`
	Fields     map[string]sourceFieldPayloadV1 `json:"Fields"`
	Schema     schemaPayloadV1                 `json:"Schema"`
}

type sourceFieldPayloadV1 struct {
	Name        string `json:"Name"`
	Field       string `json:"Field"`
	Table       string `json:"Table"`
	Description string `json:"Description"`
}

type modelTablePayloadV1 struct {
	Kind               string                          `json:"Kind"`
	Source             string                          `json:"Source"`
	Sources            []string                        `json:"Sources"`
	SourceDependencies []string                        `json:"SourceDependencies"`
	ModelDependencies  []string                        `json:"ModelDependencies"`
	Transform          transformPayloadV1              `json:"Transform"`
	SQL                string                          `json:"SQL"`
	PrimaryKey         string                          `json:"PrimaryKey"`
	Grain              string                          `json:"Grain"`
	Dimensions         map[string]fieldPayloadV1       `json:"Dimensions"`
	Fields             map[string]fieldPayloadV1       `json:"Fields"`
	Measures           map[string]measurePayloadV1     `json:"Measures"`
	Columns            map[string]modelColumnPayloadV1 `json:"Columns"`
	Schema             schemaPayloadV1                 `json:"Schema"`
}

type semanticTablePayloadV1 struct {
	Table string `json:"Table"`
	modelTablePayloadV1
}

type transformPayloadV1 struct {
	SQL string `json:"SQL"`
}

type semanticModelPayloadV1 struct {
	Name          string                         `json:"Name"`
	Title         string                         `json:"Title"`
	Description   string                         `json:"Description"`
	BaseTable     string                         `json:"BaseTable"`
	Connections   map[string]connectionPayloadV1 `json:"Connections"`
	Sources       map[string]sourcePayloadV1     `json:"Sources"`
	Tables        map[string]modelTablePayloadV1 `json:"Tables"`
	Models        map[string]modelTablePayloadV1 `json:"Models"`
	Measures      map[string]measurePayloadV1    `json:"Measures"`
	Relationships []relationshipPayloadV1        `json:"Relationships"`
}

type fieldPayloadV1 struct {
	Field       string `json:"Field"`
	Table       string `json:"Table"`
	Name        string `json:"Name"`
	Label       string `json:"Label"`
	Description string `json:"Description"`
	Expr        string `json:"Expr"`
	Expression  string `json:"Expression"`
}

type measurePayloadV1 struct {
	Field       string   `json:"Field"`
	Table       string   `json:"Table"`
	Name        string   `json:"Name"`
	Label       string   `json:"Label"`
	Description string   `json:"Description"`
	Expr        string   `json:"Expr"`
	Expression  string   `json:"Expression"`
	Unit        string   `json:"Unit"`
	Format      string   `json:"Format"`
	Grain       string   `json:"Grain"`
	Time        string   `json:"Time"`
	Grains      []string `json:"Grains"`
}

type relationshipPayloadV1 struct {
	ID          string `json:"ID"`
	Description string `json:"Description"`
	From        string `json:"From"`
	To          string `json:"To"`
	Cardinality string `json:"Cardinality"`
	Active      bool   `json:"Active"`
}

type modelColumnPayloadV1 struct {
	Field       string `json:"Field"`
	Name        string `json:"Name"`
	SourceField string `json:"SourceField"`
	Description string `json:"Description"`
	Type        string `json:"Type"`
}

type schemaPayloadV1 struct {
	Columns []schemaColumnPayloadV1 `json:"Columns"`
}

type schemaColumnPayloadV1 struct {
	Name         string `json:"Name"`
	Ordinal      int    `json:"Ordinal"`
	PhysicalType string `json:"PhysicalType"`
	Nullable     *bool  `json:"Nullable"`
	Default      string `json:"Default"`
	Comment      string `json:"Comment"`
	PrimaryKey   bool   `json:"PrimaryKey"`
}

type dashboardPayloadV1 struct {
	ID            string   `json:"ID"`
	Title         string   `json:"Title"`
	Description   string   `json:"Description"`
	SemanticModel string   `json:"SemanticModel"`
	Tags          []string `json:"Tags"`
}

type filterPayloadV1 struct {
	Type             string                   `json:"Type"`
	Label            string                   `json:"Label"`
	Description      string                   `json:"Description"`
	Dimension        string                   `json:"Dimension"`
	Default          any                      `json:"Default"`
	Custom           bool                     `json:"Custom"`
	Presets          []reportdef.FilterPreset `json:"Presets"`
	Operator         string                   `json:"Operator"`
	Values           reportdef.FilterValues   `json:"Values"`
	DefaultOperator  string                   `json:"DefaultOperator"`
	Operators        []string                 `json:"Operators"`
	Options          []reportdef.FilterOption `json:"Options"`
	URLParam         string                   `json:"URLParam"`
	FromURLParam     string                   `json:"FromURLParam"`
	ToURLParam       string                   `json:"ToURLParam"`
	OperatorURLParam string                   `json:"OperatorURLParam"`
	Targets          reportdef.FilterTargets  `json:"Targets"`
}

type visualPayloadV1 struct {
	Title           string               `json:"Title"`
	Description     string               `json:"Description"`
	Kind            string               `json:"Kind"`
	Shape           string               `json:"Shape"`
	Renderer        string               `json:"Renderer"`
	Type            string               `json:"Type"`
	Query           visualQueryPayloadV1 `json:"Query"`
	Options         map[string]any       `json:"Options"`
	RendererOptions map[string]any       `json:"RendererOptions"`
	Encode          map[string]string    `json:"Encode"`
}

type visualQueryPayloadV1 struct {
	Table      string              `json:"Table"`
	Dimensions []string            `json:"Dimensions"`
	Series     string              `json:"Series"`
	Measures   []string            `json:"Measures"`
	Time       reportdef.QueryTime `json:"Time"`
	Sort       []reportdef.Sort    `json:"Sort"`
	Limit      int                 `json:"Limit"`
}

type tablePayloadV1 struct {
	Title       string               `json:"Title"`
	Description string               `json:"Description"`
	Kind        string               `json:"Kind"`
	Query       tableQueryPayloadV1  `json:"Query"`
	Rows        []string             `json:"Rows"`
	ColumnDims  []string             `json:"ColumnDims"`
	DataColumns []reportdef.FieldRef `json:"DataColumns"`
	Style       dashboard.TableStyle `json:"Style"`
	DefaultSort dashboard.TableSort  `json:"DefaultSort"`
}

type tableQueryPayloadV1 struct {
	Table    string   `json:"Table"`
	Measures []string `json:"Measures"`
}

type pagePayloadV1 struct {
	ID          string               `json:"ID"`
	Title       string               `json:"Title"`
	Description string               `json:"Description"`
	Canvas      dashboard.PageCanvas `json:"Canvas"`
	Grid        dashboard.PageGrid   `json:"Grid"`
}

type pageItemPayloadV1 struct {
	ID          string                  `json:"ID"`
	Kind        string                  `json:"Kind"`
	Visual      string                  `json:"Visual"`
	Table       string                  `json:"Table"`
	Filter      string                  `json:"Filter"`
	Description string                  `json:"Description"`
	Placement   dashboard.PagePlacement `json:"Placement"`
	Title       string                  `json:"Title"`
	Subtitle    string                  `json:"Subtitle"`
	Badges      []string                `json:"Badges"`
}

func catalogPayload(definition *workspace.Definition) catalogPayloadV1 {
	return catalogPayloadV1{
		Workspace: catalogWorkspacePayloadV1{
			ID:          definition.Catalog.Workspace.ID,
			Title:       workspaceTitle(definition.Catalog.Workspace.Title),
			Description: definition.Catalog.Workspace.Description,
		},
		SemanticModels: definition.Catalog.SemanticModels,
		Dashboards:     definition.Catalog.Dashboards,
	}
}

func connectionPayload(connection semanticmodel.Connection) connectionPayloadV1 {
	return connectionPayloadV1{
		Kind:                  connection.Kind,
		Path:                  connection.Path,
		Root:                  connection.Root,
		Scope:                 connection.Scope,
		Options:               connection.Options,
		Defaults:              connectionDefaultsPayloadV1{Options: connection.Defaults.Options},
		CredentialsConfigured: len(connection.Auth) > 0,
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
		out[name] = sourceFieldPayloadV1{Name: field.Name, Field: field.Field, Table: field.Table, Description: field.Description}
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
		Presets:          filter.Presets,
		Operator:         filter.Operator,
		Values:           filter.Values,
		DefaultOperator:  filter.DefaultOperator,
		Operators:        filter.Operators,
		Options:          filter.Options,
		URLParam:         filter.URLParam,
		FromURLParam:     filter.FromURLParam,
		ToURLParam:       filter.ToURLParam,
		OperatorURLParam: filter.OperatorURLParam,
		Targets:          filter.Targets,
	}
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
		Time:       query.Time,
		Sort:       query.Sort,
		Limit:      query.Limit,
	}
}

func fieldRefStrings(refs []reportdef.FieldRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Field)
	}
	return out
}

func tableVisualPayload(table reportdef.TableVisual) map[string]any {
	return map[string]any{
		"Title":       table.Title,
		"Description": table.Description,
		"Kind":        table.KindOrDefault(),
		"Query": map[string]any{
			"Table":    table.Query.Table,
			"Measures": fieldRefStrings(table.Query.Measures),
		},
		"Rows":        table.Rows,
		"ColumnDims":  table.ColumnDims,
		"DataColumns": table.DataColumns,
		"Style":       table.Style,
		"DefaultSort": table.DefaultSort,
	}
}

func pagePayload(page dashboard.Page) map[string]any {
	return map[string]any{
		"ID":          page.ID,
		"Title":       page.Title,
		"Description": page.Description,
		"Canvas":      page.Canvas,
		"Grid":        page.Grid,
	}
}

func pageItemPayload(item dashboard.PageVisual) map[string]any {
	return map[string]any{
		"ID":          item.ID,
		"Kind":        item.Kind,
		"Visual":      item.Visual,
		"Table":       item.Table,
		"Filter":      item.Filter,
		"Description": item.Description,
		"Placement":   item.Placement,
		"Title":       item.Title,
		"Subtitle":    item.Subtitle,
		"Badges":      item.Badges,
	}
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fieldAssetID(modelID string, model *semanticmodel.Model, ref string, assetID func(workspace.AssetType, string) (workspace.AssetID, error)) (workspace.AssetID, error) {
	dimension, err := model.ResolveRelationshipEndpoint(ref)
	if err != nil {
		return "", err
	}
	return assetID(workspace.AssetTypeField, modelID+"."+dimension.Field)
}

func lineageMeasureFieldRefs(model *semanticmodel.Model, measure semanticmodel.MetricMeasure) []string {
	return lineageExpressionFieldRefs(model, measure.SQLExpression())
}

func lineageExpressionFieldRefs(model *semanticmodel.Model, expression string) []string {
	seen := map[string]struct{}{}
	for _, match := range lineageFieldRefPattern.FindAllStringSubmatch(expression, -1) {
		ref := match[1] + "." + match[2]
		dimension, err := model.ResolveRelationshipEndpoint(ref)
		if err != nil {
			continue
		}
		seen[dimension.Field] = struct{}{}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func pageItemTitle(item dashboard.PageVisual) string {
	if item.Title != "" {
		return item.Title
	}
	if item.ID != "" {
		return item.ID
	}
	return item.Kind
}

func dimensionLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}

func measureLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}
