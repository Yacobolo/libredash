package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dataquery"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func (s *Server) globalDataExplorerState(r *http.Request, command uisignals.DataExplorerCommand) (uisignals.DataExplorerPageSignal, uisignals.DataExplorerSignal, error) {
	return s.globalDataExplorerStateWithCurrent(r, command, nil)
}

func (s *Server) globalDataExplorerStateWithCurrent(r *http.Request, command uisignals.DataExplorerCommand, current *uisignals.DataExplorerSignal) (uisignals.DataExplorerPageSignal, uisignals.DataExplorerSignal, error) {
	command = normalizeDataExplorerCommand(command)
	workspaces, err := s.workspaceList(r)
	if err != nil {
		return uisignals.DataExplorerPageSignal{}, uisignals.DataExplorerSignal{}, err
	}
	sort.SliceStable(workspaces, func(i, j int) bool {
		left := strings.ToLower(firstNonEmpty(workspaces[i].Title, workspaces[i].ID))
		right := strings.ToLower(firstNonEmpty(workspaces[j].Title, workspaces[j].ID))
		if left != right {
			return left < right
		}
		return workspaces[i].ID < workspaces[j].ID
	})
	environment := string(s.requestServingEnvironment(r))
	objects := []uisignals.DataExplorerObjectSignal{}
	warnings := []string{}
	for _, workspace := range workspaces {
		metrics, ok := s.metricsForWorkspace(workspace.ID)
		if !ok || metrics == nil {
			warnings = append(warnings, fmt.Sprintf("Workspace %q metrics are not configured.", workspace.ID))
			continue
		}
		assets, edges, err := s.workspaceAssetsAndEdgesForData(r.Context(), workspace.ID, environment)
		if err != nil {
			fallback := dataExplorerObjectsFromMetrics(workspace.ID, firstNonEmpty(workspace.Title, workspace.ID), metrics)
			if len(fallback) == 0 {
				warnings = append(warnings, fmt.Sprintf("Workspace %q assets are unavailable: %v", workspace.ID, err))
			}
			objects = append(objects, fallback...)
			continue
		}
		workspaceObjects, objectWarnings := dataExplorerObjects(workspace.ID, firstNonEmpty(workspace.Title, workspace.ID), metrics, assets, edges)
		objects = append(objects, workspaceObjects...)
		warnings = append(warnings, objectWarnings...)
	}
	selected, selectionWarnings := selectGlobalDataExplorerObject(objects, command.WorkspaceID, command.ObjectKey)
	warnings = append(warnings, selectionWarnings...)
	if selected != nil {
		command.WorkspaceID = selected.WorkspaceID
		command.ObjectKey = selected.Key
		command.Sort = dataPreviewSortForColumns(selected.Columns, command.Sort)
	}
	explorer := uisignals.DataExplorerSignal{
		Objects:             objects,
		SelectedWorkspaceID: command.WorkspaceID,
		SelectedKey:         command.ObjectKey,
		Command:             command,
		Warnings:            warnings,
		Preview: uisignals.DataPreviewSignal{
			Columns:       []uisignals.DataPreviewColumnSignal{},
			TotalRows:     0,
			AvailableRows: 0,
			ChunkSize:     command.Count,
			RowHeight:     dataExplorerRowHeight,
			ResetVersion:  command.ResetVersion,
			Blocks:        emptyDataPreviewBlocks(command.Count, command.Sort, command.ResetVersion),
			Sort:          command.Sort,
		},
	}
	if selected != nil {
		copy := *selected
		explorer.SelectedObject = &copy
		if metrics, ok := s.metricsForWorkspace(copy.WorkspaceID); ok && metrics != nil {
			explorer.Preview = s.dataPreview(r.Context(), metrics, copy, command, current)
		} else {
			explorer.Preview.Error = fmt.Sprintf("workspace %q metrics are not configured", copy.WorkspaceID)
		}
	}
	page := uisignals.DataExplorerPageSignal{
		Kind:                uisignals.RouteData,
		Title:               "Data Explorer",
		Description:         "Inspect source rows, materialized model tables, and semantic row views.",
		WorkspaceID:         command.WorkspaceID,
		SelectedWorkspaceID: command.WorkspaceID,
		SelectedObject:      command.ObjectKey,
		Workspaces:          dataExplorerWorkspaceSignals(workspaces, objects, command.WorkspaceID),
		Tabs: []uisignals.WorkspaceTabSignal{
			{ID: "all", Label: "All", Href: "/data", Active: true},
		},
	}
	return page, explorer, nil
}

