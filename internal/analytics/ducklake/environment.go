package ducklake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

const catalogAlias = "lake"

type Config struct {
	RootDir     string
	CatalogPath string
	DataPath    string
	SnapshotID  int64
}

type Layout struct {
	RootDir          string
	CatalogPath      string
	DataPath         string
	LegacyDuckDBPath string
}

type Environment struct {
	db     *sql.DB
	layout Layout
}

type Snapshot struct {
	ID int64
}

func NewLayout(rootDir string) Layout {
	return Layout{
		RootDir:          rootDir,
		CatalogPath:      filepath.Join(rootDir, "catalog.sqlite"),
		DataPath:         filepath.Join(rootDir, "data"),
		LegacyDuckDBPath: filepath.Join(rootDir, "libredash-workspace.duckdb"),
	}
}

func Open(ctx context.Context, config Config) (*Environment, error) {
	return open(ctx, config, false)
}

func OpenSnapshot(ctx context.Context, config Config) (*Environment, error) {
	if config.SnapshotID < 0 {
		return nil, fmt.Errorf("snapshot id must be non-negative")
	}
	return open(ctx, config, true)
}

func open(ctx context.Context, config Config, snapshot bool) (*Environment, error) {
	layout, err := config.layout()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(layout.RootDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(layout.DataPath, 0o755); err != nil {
		return nil, err
	}
	if err := MigrateSQLiteCatalogDataPath(ctx, layout.CatalogPath, layout.DataPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	env := &Environment{db: db, layout: layout}
	if err := env.initialize(ctx, snapshot, config.SnapshotID); err != nil {
		db.Close()
		return nil, err
	}
	return env, nil
}

func (c Config) layout() (Layout, error) {
	root := strings.TrimSpace(c.RootDir)
	if root == "" {
		if c.CatalogPath == "" && c.DataPath == "" {
			return Layout{}, fmt.Errorf("ducklake root dir is required")
		}
		root = filepath.Dir(firstNonEmpty(c.CatalogPath, c.DataPath))
	}
	layout := NewLayout(root)
	if c.CatalogPath != "" {
		layout.CatalogPath = c.CatalogPath
	}
	if c.DataPath != "" {
		layout.DataPath = c.DataPath
	}
	return layout, nil
}

func MigrateSQLiteCatalogDataPath(ctx context.Context, catalogPath, targetDataPath string) error {
	if strings.TrimSpace(catalogPath) == "" || strings.TrimSpace(targetDataPath) == "" {
		return nil
	}
	if _, err := os.Stat(catalogPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	db, err := sql.Open("sqlite", sqliteFileDSN(catalogPath))
	if err != nil {
		return err
	}
	defer db.Close()
	var stored string
	err = db.QueryRowContext(ctx, `SELECT value FROM ducklake_metadata WHERE "key" = 'data_path' AND scope IS NULL LIMIT 1`).Scan(&stored)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil
		}
		return err
	}
	if sameFilesystemPath(stored, targetDataPath) {
		return nil
	}
	if err := migrateLocalDataDir(stored, targetDataPath); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE ducklake_metadata SET value = ? WHERE "key" = 'data_path' AND scope IS NULL`, duckLakeMetadataPath(targetDataPath))
	return err
}

func sqliteFileDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)"
}

func sameFilesystemPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr == nil && rightErr == nil {
		return leftAbs == rightAbs
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func duckLakeMetadataPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func migrateLocalDataDir(source, target string) error {
	source = filepath.Clean(source)
	target = filepath.Clean(target)
	if sameFilesystemPath(source, target) {
		return nil
	}
	sourceInfo, sourceErr := os.Stat(source)
	if sourceErr != nil {
		if os.IsNotExist(sourceErr) {
			return os.MkdirAll(target, 0o755)
		}
		return sourceErr
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("DuckLake data path %s is not a directory", source)
	}
	targetInfo, targetErr := os.Stat(target)
	if targetErr != nil {
		if os.IsNotExist(targetErr) {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Rename(source, target); err == nil {
				return nil
			}
			return copyDir(source, target)
		}
		return targetErr
	}
	if !targetInfo.IsDir() {
		return fmt.Errorf("DuckLake target data path %s is not a directory", target)
	}
	empty, err := dirIsEmpty(target)
	if err != nil {
		return err
	}
	if empty {
		if err := os.Remove(target); err != nil {
			return err
		}
		if err := os.Rename(source, target); err == nil {
			return nil
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
	}
	return copyDir(source, target)
}

func dirIsEmpty(path string) (bool, error) {
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer dir.Close()
	_, err = dir.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if existing, err := os.Stat(targetPath); err == nil && existing.Size() == info.Size() {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		return copyFile(path, targetPath, info.Mode())
	})
}

func copyFile(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func (e *Environment) initialize(ctx context.Context, snapshot bool, snapshotID int64) error {
	for _, extension := range []string{"sqlite", "ducklake"} {
		if err := loadExtension(ctx, e.db, extension); err != nil {
			return err
		}
	}
	attach := fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS %s", sqlLiteral(e.layout.CatalogPath), catalogAlias)
	parts := []string{fmt.Sprintf("DATA_PATH '%s'", sqlLiteral(e.layout.DataPath))}
	if snapshot {
		parts = append(parts, fmt.Sprintf("SNAPSHOT_VERSION %d", snapshotID))
	}
	attach += " (" + strings.Join(parts, ", ") + ")"
	if _, err := e.db.ExecContext(ctx, attach); err != nil {
		return fmt.Errorf("attaching DuckLake catalog: %w", err)
	}
	if _, err := e.db.ExecContext(ctx, "USE "+catalogAlias); err != nil {
		return fmt.Errorf("using DuckLake catalog: %w", err)
	}
	return nil
}

func loadExtension(ctx context.Context, db *sql.DB, name string) error {
	if _, err := db.ExecContext(ctx, "LOAD "+name); err == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, "INSTALL "+name); err != nil {
		return fmt.Errorf("installing DuckDB extension %s: %w", name, err)
	}
	if _, err := db.ExecContext(ctx, "LOAD "+name); err != nil {
		return fmt.Errorf("loading DuckDB extension %s: %w", name, err)
	}
	return nil
}

func (e *Environment) Commit(ctx context.Context, servingStateID string, extra map[string]string, fn func(*sql.Tx) error) (int64, error) {
	if e == nil || e.db == nil {
		return 0, fmt.Errorf("ducklake environment is not initialized")
	}
	if fn == nil {
		return 0, fmt.Errorf("commit function is required")
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := setCommitMessage(ctx, tx, servingStateID, extra); err != nil {
		return 0, err
	}
	if err := fn(tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	snapshot, err := e.lastCommittedSnapshot(ctx)
	if err != nil {
		return 0, err
	}
	return snapshot, nil
}

func (e *Environment) Snapshots(ctx context.Context) ([]Snapshot, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT snapshot_id FROM "+catalogAlias+".snapshots() ORDER BY snapshot_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []Snapshot
	for rows.Next() {
		var snapshot Snapshot
		if err := rows.Scan(&snapshot.ID); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (e *Environment) RetentionCandidates(ctx context.Context, protected map[int64]struct{}) ([]int64, error) {
	snapshots, err := e.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []int64
	for _, snapshot := range snapshots {
		if snapshot.ID == 0 {
			continue
		}
		if _, ok := protected[snapshot.ID]; ok {
			continue
		}
		candidates = append(candidates, snapshot.ID)
	}
	return candidates, nil
}

func (e *Environment) ExpireSnapshots(ctx context.Context, versions []int64, dryRun bool) error {
	if len(versions) == 0 {
		return nil
	}
	_, err := e.db.ExecContext(ctx, fmt.Sprintf("CALL ducklake_expire_snapshots(%s, versions => %s, dry_run => %t)", sqlStringLiteral(catalogAlias), snapshotListLiteral(versions), dryRun))
	return err
}

func (e *Environment) CleanupOldFiles(ctx context.Context, dryRun bool) error {
	_, err := e.db.ExecContext(ctx, fmt.Sprintf("CALL ducklake_cleanup_old_files(%s, dry_run => %t)", sqlStringLiteral(catalogAlias), dryRun))
	return err
}

func (e *Environment) DeleteOrphanedFiles(ctx context.Context, dryRun bool) error {
	_, err := e.db.ExecContext(ctx, fmt.Sprintf("CALL ducklake_delete_orphaned_files(%s, dry_run => %t)", sqlStringLiteral(catalogAlias), dryRun))
	return err
}

func setCommitMessage(ctx context.Context, tx *sql.Tx, servingStateID string, extra map[string]string) error {
	servingStateID = strings.TrimSpace(servingStateID)
	if servingStateID == "" {
		servingStateID = "unknown"
	}
	payload := map[string]string{"servingStateId": servingStateID}
	for key, value := range extra {
		payload[key] = value
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"CALL "+catalogAlias+".set_commit_message(?, ?, extra_info => ?)",
		"LibreDash",
		"serving-state "+servingStateID,
		string(bytes),
	)
	return err
}

func (e *Environment) lastCommittedSnapshot(ctx context.Context) (int64, error) {
	var snapshot sql.NullInt64
	err := e.db.QueryRowContext(ctx, "SELECT id FROM "+catalogAlias+".last_committed_snapshot()").Scan(&snapshot)
	if err == nil && snapshot.Valid {
		return snapshot.Int64, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err := e.db.QueryRowContext(ctx, "SELECT id FROM "+catalogAlias+".current_snapshot()").Scan(&snapshot); err != nil {
		return 0, err
	}
	if !snapshot.Valid {
		return 0, fmt.Errorf("DuckLake did not report a committed snapshot")
	}
	return snapshot.Int64, nil
}

func (e *Environment) SQLDB() *sql.DB {
	if e == nil {
		return nil
	}
	return e.db
}

func (e *Environment) Path() string {
	if e == nil {
		return ""
	}
	return e.layout.CatalogPath
}

func (e *Environment) Exec(ctx context.Context, statement string) error {
	_, err := e.db.ExecContext(ctx, statement)
	return err
}

func (e *Environment) Query(ctx context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	rows, err := e.db.QueryContext(ctx, plan.SQL, plan.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]any, len(plan.Columns))
	scans := make([]any, len(plan.Columns))
	for i := range values {
		scans[i] = &values[i]
	}
	result := semanticquery.Rows{}
	for rows.Next() {
		if err := rows.Scan(scans...); err != nil {
			return nil, err
		}
		row := semanticquery.Row{}
		for i, column := range plan.Columns {
			row[column] = cloneValue(values[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (e *Environment) Count(ctx context.Context, plan semanticquery.Plan) (int, error) {
	var count int
	if err := e.db.QueryRowContext(ctx, plan.SQL, plan.Args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Environment) FloatBounds(ctx context.Context, plan semanticquery.Plan, valueColumn string) (semanticquery.FloatBounds, error) {
	if err := validateColumnAlias(valueColumn); err != nil {
		return semanticquery.FloatBounds{}, err
	}
	query := "WITH raw AS (" + plan.SQL + ")\nSELECT MIN(" + valueColumn + "), MAX(" + valueColumn + ") FROM raw"
	var minValue, maxValue sql.NullFloat64
	if err := e.db.QueryRowContext(ctx, query, plan.Args...).Scan(&minValue, &maxValue); err != nil {
		return semanticquery.FloatBounds{}, err
	}
	if !minValue.Valid || !maxValue.Valid {
		return semanticquery.FloatBounds{}, nil
	}
	return semanticquery.FloatBounds{Min: minValue.Float64, Max: maxValue.Float64, Valid: true}, nil
}

func (e *Environment) Histogram(ctx context.Context, plan semanticquery.Plan, spec semanticquery.HistogramSpec) ([]semanticquery.HistogramBin, error) {
	if err := validateColumnAlias(spec.ValueColumn); err != nil {
		return nil, err
	}
	bounds, err := e.FloatBounds(ctx, plan, spec.ValueColumn)
	if err != nil {
		return nil, err
	}
	if !bounds.Valid {
		return []semanticquery.HistogramBin{}, nil
	}
	if spec.BinCount <= 0 {
		return nil, fmt.Errorf("histogram bin count must be positive")
	}
	if bounds.Min == bounds.Max {
		var count int
		query := "WITH raw AS (" + plan.SQL + ")\nSELECT COUNT(*) FROM raw"
		if err := e.db.QueryRowContext(ctx, query, plan.Args...).Scan(&count); err != nil {
			return nil, err
		}
		return []semanticquery.HistogramBin{{Bucket: 0, Count: count, Start: bounds.Min, End: bounds.Max}}, nil
	}

	bucketExpr := fmt.Sprintf("LEAST(%d, CAST(FLOOR(((%s - ?) / NULLIF(? - ?, 0)) * ?) AS INTEGER))", spec.BinCount-1, spec.ValueColumn)
	query := fmt.Sprintf(`WITH raw AS (%s)
SELECT %s AS bucket, COUNT(*) AS value
FROM raw
GROUP BY bucket
ORDER BY bucket ASC`, plan.SQL, bucketExpr)
	args := append(append([]any{}, plan.Args...), bounds.Min, bounds.Max, bounds.Min, spec.BinCount)
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	width := (bounds.Max - bounds.Min) / float64(spec.BinCount)
	bins := []semanticquery.HistogramBin{}
	for rows.Next() {
		var bucket int
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		start := bounds.Min + float64(bucket)*width
		bins = append(bins, semanticquery.HistogramBin{
			Bucket: bucket,
			Count:  count,
			Start:  start,
			End:    start + width,
		})
	}
	return bins, rows.Err()
}

func (e *Environment) Distribution(ctx context.Context, plan semanticquery.Plan, spec semanticquery.DistributionSpec) (semanticquery.Rows, error) {
	if err := validateColumnAlias(spec.GroupColumn); err != nil {
		return nil, err
	}
	if err := validateColumnAlias(spec.ValueColumn); err != nil {
		return nil, err
	}
	orderBy, err := distributionOrderBy(spec.Sort)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`WITH raw AS (%s)
SELECT %s AS label,
       MIN(%s) AS min,
       quantile_cont(%s, 0.25) AS q1,
       median(%s) AS median,
       quantile_cont(%s, 0.75) AS q3,
       MAX(%s) AS max
FROM raw
GROUP BY label
ORDER BY %s`, plan.SQL, spec.GroupColumn, spec.ValueColumn, spec.ValueColumn, spec.ValueColumn, spec.ValueColumn, spec.ValueColumn, orderBy)
	if spec.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", spec.Limit)
	}
	return e.Query(ctx, semanticquery.Plan{
		SQL:     query,
		Args:    plan.Args,
		Columns: []string{"label", "min", "q1", "median", "q3", "max"},
	})
}

func (e *Environment) Layout() Layout {
	if e == nil {
		return Layout{}
	}
	return e.layout
}

func (e *Environment) Close() error {
	if e == nil || e.db == nil {
		return nil
	}
	return e.db.Close()
}

func extensionUnavailable(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "extension") &&
		(strings.Contains(text, "not found") ||
			strings.Contains(text, "failed to download") ||
			strings.Contains(text, "failed to install") ||
			strings.Contains(text, "not be loaded"))
}

func sqlLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func sqlStringLiteral(value string) string {
	return "'" + sqlLiteral(value) + "'"
}

func snapshotListLiteral(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "."
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed
	default:
		return typed
	}
}

func validateColumnAlias(value string) error {
	if value == "" {
		return fmt.Errorf("empty column alias")
	}
	for i, r := range value {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return fmt.Errorf("invalid column alias %q", value)
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("invalid column alias %q", value)
		}
	}
	return nil
}

func distributionOrderBy(sorts []semanticquery.Sort) (string, error) {
	if len(sorts) == 0 {
		return "label ASC", nil
	}
	parts := make([]string, 0, len(sorts))
	for _, sortSpec := range sorts {
		field := sortSpec.Field
		if field == "" {
			field = "label"
		}
		switch field {
		case "label", "min", "q1", "median", "q3", "max":
		default:
			return "", fmt.Errorf("unsupported distribution sort field %q", sortSpec.Field)
		}
		direction := "ASC"
		if strings.EqualFold(sortSpec.Direction, "desc") {
			direction = "DESC"
		} else if sortSpec.Direction != "" && !strings.EqualFold(sortSpec.Direction, "asc") {
			return "", fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
		}
		parts = append(parts, field+" "+direction)
	}
	return strings.Join(parts, ", "), nil
}
