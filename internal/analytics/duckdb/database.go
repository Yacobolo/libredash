package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
	_ "github.com/duckdb/duckdb-go/v2"
)

type Database struct {
	db   *sql.DB
	path string
}

func Open(ctx context.Context, path string) (*Database, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Database{db: db, path: path}, nil
}

func (d *Database) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *Database) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *Database) SQLDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.db
}

func (d *Database) Exec(ctx context.Context, statement string) error {
	_, err := d.db.ExecContext(ctx, statement)
	return err
}

func (d *Database) Query(ctx context.Context, plan semanticquery.Plan) (semanticquery.Rows, error) {
	conn, err := d.queryConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return queryRows(ctx, conn, plan)
}

func queryRows(ctx context.Context, conn *sql.Conn, plan semanticquery.Plan) (semanticquery.Rows, error) {
	rows, err := conn.QueryContext(ctx, plan.SQL, plan.Args...)
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

func (d *Database) Count(ctx context.Context, plan semanticquery.Plan) (int, error) {
	conn, err := d.queryConnection(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	var count int
	if err := conn.QueryRowContext(ctx, plan.SQL, plan.Args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *Database) FloatBounds(ctx context.Context, plan semanticquery.Plan, valueColumn string) (semanticquery.FloatBounds, error) {
	if err := validateColumnAlias(valueColumn); err != nil {
		return semanticquery.FloatBounds{}, err
	}
	conn, err := d.queryConnection(ctx)
	if err != nil {
		return semanticquery.FloatBounds{}, err
	}
	defer conn.Close()
	return floatBounds(ctx, conn, plan, valueColumn)
}

func floatBounds(ctx context.Context, conn *sql.Conn, plan semanticquery.Plan, valueColumn string) (semanticquery.FloatBounds, error) {
	if err := validateColumnAlias(valueColumn); err != nil {
		return semanticquery.FloatBounds{}, err
	}
	query := "WITH raw AS (" + plan.SQL + ")\nSELECT MIN(" + valueColumn + "), MAX(" + valueColumn + ") FROM raw"
	var minValue, maxValue sql.NullFloat64
	if err := conn.QueryRowContext(ctx, query, plan.Args...).Scan(&minValue, &maxValue); err != nil {
		return semanticquery.FloatBounds{}, err
	}
	if !minValue.Valid || !maxValue.Valid {
		return semanticquery.FloatBounds{}, nil
	}
	return semanticquery.FloatBounds{Min: minValue.Float64, Max: maxValue.Float64, Valid: true}, nil
}

func (d *Database) Histogram(ctx context.Context, plan semanticquery.Plan, spec semanticquery.HistogramSpec) ([]semanticquery.HistogramBin, error) {
	if err := validateColumnAlias(spec.ValueColumn); err != nil {
		return nil, err
	}
	conn, err := d.queryConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	bounds, err := floatBounds(ctx, conn, plan, spec.ValueColumn)
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
		if err := conn.QueryRowContext(ctx, query, plan.Args...).Scan(&count); err != nil {
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
	rows, err := conn.QueryContext(ctx, query, args...)
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

func (d *Database) Distribution(ctx context.Context, plan semanticquery.Plan, spec semanticquery.DistributionSpec) (semanticquery.Rows, error) {
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
	conn, err := d.queryConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
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
	return queryRows(ctx, conn, semanticquery.Plan{
		SQL:     query,
		Args:    plan.Args,
		Columns: []string{"label", "min", "q1", "median", "q3", "max"},
	})
}

func (d *Database) queryConnection(ctx context.Context) (*sql.Conn, error) {
	started := time.Now()
	conn, err := d.db.Conn(ctx)
	dataquery.ObserveConnectionWait(ctx, time.Since(started))
	return conn, err
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