func (s *Server) workspaceAssetsAndEdgesForData(ctx context.Context, workspaceID, environment string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	catalog, ok, err := s.workspaceAssetCatalog(ctx, workspaceID, environment)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		return assetCatalogViews(catalog), assetCatalogEdgeViews(catalog), nil
	}
	return nil, nil, fmt.Errorf("workspace %q assets were not found", workspaceID)
}

func dataExplorerObjects(workspaceID, workspaceTitle string, metrics QueryMetrics, assets []workspace.AssetView, edges []workspace.AssetEdgeView) ([]uisignals.DataExplorerObjectSignal, []string) {
	out := []uisignals.DataExplorerObjectSignal{}
	warnings := []string{}
	for _, asset := range assets {
		modelID, name := keyParts(asset.Key)
		switch asset.Type {
		case string(workspace.AssetTypeSource):
			modelID, sourceKey, source, ok := dataExplorerSourceForAsset(metrics, asset.Key)
			if !ok {
				warnings = append(warnings, fmt.Sprintf("Source %q in workspace %q is not exposed by an active semantic model.", asset.Key, workspaceID))
				continue
			}
			columns := dataColumnsFromSource(source)
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("source", modelID+"."+asset.Key),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        asset.ID,
				Layer:          "source",
				ModelID:        modelID,
				Source:         sourceKey,
				Title:          asset.Title,
				Description:    asset.Description,
				DetailHref:     assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges),
				ColumnCount:    len(columns),
				RowCountLabel:  "Unknown",
				Columns:        columns,
			})
		case string(workspace.AssetTypeModelTable):
			model, _ := metrics.SemanticModel(modelID)
			table := semanticmodel.Table{}
			if model != nil {
				table = model.Tables[name]
			}
			columns := dataColumnsFromTable(table, false)
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("model_table", asset.ID),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        asset.ID,
				Layer:          "model_table",
				ModelID:        modelID,
				Table:          name,
				Title:          asset.Title,
				Description:    asset.Description,
				DetailHref:     assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges),
				ColumnCount:    len(columns),
				RowCountLabel:  "Unknown",
				Columns:        columns,
			})
			semanticColumns := dataColumnsFromTable(table, true)
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("semantic_view", modelID+"."+name),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        asset.ID,
				Layer:          "semantic_view",
				ModelID:        modelID,
				Table:          name,
				Title:          asset.Title + " semantic view",
				Description:    "Exposed row fields from the semantic model.",
				DetailHref:     assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges),
				ColumnCount:    len(semanticColumns),
				RowCountLabel:  "Unknown",
				Columns:        semanticColumns,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Layer != out[j].Layer {
			return dataLayerRank(out[i].Layer) < dataLayerRank(out[j].Layer)
		}
		if out[i].ModelID != out[j].ModelID {
			return out[i].ModelID < out[j].ModelID
		}
		return out[i].Title < out[j].Title
	})
	return out, warnings
}

func dataExplorerSourceForAsset(metrics QueryMetrics, sourceKey string) (string, string, semanticmodel.Source, bool) {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" || metrics == nil {
		return "", "", semanticmodel.Source{}, false
	}
	for _, modelSummary := range metrics.Catalog().Models {
		model, ok := metrics.SemanticModel(modelSummary.ID)
		if !ok || model == nil {
			continue
		}
		source, ok := dataExplorerSourceInModel(model, sourceKey)
		if ok {
			return modelSummary.ID, sourceKey, source, true
		}
	}
	modelID, name := keyParts(sourceKey)
	if modelID == "" || name == "" {
		return "", "", semanticmodel.Source{}, false
	}
	model, ok := metrics.SemanticModel(modelID)
	if !ok || model == nil {
		return "", "", semanticmodel.Source{}, false
	}
	source, ok := model.Sources[name]
	if !ok {
		return "", "", semanticmodel.Source{}, false
	}
	return modelID, name, source, true
}

