package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/ui"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/starfederation/datastar-go/datastar"
)

type adminStorageCommandSignals struct {
	AdminStorageCommand ui.AdminStorageCommand `json:"adminStorageCommand"`
}

func (s *Server) adminStorageData(r interface{ Context() context.Context }) ui.AdminStorageData {
	data := ui.AdminStorageData{DuckDBDir: s.duckDBDir}
	if strings.TrimSpace(data.DuckDBDir) == "" {
		data.Status = "DuckDB directory is not configured."
		return data
	}
	entries, err := discoverDuckDBFiles(data.DuckDBDir)
	if err != nil {
		data.Status = err.Error()
		return data
	}
	if len(entries) == 0 {
		data.Status = "No DuckDB database files found."
		return data
	}
	data.DatabaseCount = len(entries)
	modelTitles := map[string]string{}
	for _, model := range s.metrics.Catalog().Models {
		modelTitles[model.ID] = model.Title
	}
	for _, entry := range entries {
		data.TotalSizeBytes += entry.SizeBytes
		data.Databases = append(data.Databases, ui.AdminStorageDatabase{
			ID:        entry.ID,
			Name:      entry.Name,
			Path:      entry.Path,
			ModelID:   entry.ModelID,
			ModelName: firstNonEmpty(modelTitles[entry.ModelID], entry.ModelID, "-"),
			SizeBytes: entry.SizeBytes,
			SizeLabel: formatBytes(entry.SizeBytes),
		})
		tables, warning := inspectDuckDBTables(r.Context(), entry, modelTitles)
		if warning != "" {
			data.Warnings = append(data.Warnings, warning)
		}
		data.Tables = append(data.Tables, tables...)
	}
	sort.SliceStable(data.Databases, func(i, j int) bool {
		return data.Databases[i].Name < data.Databases[j].Name
	})
	sort.SliceStable(data.Tables, func(i, j int) bool {
		left := data.Tables[i]
		right := data.Tables[j]
		return strings.Join([]string{left.DatabaseName, left.Schema, left.Name}, "\x00") < strings.Join([]string{right.DatabaseName, right.Schema, right.Name}, "\x00")
	})
	data.TableCount = len(data.Tables)
	data.TotalSizeLabel = formatBytes(data.TotalSizeBytes)
	return data
}

func (s *Server) adminStorageUpdates(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := s.broker.Subscribe(adminStorageStreamID(clientID))
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case patch := <-updates:
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		}
	}
}

