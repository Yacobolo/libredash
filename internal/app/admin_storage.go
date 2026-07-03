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
	"strconv"
	"strings"

	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/ui"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/starfederation/datastar-go/datastar"
)

type adminStorageCommandSignals struct {
	AdminStorageCommand ui.AdminStorageCommand `json:"adminStorageCommand"`
}

const duckLakeStorageCatalogID = "ducklake-catalog"

func (s *Server) adminStorageData(r interface{ Context() context.Context }) ui.AdminStorageData {
	data := ui.AdminStorageData{
		CatalogPath: s.duckLakeCatalogPath,
		DataPath:    s.duckLakeDataPath,
	}
	if strings.TrimSpace(data.CatalogPath) == "" {
		data.Status = "No DuckLake catalog has been initialized."
		return data
	}
	catalogInfo, err := os.Stat(data.CatalogPath)
	if err != nil {
		if os.IsNotExist(err) {
			data.Status = "No DuckLake catalog has been initialized."
		} else {
			data.Status = fmt.Sprintf("DuckLake catalog cannot be read: %v", err)
		}
		return data
	}
	if catalogInfo.IsDir() {
		data.Status = "DuckLake catalog path is a directory."
		return data
	}
	if strings.TrimSpace(data.DataPath) == "" {
		data.Status = "DuckLake data path is not configured."
		return data
	}
	data.CatalogSizeBytes = catalogInfo.Size()
	data.CatalogSizeLabel = formatBytes(data.CatalogSizeBytes)
	if size, err := directorySize(data.DataPath); err == nil {
		data.DataSizeBytes = size
		data.DataSizeLabel = formatBytes(size)
	}
	data.TotalSizeBytes = data.CatalogSizeBytes + data.DataSizeBytes
	data.TotalSizeLabel = formatBytes(data.TotalSizeBytes)
	data.DatabaseCount = 1
	data.Databases = []ui.AdminStorageDatabase{{
		ID:        duckLakeStorageCatalogID,
		Name:      "DuckLake catalog",
		Path:      data.CatalogPath,
		ModelID:   "ducklake",
		ModelName: "DuckLake",
		SizeBytes: data.TotalSizeBytes,
		SizeLabel: data.TotalSizeLabel,
	}}
	metadata, err := inspectDuckLakeStorage(r.Context(), data.CatalogPath, data.DataPath)
	if err != nil {
		data.Status = err.Error()
		return data
	}
	data.Tables = metadata.Tables
	data.Snapshots = metadata.Snapshots
	data.Deployments = metadata.Deployments
	data.Warnings = metadata.Warnings
	data.SnapshotCount = metadata.SnapshotCount
	data.DataFileCount = metadata.DataFileCount
	data.TotalDataSizeBytes = metadata.TotalDataSizeBytes
	data.TotalDataSizeLabel = formatBytes(metadata.TotalDataSizeBytes)
	sort.SliceStable(data.Tables, func(i, j int) bool {
		left := data.Tables[i]
		right := data.Tables[j]
		return strings.Join([]string{left.Schema, left.Name}, "\x00") < strings.Join([]string{right.Schema, right.Name}, "\x00")
	})
	data.TableCount = len(data.Tables)
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
	if command.DatabaseID != duckLakeStorageCatalogID {
		return nil, fmt.Errorf("DuckLake catalog %q was not found", command.DatabaseID)
	}
	if strings.TrimSpace(s.duckLakeCatalogPath) == "" || strings.TrimSpace(s.duckLakeDataPath) == "" {
		return nil, fmt.Errorf("DuckLake catalog is not configured")
	}
	metadata, err := inspectDuckLakeStorage(ctx, s.duckLakeCatalogPath, s.duckLakeDataPath)
	if err != nil {
		return nil, err
	}
	for _, table := range metadata.Tables {
		if table.Schema == command.Schema && table.Name == command.Table {
			selected := ui.AdminStorageTableSignalFromTable(table)
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("DuckLake table %q.%q was not found", command.Schema, command.Table)
}

func adminStorageStreamID(clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "admin-storage:" + clientID
}

type duckLakeStorageMetadata struct {
	Tables             []ui.AdminStorageTable
	Snapshots          []ui.AdminStorageSnapshot
	Deployments        []ui.AdminStorageDeployment
	Warnings           []string
	SnapshotCount      int
	DataFileCount      int
	TotalDataSizeBytes int64
}

func inspectDuckLakeStorage(ctx context.Context, catalogPath, dataPath string) (duckLakeStorageMetadata, error) {
	db, err := openDuckLakeMetadataForInspection(ctx, catalogPath, dataPath)
	if err != nil {
		return duckLakeStorageMetadata{}, fmt.Errorf("DuckLake catalog could not be opened: %w", err)
	}
	defer db.Close()

	deployments, err := inspectDuckLakeDeployments(ctx, db)
	if err != nil {
		return duckLakeStorageMetadata{}, err
	}
	tables, err := inspectDuckLakeTables(ctx, db, deployments)
	if err != nil {
		return duckLakeStorageMetadata{}, err
	}
	for i := range tables {
		tables[i].DatabasePath = catalogPath
	}
	snapshots, err := inspectDuckLakeSnapshots(ctx, db, deployments)
	if err != nil {
		return duckLakeStorageMetadata{}, err
	}
	summary, err := inspectDuckLakeSummary(ctx, db)
	if err != nil {
		return duckLakeStorageMetadata{}, err
	}
	return duckLakeStorageMetadata{
		Tables:             tables,
		Snapshots:          snapshots,
		Deployments:        deployments,
		SnapshotCount:      summary.SnapshotCount,
		DataFileCount:      summary.DataFileCount,
		TotalDataSizeBytes: summary.TotalDataSizeBytes,
	}, nil
}

func openDuckLakeMetadataForInspection(ctx context.Context, catalogPath, dataPath string) (*sql.DB, error) {
	db, err := openDuckDBConnection(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s')", sqlString(catalogPath), sqlString(dataPath)),
		fmt.Sprintf("ATTACH '%s' AS meta (TYPE sqlite)", sqlString(catalogPath)),
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

type duckLakeStorageSummary struct {
	SnapshotCount      int
	DataFileCount      int
	TotalDataSizeBytes int64
}

func inspectDuckLakeSummary(ctx context.Context, db *sql.DB) (duckLakeStorageSummary, error) {
	row := db.QueryRowContext(ctx, `
SELECT
	(SELECT count(*) FROM meta.ducklake_snapshot),
	(SELECT count(*) FROM meta.ducklake_data_file WHERE end_snapshot IS NULL),
	(SELECT coalesce(sum(file_size_bytes), 0) FROM meta.ducklake_data_file WHERE end_snapshot IS NULL)`)
	var summary duckLakeStorageSummary
	if err := row.Scan(&summary.SnapshotCount, &summary.DataFileCount, &summary.TotalDataSizeBytes); err != nil {
		return duckLakeStorageSummary{}, duckLakeMetadataError(err)
	}
	return summary, nil
}

func inspectDuckLakeTables(ctx context.Context, db *sql.DB, deployments []ui.AdminStorageDeployment) ([]ui.AdminStorageTable, error) {
	columns, err := inspectDuckLakeColumns(ctx, db)
	if err != nil {
		return nil, err
	}
	files, err := inspectDuckLakeFiles(ctx, db)
	if err != nil {
		return nil, err
	}
	history, err := inspectDuckLakeTableHistory(ctx, db)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
WITH active_tables AS (
	SELECT s.schema_name, s.path AS schema_path, t.table_name, t.path AS table_path, t.table_id, t.table_uuid, t.begin_snapshot, t.end_snapshot
	FROM meta.ducklake_table t
	JOIN meta.ducklake_schema s ON s.schema_id = t.schema_id
	WHERE t.end_snapshot IS NULL
), file_rollup AS (
	SELECT table_id, count(*) AS file_count, coalesce(sum(record_count), 0) AS row_count, coalesce(sum(file_size_bytes), 0) AS byte_count
	FROM meta.ducklake_data_file
	WHERE end_snapshot IS NULL
	GROUP BY table_id
), column_rollup AS (
	SELECT table_id, count(*) AS column_count
	FROM meta.ducklake_column
	WHERE end_snapshot IS NULL AND parent_column IS NULL
	GROUP BY table_id
)
SELECT a.schema_name, a.schema_path, a.table_name, a.table_path, a.table_id, a.table_uuid, a.begin_snapshot, a.end_snapshot,
       coalesce(f.row_count, 0), coalesce(c.column_count, 0), coalesce(f.file_count, 0), coalesce(f.byte_count, 0)
FROM active_tables a
LEFT JOIN file_rollup f ON f.table_id = a.table_id
LEFT JOIN column_rollup c ON c.table_id = a.table_id
ORDER BY a.schema_name, a.table_name`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()

	var tables []ui.AdminStorageTable
	for rows.Next() {
		var schemaName, schemaPath, tableName, tablePath, tableUUID string
		var tableID, beginSnapshot, rowCount, sizeBytes int64
		var endSnapshot sql.NullInt64
		var columnCount, fileCount int
		if err := rows.Scan(&schemaName, &schemaPath, &tableName, &tablePath, &tableID, &tableUUID, &beginSnapshot, &endSnapshot, &rowCount, &columnCount, &fileCount, &sizeBytes); err != nil {
			return nil, err
		}
		end := int64(0)
		if endSnapshot.Valid {
			end = endSnapshot.Int64
		}
		table := ui.AdminStorageTable{
			DatabaseID:    duckLakeStorageCatalogID,
			DatabaseName:  "DuckLake catalog",
			DatabasePath:  "",
			ModelID:       "ducklake",
			ModelName:     "DuckLake",
			Schema:        schemaName,
			Name:          tableName,
			Type:          "table",
			TableID:       tableID,
			TableUUID:     tableUUID,
			DuckLakePath:  duckLakeTablePath(schemaPath, tablePath),
			BeginSnapshot: beginSnapshot,
			EndSnapshot:   end,
			RowCount:      rowCount,
			RowCountLabel: formatCount(rowCount),
			ColumnCount:   columnCount,
			FileCount:     fileCount,
			SizeBytes:     sizeBytes,
			SizeLabel:     formatBytes(sizeBytes),
			Columns:       columns[tableID],
			Files:         files[tableID],
			History:       history[tableID],
			Deployments:   deploymentsVisibleForTable(deployments, beginSnapshot, end),
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func inspectDuckLakeColumns(ctx context.Context, db *sql.DB) (map[int64][]ui.AdminStorageColumn, error) {
	stats, err := inspectDuckLakeColumnStats(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
SELECT table_id, column_id, column_name, column_type, column_order, nulls_allowed, default_value,
       initial_default, default_value_type, default_value_dialect, begin_snapshot
FROM meta.ducklake_column
WHERE end_snapshot IS NULL AND parent_column IS NULL
ORDER BY table_id, column_order`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()
	columns := map[int64][]ui.AdminStorageColumn{}
	for rows.Next() {
		var tableID int64
		var name, columnType string
		var ordinal int
		var columnID, beginSnapshot int64
		var nullable sql.NullInt64
		var defaultValue, initialDefault, defaultValueType, defaultValueDialect sql.NullString
		if err := rows.Scan(&tableID, &columnID, &name, &columnType, &ordinal, &nullable, &defaultValue, &initialDefault, &defaultValueType, &defaultValueDialect, &beginSnapshot); err != nil {
			return nil, err
		}
		nullableLabel := "-"
		if nullable.Valid {
			if nullable.Int64 == 0 {
				nullableLabel = "No"
			} else {
				nullableLabel = "Yes"
			}
		}
		stat := stats[columnStatsKey(tableID, columnID)]
		columns[tableID] = append(columns[tableID], ui.AdminStorageColumn{
			ID:                  columnID,
			Name:                name,
			Type:                columnType,
			Ordinal:             ordinal,
			Nullable:            nullableLabel,
			Default:             defaultValue.String,
			InitialDefault:      initialDefault.String,
			DefaultValueType:    defaultValueType.String,
			DefaultValueDialect: defaultValueDialect.String,
			BeginSnapshot:       beginSnapshot,
			ContainsNull:        stat.ContainsNull,
			ContainsNaN:         stat.ContainsNaN,
			MinValue:            stat.MinValue,
			MaxValue:            stat.MaxValue,
			ExtraStats:          stat.ExtraStats,
		})
	}
	return columns, rows.Err()
}

type duckLakeColumnStats struct {
	ContainsNull string
	ContainsNaN  string
	MinValue     string
	MaxValue     string
	ExtraStats   string
}

func inspectDuckLakeColumnStats(ctx context.Context, db *sql.DB) (map[string]duckLakeColumnStats, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_id, column_id, contains_null, contains_nan, min_value, max_value, extra_stats
FROM meta.ducklake_table_column_stats`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()
	stats := map[string]duckLakeColumnStats{}
	for rows.Next() {
		var tableID, columnID int64
		var containsNull, containsNaN sql.NullInt64
		var minValue, maxValue, extraStats sql.NullString
		if err := rows.Scan(&tableID, &columnID, &containsNull, &containsNaN, &minValue, &maxValue, &extraStats); err != nil {
			return nil, err
		}
		stats[columnStatsKey(tableID, columnID)] = duckLakeColumnStats{
			ContainsNull: ternaryStatLabel(containsNull),
			ContainsNaN:  ternaryStatLabel(containsNaN),
			MinValue:     minValue.String,
			MaxValue:     maxValue.String,
			ExtraStats:   extraStats.String,
		}
	}
	return stats, rows.Err()
}

func columnStatsKey(tableID, columnID int64) string {
	return strconv.FormatInt(tableID, 10) + "\x00" + strconv.FormatInt(columnID, 10)
}

func duckLakeTablePath(schemaPath, tablePath string) string {
	schemaPath = strings.TrimSpace(schemaPath)
	tablePath = strings.TrimSpace(tablePath)
	if schemaPath == "" {
		return tablePath
	}
	if tablePath == "" {
		return schemaPath
	}
	return strings.TrimRight(schemaPath, "/") + "/" + strings.TrimLeft(tablePath, "/")
}

func ternaryStatLabel(value sql.NullInt64) string {
	if !value.Valid {
		return "-"
	}
	if value.Int64 == 0 {
		return "No"
	}
	return "Yes"
}

func inspectDuckLakeFiles(ctx context.Context, db *sql.DB) (map[int64][]ui.AdminStorageFile, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_id, data_file_id, path, file_format, record_count, file_size_bytes, begin_snapshot, end_snapshot
FROM meta.ducklake_data_file
WHERE end_snapshot IS NULL
ORDER BY table_id, file_order, data_file_id`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()
	files := map[int64][]ui.AdminStorageFile{}
	for rows.Next() {
		var tableID int64
		var file ui.AdminStorageFile
		var endSnapshot sql.NullInt64
		if err := rows.Scan(&tableID, &file.ID, &file.Path, &file.Format, &file.RecordCount, &file.SizeBytes, &file.BeginSnapshot, &endSnapshot); err != nil {
			return nil, err
		}
		if endSnapshot.Valid {
			file.EndSnapshot = endSnapshot.Int64
		}
		file.RecordCountLabel = formatCount(file.RecordCount)
		file.SizeLabel = formatBytes(file.SizeBytes)
		files[tableID] = append(files[tableID], file)
	}
	return files, rows.Err()
}

func inspectDuckLakeTableHistory(ctx context.Context, db *sql.DB) (map[int64][]ui.AdminStorageTableHistory, error) {
	rows, err := db.QueryContext(ctx, `
WITH table_events AS (
	SELECT table_id, begin_snapshot AS snapshot_id, 'table' AS source
	FROM meta.ducklake_table
	UNION ALL
	SELECT table_id, begin_snapshot AS snapshot_id, 'column' AS source
	FROM meta.ducklake_column
	WHERE parent_column IS NULL
	UNION ALL
	SELECT table_id, begin_snapshot AS snapshot_id, 'data_file' AS source
	FROM meta.ducklake_data_file
)
SELECT e.table_id, s.snapshot_id, s.snapshot_time, s.schema_version,
       group_concat(DISTINCT e.source),
       coalesce(c.changes_made, ''), coalesce(c.author, ''), coalesce(c.commit_message, ''), coalesce(c.commit_extra_info, '')
FROM table_events e
JOIN meta.ducklake_snapshot s ON s.snapshot_id = e.snapshot_id
LEFT JOIN meta.ducklake_snapshot_changes c ON c.snapshot_id = s.snapshot_id
GROUP BY e.table_id, s.snapshot_id, s.snapshot_time, s.schema_version, c.changes_made, c.author, c.commit_message, c.commit_extra_info
ORDER BY e.table_id, s.snapshot_id`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()
	history := map[int64][]ui.AdminStorageTableHistory{}
	for rows.Next() {
		var tableID int64
		var event ui.AdminStorageTableHistory
		if err := rows.Scan(&tableID, &event.SnapshotID, &event.Time, &event.SchemaVersion, &event.Source, &event.Changes, &event.Author, &event.Message, &event.ExtraInfo); err != nil {
			return nil, err
		}
		history[tableID] = append(history[tableID], event)
	}
	return history, rows.Err()
}

func inspectDuckLakeSnapshots(ctx context.Context, db *sql.DB, deployments []ui.AdminStorageDeployment) ([]ui.AdminStorageSnapshot, error) {
	rows, err := db.QueryContext(ctx, `
SELECT s.snapshot_id, s.snapshot_time, s.schema_version,
       coalesce(c.changes_made, ''), coalesce(c.author, ''), coalesce(c.commit_message, ''), coalesce(c.commit_extra_info, '')
FROM meta.ducklake_snapshot s
LEFT JOIN meta.ducklake_snapshot_changes c ON c.snapshot_id = s.snapshot_id
ORDER BY s.snapshot_id`)
	if err != nil {
		return nil, duckLakeMetadataError(err)
	}
	defer rows.Close()
	deploymentCounts := map[int64]int{}
	for _, deployment := range deployments {
		if deployment.SnapshotID > 0 && deployment.Status == "active" {
			deploymentCounts[deployment.SnapshotID]++
		}
	}
	var snapshots []ui.AdminStorageSnapshot
	for rows.Next() {
		var snapshot ui.AdminStorageSnapshot
		if err := rows.Scan(&snapshot.ID, &snapshot.Time, &snapshot.SchemaVersion, &snapshot.Changes, &snapshot.Author, &snapshot.Message, &snapshot.ExtraInfo); err != nil {
			return nil, err
		}
		snapshot.DeploymentCount = deploymentCounts[snapshot.ID]
		snapshot.Protected = snapshot.DeploymentCount > 0
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func inspectDuckLakeDeployments(ctx context.Context, db *sql.DB) ([]ui.AdminStorageDeployment, error) {
	rows, err := db.QueryContext(ctx, `
SELECT d.workspace_id, d.environment, d.id, d.status, d.ducklake_snapshot_id, d.digest,
       coalesce(d.activated_at, ''),
       CASE WHEN active.deployment_id IS NOT NULL THEN 1 ELSE 0 END
FROM meta.deployments d
LEFT JOIN meta.workspace_active_deployments active
  ON active.workspace_id = d.workspace_id
 AND active.environment = d.environment
 AND active.deployment_id = d.id
WHERE d.ducklake_snapshot_id > 0
ORDER BY d.workspace_id, d.environment, d.created_at, d.id`)
	if err != nil {
		if isMissingSQLiteTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var deployments []ui.AdminStorageDeployment
	for rows.Next() {
		var deployment ui.AdminStorageDeployment
		var active int
		if err := rows.Scan(&deployment.WorkspaceID, &deployment.Environment, &deployment.DeploymentID, &deployment.Status, &deployment.SnapshotID, &deployment.Digest, &deployment.ActivatedAt, &active); err != nil {
			return nil, err
		}
		deployment.Active = active == 1
		deployments = append(deployments, deployment)
	}
	return deployments, rows.Err()
}

func deploymentsVisibleForTable(deployments []ui.AdminStorageDeployment, beginSnapshot, endSnapshot int64) []ui.AdminStorageDeployment {
	var out []ui.AdminStorageDeployment
	for _, deployment := range deployments {
		if deployment.SnapshotID < beginSnapshot {
			continue
		}
		if endSnapshot > 0 && deployment.SnapshotID >= endSnapshot {
			continue
		}
		out = append(out, deployment)
	}
	return out
}

func duckLakeMetadataError(err error) error {
	if isMissingSQLiteTableError(err) {
		return fmt.Errorf("No DuckLake catalog has been initialized.")
	}
	return err
}

func isMissingSQLiteTableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") || strings.Contains(message, "does not exist")
}

func openDuckDBConnection(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("duckdb", dsn)
	if err == nil {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if pingErr := db.PingContext(ctx); pingErr == nil {
			return db, nil
		} else {
			_ = db.Close()
			err = pingErr
		}
	}
	return nil, err
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

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
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

func sqlString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
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

func formatCount(value int64) string {
	if value < 0 {
		return "-"
	}
	parts := []string{}
	for value >= 1000 {
		parts = append(parts, fmt.Sprintf("%03d", value%1000))
		value /= 1000
	}
	parts = append(parts, strconv.FormatInt(value, 10))
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, ",")
}