func dataExplorerSourceInModel(model *semanticmodel.Model, sourceKey string) (semanticmodel.Source, bool) {
	if model == nil {
		return semanticmodel.Source{}, false
	}
	if source, ok := model.Sources[sourceKey]; ok {
		return source, true
	}
	if source, ok := model.Sources[dataExplorerLocalSourceName(sourceKey)]; ok {
		return source, true
	}
	return semanticmodel.Source{}, false
}

func dataExplorerLocalSourceName(sourceID string) string {
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

func dataExplorerObjectsFromMetrics(workspaceID, workspaceTitle string, metrics QueryMetrics) []uisignals.DataExplorerObjectSignal {
	out := []uisignals.DataExplorerObjectSignal{}
	for _, modelSummary := range metrics.Catalog().Models {
		model, ok := metrics.SemanticModel(modelSummary.ID)
		if !ok || model == nil {
			continue
		}
		sourceNames := make([]string, 0, len(model.Sources))
		for name := range model.Sources {
			sourceNames = append(sourceNames, name)
		}
		sort.Strings(sourceNames)
		for _, name := range sourceNames {
			source := model.Sources[name]
			columns := dataColumnsFromSource(source)
			assetID := "source:" + modelSummary.ID + "." + name
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("source", assetID),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        assetID,
				Layer:          "source",
				ModelID:        modelSummary.ID,
				Source:         name,
				Title:          firstNonEmpty(name, assetID),
				ColumnCount:    len(columns),
				RowCountLabel:  "Unknown",
				Columns:        columns,
			})
		}
		tableNames := make([]string, 0, len(model.Tables))
		for name := range model.Tables {
			tableNames = append(tableNames, name)
		}
		sort.Strings(tableNames)
		for _, name := range tableNames {
			table := model.Tables[name]
			assetID := "model_table:" + modelSummary.ID + "." + name
			columns := dataColumnsFromTable(table, false)
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("model_table", assetID),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        assetID,
				Layer:          "model_table",
				ModelID:        modelSummary.ID,
				Table:          name,
				Title:          name,
				ColumnCount:    len(columns),
				RowCountLabel:  "Unknown",
				Columns:        columns,
			})
			semanticColumns := dataColumnsFromTable(table, true)
			out = append(out, uisignals.DataExplorerObjectSignal{
				Key:            dataObjectKey("semantic_view", modelSummary.ID+"."+name),
				WorkspaceID:    workspaceID,
				WorkspaceTitle: workspaceTitle,
				AssetID:        assetID,
				Layer:          "semantic_view",
				ModelID:        modelSummary.ID,
				Table:          name,
				Title:          name + " semantic view",
				Description:    "Exposed row fields from the semantic model.",
				ColumnCount:    len(semanticColumns),
				RowCountLabel:  "Unknown",
				Columns:        semanticColumns,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Layer != out[j].Layer {
			return dataLayerRank(out[i].Layer) < dataLayerRank(out[j].Layer)
		}
		if out[i].ModelID != out[j].ModelID {
			return out[i].ModelID < out[j].ModelID
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func dataExplorerWorkspaceSignals(workspaces []workspace.WorkspaceView, objects []uisignals.DataExplorerObjectSignal, activeWorkspaceID string) []uisignals.DataExplorerWorkspaceSignal {
	counts := map[string]int{}
	for _, object := range objects {
		counts[object.WorkspaceID]++
	}
	out := make([]uisignals.DataExplorerWorkspaceSignal, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, uisignals.DataExplorerWorkspaceSignal{
			ID:          workspace.ID,
			Title:       firstNonEmpty(workspace.Title, workspace.ID),
			Href:        "/data?workspace=" + url.QueryEscape(workspace.ID),
			ObjectCount: counts[workspace.ID],
			Active:      workspace.ID == activeWorkspaceID,
		})
	}
	return out
}

func selectGlobalDataExplorerObject(objects []uisignals.DataExplorerObjectSignal, workspaceID, key string) (*uisignals.DataExplorerObjectSignal, []string) {
	workspaceID = strings.TrimSpace(workspaceID)
	key = strings.TrimSpace(key)
	warnings := []string{}
	if workspaceID != "" && key != "" {
		for i := range objects {
			if objects[i].WorkspaceID == workspaceID && dataExplorerObjectMatchesKey(objects[i], key) {
				return &objects[i], warnings
			}
		}
		warnings = append(warnings, fmt.Sprintf("Data object %q was not found in workspace %q.", key, workspaceID))
	}
	if workspaceID != "" {
		for i := range objects {
			if objects[i].WorkspaceID == workspaceID {
				return &objects[i], warnings
			}
		}
		warnings = append(warnings, fmt.Sprintf("Workspace %q has no inspectable data objects.", workspaceID))
	}
	if key != "" {
		for i := range objects {
			if dataExplorerObjectMatchesKey(objects[i], key) {
				return &objects[i], warnings
			}
		}
		warnings = append(warnings, fmt.Sprintf("Data object %q was not found.", key))
	}
	if len(objects) == 0 {
		return nil, warnings
	}
	return &objects[0], warnings
}

func dataExplorerObjectMatchesKey(object uisignals.DataExplorerObjectSignal, key string) bool {
	if object.Key == key || object.AssetID == key {
		return true
	}
	if object.Layer == "source" && dataObjectKey("source", object.AssetID) == key {
		return true
	}
	return false
}

func (s *Server) dataPreview(ctx context.Context, metrics QueryMetrics, object uisignals.DataExplorerObjectSignal, command uisignals.DataExplorerCommand, current *uisignals.DataExplorerSignal) uisignals.DataPreviewSignal {
	preview := uisignals.DataPreviewSignal{
		Columns:       object.Columns,
		TotalRows:     0,
		AvailableRows: 0,
		ChunkSize:     command.Count,
		RowHeight:     dataExplorerRowHeight,
		ResetVersion:  command.ResetVersion,
		Blocks:        emptyDataPreviewBlocks(command.Count, command.Sort, command.ResetVersion),
		TotalRowLabel: object.RowCountLabel,
		Sort:          command.Sort,
	}
	if totals, ok := reusableDataPreviewTotals(current, object, command); ok {
		preview.TotalRows = totals.TotalRows
		preview.AvailableRows = totals.AvailableRows
		preview.TotalRowLabel = totals.TotalRowLabel
	} else {
		total, err := s.countDataPreview(ctx, metrics, object)
		if err != nil {
			preview.Error = err.Error()
			return preview
		}
		preview.TotalRowLabel = total
		preview.TotalRows = dataPreviewTotalRows(total)
		preview.AvailableRows = preview.TotalRows
	}
	if preview.TotalRows == 0 && preview.TotalRowLabel != "0" {
		preview.TotalRows = command.Start + command.Count*len(dataExplorerBlockIDs)
		preview.AvailableRows = preview.TotalRows
	}
	blockStarts := []int{command.Start}
	blockIDs := []string{command.Block}
	if command.Block == "all" {
		blockStarts = dataPreviewBlockStarts(command.Start, command.Count, preview.AvailableRows)
		blockIDs = dataExplorerBlockIDs[:len(blockStarts)]
	}
	for index, blockID := range blockIDs {
		start := blockStarts[index]
		rows, sqlText, err := s.previewRows(ctx, metrics, object, command, start, command.Count)
		if sqlText != "" {
			preview.SQL = sqlText
		}
		if err != nil {
			preview.Error = err.Error()
			return preview
		}
		if preview.AvailableRows == 0 && len(rows) > 0 {
			preview.AvailableRows = start + len(rows)
			preview.TotalRows = preview.AvailableRows
		}
		preview.Blocks[blockID] = uisignals.DataPreviewBlockSignal{
			Start:        start,
			RequestSeq:   command.RequestSeq,
			ResetVersion: command.ResetVersion,
			Sort:         command.Sort,
			Rows:         rows,
		}
	}
	return preview
}

func reusableDataPreviewTotals(current *uisignals.DataExplorerSignal, object uisignals.DataExplorerObjectSignal, command uisignals.DataExplorerCommand) (uisignals.DataPreviewSignal, bool) {
	if current == nil || current.SelectedObject == nil {
		return uisignals.DataPreviewSignal{}, false
	}
	if current.SelectedObject.WorkspaceID != object.WorkspaceID || current.SelectedObject.Key != object.Key {
		return uisignals.DataPreviewSignal{}, false
	}
	if current.Preview.ResetVersion != command.ResetVersion || current.Preview.ChunkSize != command.Count || current.Preview.Sort != command.Sort {
		return uisignals.DataPreviewSignal{}, false
	}
	if current.Preview.TotalRows <= 0 && current.Preview.AvailableRows <= 0 && dataPreviewTotalRows(current.Preview.TotalRowLabel) <= 0 {
		return uisignals.DataPreviewSignal{}, false
	}
	return current.Preview, true
}

func emptyDataPreviewBlocks(count int, sort uisignals.DataPreviewSortSignal, resetVersion int) map[string]uisignals.DataPreviewBlockSignal {
	if count <= 0 {
		count = dataExplorerDefaultLimit
	}
	return map[string]uisignals.DataPreviewBlockSignal{
		"a": {Start: 0, ResetVersion: resetVersion, Sort: sort, Rows: []map[string]any{}},
		"b": {Start: count, ResetVersion: resetVersion, Sort: sort, Rows: []map[string]any{}},
		"c": {Start: count * 2, ResetVersion: resetVersion, Sort: sort, Rows: []map[string]any{}},
	}
}

func dataPreviewBlockStarts(start, count, availableRows int) []int {
	if count <= 0 {
		count = dataExplorerDefaultLimit
	}
	current := max(0, (start/count)*count)
	starts := []int{}
	if current <= 0 {
		starts = []int{0, count, count * 2}
	} else {
		starts = []int{max(0, current-count), current, current + count}
	}
	out := []int{}
	for _, candidate := range starts {
		if candidate < availableRows {
			out = append(out, candidate)
		}
	}
	return out
}

func dataPreviewTotalRows(label string) int {
	normalized := strings.ReplaceAll(strings.TrimSpace(label), ",", "")
	total, err := strconv.Atoi(normalized)
	if err != nil || total < 0 {
		return 0
	}
	return total
}

func dataPreviewCanceled(preview uisignals.DataPreviewSignal) bool {
	message := strings.ToLower(preview.Error)
	return strings.Contains(message, "context canceled") ||
		strings.Contains(message, "context cancelled") ||
		strings.Contains(message, "interrupt")
}

func (s *Server) countDataPreview(ctx context.Context, metrics QueryMetrics, object uisignals.DataExplorerObjectSignal) (string, error) {
	switch object.Layer {
	case "source", "model_table":
		result, err := metrics.ExecuteDataQuery(ctx, dataPreviewQuery(object, uisignals.DataExplorerCommand{}, 0, 1, true))
		if err != nil {
			return "Unknown", err
		}
		if !result.TotalRowsKnown {
			return "Unknown", nil
		}
		return strconv.Itoa(result.TotalRows), nil
	case "semantic_view":
		return firstNonEmpty(object.RowCountLabel, "Unknown"), nil
	default:
		return "Unknown", fmt.Errorf("unsupported data layer %q", object.Layer)
	}
}

func (s *Server) previewRows(ctx context.Context, metrics QueryMetrics, object uisignals.DataExplorerObjectSignal, command uisignals.DataExplorerCommand, start, count int) ([]map[string]any, string, error) {
	result, err := metrics.ExecuteDataQuery(ctx, dataPreviewQuery(object, command, start, count, false))
	if err != nil {
		return nil, "", err
	}
	return dataRowsFromQuery(result.Rows), result.SQL, nil
}

func dataPreviewColumnKeys(columns []uisignals.DataPreviewColumnSignal) []string {
	keys := make([]string, 0, len(columns))
	for _, column := range columns {
		if strings.TrimSpace(column.Key) != "" {
			keys = append(keys, column.Key)
		}
	}
	return keys
}

func dataPreviewQuery(object uisignals.DataExplorerObjectSignal, command uisignals.DataExplorerCommand, start, count int, includeTotal bool) dataquery.Query {
	columns := dataPreviewColumnKeys(object.Columns)
	sortSpec := dataPreviewSort(command.Sort)
	metadata := dataquery.Metadata{
		Surface:    dataquery.SurfaceDataExplorer,
		Operation:  dataquery.OperationPreviewWindow,
		ObjectType: object.Layer,
		ObjectID:   object.WorkspaceID + ":" + object.Key,
	}
	withMetadata := func(query dataquery.Query) dataquery.Query {
		query.WorkspaceID = object.WorkspaceID
		return query.WithMetadata(metadata)
	}
	switch object.Layer {
	case "source":
		return withMetadata(dataquery.SourceRows(object.ModelID, object.Source, columns, sortSpec, start, count, includeTotal))
	case "model_table":
		return withMetadata(dataquery.ModelTableRows(object.ModelID, object.Table, columns, sortSpec, start, count, includeTotal))
	case "semantic_view":
		fields := make([]dataquery.Field, 0, len(columns))
		for _, column := range columns {
			fields = append(fields, dataquery.Field{Field: object.Table + "." + column, Alias: column})
		}
		return withMetadata(dataquery.SemanticRows(object.ModelID, object.Table, fields, nil, nil, sortSpec, start, count, includeTotal))
	default:
		return withMetadata(dataquery.Query{ModelID: object.ModelID, Kind: dataquery.Kind(object.Layer), Target: object.Table, Limit: count, Offset: start, IncludeTotal: includeTotal})
	}
}

func dataPreviewSort(sort uisignals.DataPreviewSortSignal) []dataquery.Sort {
	if sort.Column == "" {
		return nil
	}
	return []dataquery.Sort{{Field: sort.Column, Direction: sort.Direction}}
}

func dataRowsFromQuery(rows []dataquery.Row) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		converted := map[string]any{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
}

func dataPreviewSortForColumns(columns []uisignals.DataPreviewColumnSignal, sort uisignals.DataPreviewSortSignal) uisignals.DataPreviewSortSignal {
	if sort.Column == "" || !dataColumnExists(columns, sort.Column) {
		return uisignals.DataPreviewSortSignal{}
	}
	if sort.Direction != "asc" && sort.Direction != "desc" {
		return uisignals.DataPreviewSortSignal{}
	}
	return sort
}

func dataColumnsFromSource(source semanticmodel.Source) []uisignals.DataPreviewColumnSignal {
	if len(source.Schema.Columns) > 0 {
		return dataColumnsFromSchema(source.Schema.Columns)
	}
	names := make([]string, 0, len(source.Fields))
	for name := range source.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]uisignals.DataPreviewColumnSignal, 0, len(names))
	for _, name := range names {
		field := source.Fields[name]
		out = append(out, uisignals.DataPreviewColumnSignal{Key: name, Label: name, Type: field.Type})
	}
	return out
}