func (s *Server) adminStorageSelectTable(w http.ResponseWriter, r *http.Request) {
	clientID := lddatastar.EnsureClientID(w, r)
	signals := adminStorageCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	selectedTable, err := s.adminStorageSelectedTable(r.Context(), signals.AdminStorageCommand)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.broker.Publish(adminStorageStreamID(clientID), map[string]any{
		"adminStorage": map[string]any{
			"selectedKey":   selectedTable.Key,
			"selectedTable": selectedTable,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminStorageSelectedTable(ctx context.Context, command ui.AdminStorageCommand) (*ui.AdminStorageTableSignal, error) {
	if strings.TrimSpace(command.DatabaseID) == "" || strings.TrimSpace(command.Schema) == "" || strings.TrimSpace(command.Table) == "" {
		return nil, fmt.Errorf("storage table selection is incomplete")
	}
	if strings.TrimSpace(s.duckDBDir) == "" {
		return nil, fmt.Errorf("DuckDB directory is not configured")
	}
	entries, err := discoverDuckDBFiles(s.duckDBDir)
	if err != nil {
		return nil, err
	}
	var selectedFile *duckDBFile
	for i := range entries {
		if entries[i].ID == command.DatabaseID {
			selectedFile = &entries[i]
			break
		}
	}
	if selectedFile == nil {
		return nil, fmt.Errorf("DuckDB database %q was not found", command.DatabaseID)
	}
	modelTitles := map[string]string{}
	for _, model := range s.metrics.Catalog().Models {
		modelTitles[model.ID] = model.Title
	}
	table, err := inspectDuckDBTable(ctx, *selectedFile, modelTitles, command.Schema, command.Table)
	if err != nil {
		return nil, err
	}
	selected := ui.AdminStorageTableSignalFromTable(table)
	return &selected, nil
}

func adminStorageStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-storage:" + clientID
}

type duckDBFile struct {
	ID        string
	Name      string
	RelPath   string
	Path      string
	ModelID   string
	SizeBytes int64
}

func discoverDuckDBFiles(root string) ([]duckDBFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("DuckDB directory cannot be read: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("DuckDB path is not a directory.")
	}
	var files []duckDBFile
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".duckdb" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = entry.Name()
		}
		files = append(files, duckDBFile{
			Name:      entry.Name(),
			RelPath:   relPath,
			Path:      path,
			ModelID:   modelIDFromDuckDBFile(entry.Name()),
			SizeBytes: info.Size(),
		})
		return nil
	})
	nameCounts := map[string]int{}
	for _, file := range files {
		nameCounts[file.Name]++
	}
	for i := range files {
		files[i].ID = files[i].Name
		if nameCounts[files[i].Name] > 1 {
			files[i].ID = storageDatabaseID(files[i].RelPath)
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, err
}

func inspectDuckDBTables(ctx context.Context, file duckDBFile, modelTitles map[string]string) ([]ui.AdminStorageTable, string) {
	db, err := openDuckDBForInspection(ctx, file.Path)
	if err != nil {
		return nil, fmt.Sprintf("%s could not be opened: %v", file.Name, err)
	}
	defer db.Close()
	columns, err := inspectDuckDBColumns(ctx, db)
	if err != nil {
		return nil, fmt.Sprintf("%s columns could not be inspected: %v", file.Name, err)
	}
	tables, err := inspectDuckDBObjects(ctx, db, file, modelTitles, columns)
	if err != nil {
		return nil, fmt.Sprintf("%s tables could not be inspected: %v", file.Name, err)
	}
	return tables, ""
}

func openDuckDBForInspection(ctx context.Context, path string) (*sql.DB, error) {
	db, err := openDuckDBConnection(ctx, duckDBReadOnlyDSN(path))
	if err == nil {
		return db, nil
	}
	fallbackDB, fallbackErr := openDuckDBConnection(ctx, path)
	if fallbackErr == nil {
		return fallbackDB, nil
	}
	return nil, errors.Join(err, fallbackErr)
}

func openDuckDBConnection(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("duckdb", dsn)
	if err == nil {
		if pingErr := db.PingContext(ctx); pingErr == nil {
			return db, nil
		} else {
			_ = db.Close()
			err = pingErr
		}
	}
	return nil, err
}

func duckDBReadOnlyDSN(path string) string {
	before, query, hasQuery := strings.Cut(path, "?")
	if !hasQuery {
		return path + "?access_mode=READ_ONLY"
	}
	values := strings.Split(query, "&")
	for i, value := range values {
		key, _, _ := strings.Cut(value, "=")
		if key == "access_mode" {
			values[i] = "access_mode=READ_ONLY"
			return before + "?" + strings.Join(values, "&")
		}
	}
	return path + "&access_mode=READ_ONLY"
}

func inspectDuckDBObjects(ctx context.Context, db *sql.DB, file duckDBFile, modelTitles map[string]string, columns map[string][]ui.AdminStorageColumn) ([]ui.AdminStorageTable, error) {
	rows, err := db.QueryContext(ctx, `
SELECT schema_name, table_name, 'table' AS object_type, column_count
FROM duckdb_tables()
WHERE internal = false AND temporary = false AND schema_name NOT IN ('information_schema', 'pg_catalog')
UNION ALL
SELECT schema_name, view_name AS table_name, 'view' AS object_type, column_count
FROM duckdb_views()
WHERE internal = false AND temporary = false AND schema_name NOT IN ('information_schema', 'pg_catalog')
ORDER BY schema_name, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []ui.AdminStorageTable
	for rows.Next() {
		var schemaName, tableName, objectType string
		var columnCount int
		if err := rows.Scan(&schemaName, &tableName, &objectType, &columnCount); err != nil {
			return nil, err
		}
		rowCount := "-"
		sizeLabel := "Unknown"
		if objectType == "table" {
			if count, err := countDuckDBRows(ctx, db, schemaName, tableName); err == nil {
				rowCount = fmt.Sprint(count)
			}
			if bytes, err := estimateDuckDBTableSize(ctx, db, schemaName, tableName); err == nil {
				sizeLabel = formatBytes(bytes)
			}
		}
		key := storageColumnKey(schemaName, tableName)
		tables = append(tables, ui.AdminStorageTable{
			DatabaseID:    file.ID,
			DatabaseName:  file.Name,
			DatabasePath:  file.Path,
			ModelID:       file.ModelID,
			ModelName:     firstNonEmpty(modelTitles[file.ModelID], file.ModelID, "-"),
			Schema:        schemaName,
			Name:          tableName,
			Type:          objectType,
			RowCountLabel: rowCount,
			ColumnCount:   columnCount,
			SizeLabel:     sizeLabel,
			Columns:       columns[key],
		})
	}
	return tables, rows.Err()
}

func inspectDuckDBTable(ctx context.Context, file duckDBFile, modelTitles map[string]string, schemaName, tableName string) (ui.AdminStorageTable, error) {
	db, err := openDuckDBForInspection(ctx, file.Path)
	if err != nil {
		return ui.AdminStorageTable{}, fmt.Errorf("%s could not be opened: %w", file.Name, err)
	}
	defer db.Close()
	columns, err := inspectDuckDBColumnsForTable(ctx, db, schemaName, tableName)
	if err != nil {
		return ui.AdminStorageTable{}, fmt.Errorf("%s columns could not be inspected: %w", file.Name, err)
	}
	table, err := inspectDuckDBObject(ctx, db, file, modelTitles, schemaName, tableName, columns)
	if err != nil {
		return ui.AdminStorageTable{}, err
	}
	return table, nil
}

func inspectDuckDBObject(ctx context.Context, db *sql.DB, file duckDBFile, modelTitles map[string]string, schemaName, tableName string, columns []ui.AdminStorageColumn) (ui.AdminStorageTable, error) {
	row := db.QueryRowContext(ctx, `
SELECT schema_name, table_name, 'table' AS object_type, column_count
FROM duckdb_tables()
WHERE internal = false AND temporary = false AND schema_name NOT IN ('information_schema', 'pg_catalog') AND schema_name = ? AND table_name = ?
UNION ALL
SELECT schema_name, view_name AS table_name, 'view' AS object_type, column_count
FROM duckdb_views()
WHERE internal = false AND temporary = false AND schema_name NOT IN ('information_schema', 'pg_catalog') AND schema_name = ? AND view_name = ?`, schemaName, tableName, schemaName, tableName)
	var objectSchema, objectName, objectType string
	var columnCount int
	if err := row.Scan(&objectSchema, &objectName, &objectType, &columnCount); err != nil {
		if err == sql.ErrNoRows {
			return ui.AdminStorageTable{}, fmt.Errorf("DuckDB table %q.%q was not found", schemaName, tableName)
		}
		return ui.AdminStorageTable{}, err
	}
	rowCount := "-"
	sizeLabel := "Unknown"
	if objectType == "table" {
		if count, err := countDuckDBRows(ctx, db, objectSchema, objectName); err == nil {
			rowCount = fmt.Sprint(count)
		}
		if bytes, err := estimateDuckDBTableSize(ctx, db, objectSchema, objectName); err == nil {
			sizeLabel = formatBytes(bytes)
		}
	}
	return ui.AdminStorageTable{
		DatabaseID:    file.ID,
		DatabaseName:  file.Name,
		DatabasePath:  file.Path,
		ModelID:       file.ModelID,
		ModelName:     firstNonEmpty(modelTitles[file.ModelID], file.ModelID, "-"),
		Schema:        objectSchema,
		Name:          objectName,
		Type:          objectType,
		RowCountLabel: rowCount,
		ColumnCount:   columnCount,
		SizeLabel:     sizeLabel,
		Columns:       columns,
	}, nil
}

func inspectDuckDBColumns(ctx context.Context, db *sql.DB) (map[string][]ui.AdminStorageColumn, error) {
	rows, err := db.QueryContext(ctx, `
SELECT schema_name, table_name, column_name, column_index, data_type, is_nullable, column_default
FROM duckdb_columns()
WHERE internal = false AND schema_name NOT IN ('information_schema', 'pg_catalog')
ORDER BY schema_name, table_name, column_index`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string][]ui.AdminStorageColumn{}
	for rows.Next() {
		var schemaName, tableName, columnName, dataType string
		var ordinal int
		var nullable sql.NullBool
		var defaultValue sql.NullString
		if err := rows.Scan(&schemaName, &tableName, &columnName, &ordinal, &dataType, &nullable, &defaultValue); err != nil {
			return nil, err
		}
		columns[storageColumnKey(schemaName, tableName)] = append(columns[storageColumnKey(schemaName, tableName)], ui.AdminStorageColumn{
			Name:     columnName,
			Type:     dataType,
			Ordinal:  ordinal,
			Nullable: duckDBNullableLabel(nullable),
			Default:  defaultValue.String,
		})
	}
	return columns, rows.Err()
}

func inspectDuckDBColumnsForTable(ctx context.Context, db *sql.DB, schemaName, tableName string) ([]ui.AdminStorageColumn, error) {
	rows, err := db.QueryContext(ctx, `
SELECT schema_name, table_name, column_name, column_index, data_type, is_nullable, column_default
FROM duckdb_columns()
WHERE internal = false AND schema_name NOT IN ('information_schema', 'pg_catalog') AND schema_name = ? AND table_name = ?
ORDER BY column_index`, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []ui.AdminStorageColumn
	for rows.Next() {
		var ignoredSchema, ignoredTable, columnName, dataType string
		var ordinal int
		var nullable sql.NullBool
		var defaultValue sql.NullString
		if err := rows.Scan(&ignoredSchema, &ignoredTable, &columnName, &ordinal, &dataType, &nullable, &defaultValue); err != nil {
			return nil, err
		}
		columns = append(columns, ui.AdminStorageColumn{
			Name:     columnName,
			Type:     dataType,
			Ordinal:  ordinal,
			Nullable: duckDBNullableLabel(nullable),
			Default:  defaultValue.String,
		})
	}
	return columns, rows.Err()
}

func duckDBNullableLabel(nullable sql.NullBool) string {
	if !nullable.Valid {
		return "-"
	}
	if nullable.Bool {
		return "Yes"
	}
	return "No"
}

func countDuckDBRows(ctx context.Context, db *sql.DB, schemaName, tableName string) (int64, error) {
	query := fmt.Sprintf("SELECT count(*) FROM %s.%s", quoteDuckDBIdentifier(schemaName), quoteDuckDBIdentifier(tableName))
	var count int64
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func estimateDuckDBTableSize(ctx context.Context, db *sql.DB, schemaName, tableName string) (int64, error) {
	tableRef := quoteDuckDBIdentifier(schemaName) + "." + quoteDuckDBIdentifier(tableName)
	query := fmt.Sprintf(`
WITH db AS (
	SELECT block_size FROM pragma_database_size()
), blocks AS (
	SELECT block_id
	FROM pragma_storage_info(%s)
	WHERE persistent AND block_id >= 0
	UNION ALL
	SELECT unnest(additional_block_ids) AS block_id
	FROM pragma_storage_info(%s)
	WHERE persistent AND additional_block_ids IS NOT NULL
)
SELECT count(DISTINCT block_id) * any_value(block_size)
FROM blocks CROSS JOIN db`, sqlStringLiteral(tableRef), sqlStringLiteral(tableRef))
	var bytes sql.NullInt64
	if err := db.QueryRowContext(ctx, query).Scan(&bytes); err != nil {
		return 0, err
	}
	if !bytes.Valid {
		return 0, nil
	}
	return bytes.Int64, nil
}

func modelIDFromDuckDBFile(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return strings.TrimPrefix(base, "libredash-")
}

func storageDatabaseID(relPath string) string {
	return strings.ReplaceAll(filepath.ToSlash(relPath), "/", "~")
}

func storageColumnKey(schemaName, tableName string) string {
	return schemaName + "\x00" + tableName
}

func quoteDuckDBIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func sqlStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func formatBytes(bytes int64) string {
	if bytes < 0 {
		return "-"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