func dataColumnsFromTable(table semanticmodel.Table, semanticOnly bool) []uisignals.DataPreviewColumnSignal {
	if semanticOnly {
		names := make([]string, 0, len(table.Dimensions))
		for name := range table.Dimensions {
			names = append(names, name)
		}
		sort.Strings(names)
		out := make([]uisignals.DataPreviewColumnSignal, 0, len(names))
		for _, name := range names {
			dimension := table.Dimensions[name]
			out = append(out, uisignals.DataPreviewColumnSignal{Key: name, Label: firstNonEmpty(dimension.Label, name), Type: dimension.Type})
		}
		return out
	}
	if len(table.Schema.Columns) > 0 {
		return dataColumnsFromSchema(table.Schema.Columns)
	}
	names := make([]string, 0, len(table.Columns))
	for name := range table.Columns {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]uisignals.DataPreviewColumnSignal, 0, len(names))
	for _, name := range names {
		column := table.Columns[name]
		out = append(out, uisignals.DataPreviewColumnSignal{Key: name, Label: firstNonEmpty(column.Name, name), Type: column.Type})
	}
	return out
}

func dataColumnsFromSchema(columns []semanticmodel.ColumnSchema) []uisignals.DataPreviewColumnSignal {
	out := make([]uisignals.DataPreviewColumnSignal, 0, len(columns))
	sorted := append([]semanticmodel.ColumnSchema{}, columns...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Ordinal < sorted[j].Ordinal
	})
	for _, column := range sorted {
		out = append(out, uisignals.DataPreviewColumnSignal{Key: column.Name, Label: column.Name, Type: column.PhysicalType})
	}
	return out
}

func dataColumnExists(columns []uisignals.DataPreviewColumnSignal, key string) bool {
	for _, column := range columns {
		if column.Key == key {
			return true
		}
	}
	return false
}

func dataObjectKey(layer, id string) string {
	return layer + ":" + id
}

func dataLayerRank(layer string) int {
	switch layer {
	case "source":
		return 0
	case "model_table":
		return 1
	case "semantic_view":
		return 2
	default:
		return 10
	}
}

func keyParts(key string) (string, string) {
	left, right, ok := strings.Cut(key, ".")
	if !ok {
		return "", key
	}
	return left, right
}
