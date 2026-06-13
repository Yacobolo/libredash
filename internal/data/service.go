package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	_ "github.com/marcboeker/go-duckdb/v2"
)

var requiredFiles = map[string]string{
	"orders":       "olist_orders_dataset.csv",
	"order_items":  "olist_order_items_dataset.csv",
	"payments":     "olist_order_payments_dataset.csv",
	"products":     "olist_products_dataset.csv",
	"customers":    "olist_customers_dataset.csv",
	"reviews":      "olist_order_reviews_dataset.csv",
	"translations": "product_category_name_translation.csv",
}

type MissingDataError struct {
	DataDir string
	Missing []string
}

func (e *MissingDataError) Error() string {
	return fmt.Sprintf("Olist CSVs are missing in %s: %s. Run scripts/bootstrap_olist.py or set LIBREDASH_DATA_DIR.", e.DataDir, strings.Join(e.Missing, ", "))
}

type DuckDBMetrics struct {
	db      *sql.DB
	dataDir string
	ready   bool
	missing error
}

func NewDuckDBMetrics(dataDir string) (*DuckDBMetrics, error) {
	metrics := &DuckDBMetrics{dataDir: dataDir}
	if err := metrics.validateFiles(); err != nil {
		metrics.missing = err
		return metrics, nil
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, err
	}
	metrics.db = db

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := metrics.registerViews(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	metrics.ready = true
	return metrics, nil
}

func (m *DuckDBMetrics) Close() error {
	if m.db == nil {
		return nil
	}
	return m.db.Close()
}

func (m *DuckDBMetrics) DataDir() string {
	return m.dataDir
}

func (m *DuckDBMetrics) QueryDashboard(ctx context.Context, filters dashboard.Filters) (dashboard.Patch, error) {
	filters = filters.WithDefaults()
	if !m.ready {
		return dashboard.EmptyPatch(filters, m.dataDir, m.missing), nil
	}

	patch := dashboard.Patch{
		Filters: filters,
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   time.Now().Format("15:04:05"),
			DataDirectory: m.dataDir,
		},
		Charts: map[string]dashboard.Chart{},
	}

	kpis, err := m.kpis(ctx, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.KPIs = kpis

	charts, err := m.charts(ctx, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.Charts = charts

	return patch, nil
}

func (m *DuckDBMetrics) QueryTable(ctx context.Context, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	filters = filters.WithDefaults()
	request = request.WithDefaults()
	if !m.ready {
		return dashboard.EmptyTable(request, m.missing), nil
	}
	if request.Table != "orders" {
		return dashboard.EmptyTable(request, fmt.Errorf("unknown table %q", request.Table)), nil
	}

	totalRows, err := m.countOrders(ctx, filters)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	rows, err := m.orderRows(ctx, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}

	return dashboard.Table{
		Title:     "Orders",
		Columns:   dashboard.OrdersTableColumns(),
		Rows:      rows,
		TotalRows: totalRows,
		Window:    dashboard.TableWindow{Offset: request.Offset, Limit: request.Limit},
		Sort:      request.Sort,
		Loading:   false,
		Error:     "",
	}, nil
}

func (m *DuckDBMetrics) validateFiles() error {
	var missing []string
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(m.dataDir, file)); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, file)
		} else if err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		return &MissingDataError{DataDir: m.dataDir, Missing: missing}
	}
	return nil
}

func (m *DuckDBMetrics) registerViews(ctx context.Context) error {
	for view, file := range requiredFiles {
		path := filepath.Join(m.dataDir, file)
		stmt := fmt.Sprintf("CREATE OR REPLACE VIEW %s AS SELECT * FROM read_csv_auto('%s', header=true)", view, sqlString(path))
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("registering %s: %w", view, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) kpis(ctx context.Context, filters dashboard.Filters) ([]dashboard.KPI, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
WITH filtered_orders AS (
	SELECT o.order_id
	FROM orders o
	JOIN customers c ON c.customer_id = o.customer_id
	WHERE %s
),
revenue AS (
	SELECT COALESCE(SUM(try_cast(p.payment_value AS DOUBLE)), 0) AS revenue
	FROM payments p
	JOIN filtered_orders fo ON fo.order_id = p.order_id
),
order_count AS (
	SELECT COUNT(DISTINCT order_id) AS orders FROM filtered_orders
),
review_score AS (
	SELECT AVG(try_cast(r.review_score AS DOUBLE)) AS score
	FROM reviews r
	JOIN filtered_orders fo ON fo.order_id = r.order_id
)
SELECT
	order_count.orders,
	revenue.revenue,
	CASE WHEN order_count.orders = 0 THEN 0 ELSE revenue.revenue / order_count.orders END AS aov,
	COALESCE(review_score.score, 0) AS review_score
FROM order_count, revenue, review_score`, where)

	var orders int64
	var revenue, aov, review float64
	if err := m.db.QueryRowContext(ctx, query, args...).Scan(&orders, &revenue, &aov, &review); err != nil {
		return nil, err
	}

	return []dashboard.KPI{
		{Label: "Orders", Value: formatInt(orders), Note: "Filtered order count", Tone: "ink"},
		{Label: "Revenue", Value: formatCurrency(revenue), Note: "Total payment value", Tone: "green"},
		{Label: "AOV", Value: formatCurrency(aov), Note: "Revenue per order", Tone: "amber"},
		{Label: "Review", Value: fmt.Sprintf("%.2f", review), Note: "Average score", Tone: "coral"},
	}, nil
}

func (m *DuckDBMetrics) charts(ctx context.Context, filters dashboard.Filters) (map[string]dashboard.Chart, error) {
	revenue, err := m.revenueByMonth(ctx, filters)
	if err != nil {
		return nil, err
	}
	orders, err := m.ordersByStatus(ctx, filters)
	if err != nil {
		return nil, err
	}
	categories, err := m.topCategories(ctx, filters)
	if err != nil {
		return nil, err
	}
	delivery, err := m.deliveryBuckets(ctx, filters)
	if err != nil {
		return nil, err
	}

	return map[string]dashboard.Chart{
		"revenue":    {Title: "Revenue by month", Unit: "R$", Data: revenue},
		"orders":     {Title: "Orders by status", Unit: "orders", Data: orders},
		"categories": {Title: "Top product categories", Unit: "R$", Data: categories},
		"delivery":   {Title: "Delivery speed", Unit: "orders", Data: delivery},
	}, nil
}

func (m *DuckDBMetrics) revenueByMonth(ctx context.Context, filters dashboard.Filters) ([]dashboard.Point, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
SELECT
	strftime(CAST(o.order_purchase_timestamp AS TIMESTAMP), '%%Y-%%m') AS month,
	COALESCE(SUM(try_cast(p.payment_value AS DOUBLE)), 0) AS revenue
FROM orders o
JOIN payments p ON p.order_id = o.order_id
JOIN customers c ON c.customer_id = o.customer_id
WHERE %s
GROUP BY month
ORDER BY month
LIMIT 30`, where)
	return m.queryPoints(ctx, query, args...)
}

func (m *DuckDBMetrics) ordersByStatus(ctx context.Context, filters dashboard.Filters) ([]dashboard.Point, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
SELECT o.order_status, COUNT(DISTINCT o.order_id) AS orders
FROM orders o
JOIN customers c ON c.customer_id = o.customer_id
WHERE %s
GROUP BY o.order_status
ORDER BY orders DESC`, where)
	return m.queryPoints(ctx, query, args...)
}

func (m *DuckDBMetrics) topCategories(ctx context.Context, filters dashboard.Filters) ([]dashboard.Point, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
SELECT
	COALESCE(t.product_category_name_english, p.product_category_name, 'uncategorized') AS category,
	COALESCE(SUM(try_cast(oi.price AS DOUBLE) + try_cast(oi.freight_value AS DOUBLE)), 0) AS revenue
FROM order_items oi
JOIN orders o ON o.order_id = oi.order_id
JOIN customers c ON c.customer_id = o.customer_id
LEFT JOIN products p ON p.product_id = oi.product_id
LEFT JOIN translations t ON t.product_category_name = p.product_category_name
WHERE %s
GROUP BY category
ORDER BY revenue DESC
LIMIT 10`, where)
	return m.queryPoints(ctx, query, args...)
}

func (m *DuckDBMetrics) deliveryBuckets(ctx context.Context, filters dashboard.Filters) ([]dashboard.Point, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
WITH deliveries AS (
	SELECT datediff('day', CAST(o.order_purchase_timestamp AS TIMESTAMP), CAST(o.order_delivered_customer_date AS TIMESTAMP)) AS days
	FROM orders o
	JOIN customers c ON c.customer_id = o.customer_id
	WHERE %s AND o.order_delivered_customer_date IS NOT NULL
)
SELECT
	CASE
		WHEN days <= 3 THEN '0-3 days'
		WHEN days <= 7 THEN '4-7 days'
		WHEN days <= 14 THEN '8-14 days'
		WHEN days <= 30 THEN '15-30 days'
		ELSE '31+ days'
	END AS bucket,
	COUNT(*) AS orders
FROM deliveries
GROUP BY bucket
ORDER BY MIN(days)`, where)
	return m.queryPoints(ctx, query, args...)
}

func (m *DuckDBMetrics) queryPoints(ctx context.Context, query string, args ...any) ([]dashboard.Point, error) {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []dashboard.Point{}
	for rows.Next() {
		var label string
		var value float64
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		points = append(points, dashboard.Point{Label: label, Value: round(value)})
	}
	return points, rows.Err()
}

func (m *DuckDBMetrics) countOrders(ctx context.Context, filters dashboard.Filters) (int, error) {
	where, args := filterWhere("o", filters)
	query := fmt.Sprintf(`
SELECT COUNT(DISTINCT o.order_id)
FROM orders o
JOIN customers c ON c.customer_id = o.customer_id
WHERE %s`, where)

	var total int
	if err := m.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (m *DuckDBMetrics) orderRows(ctx context.Context, filters dashboard.Filters, request dashboard.TableRequest) ([]map[string]any, error) {
	where, args := filterWhere("o", filters)
	sortExpr := orderTableSortExpr(request.Sort.Key)
	direction := "DESC"
	if request.Sort.Direction == "asc" {
		direction = "ASC"
	}

	query := fmt.Sprintf(`
WITH revenue AS (
	SELECT order_id, SUM(try_cast(payment_value AS DOUBLE)) AS revenue
	FROM payments
	GROUP BY order_id
),
review AS (
	SELECT order_id, AVG(try_cast(review_score AS DOUBLE)) AS review_score
	FROM reviews
	GROUP BY order_id
),
category AS (
	SELECT
		oi.order_id,
		MIN(COALESCE(t.product_category_name_english, p.product_category_name, 'uncategorized')) AS category
	FROM order_items oi
	LEFT JOIN products p ON p.product_id = oi.product_id
	LEFT JOIN translations t ON t.product_category_name = p.product_category_name
	GROUP BY oi.order_id
)
SELECT
	o.order_id,
	strftime(CAST(o.order_purchase_timestamp AS TIMESTAMP), '%%Y-%%m-%%d') AS purchase_date,
	o.order_status,
	c.customer_state,
	COALESCE(category.category, 'uncategorized') AS category,
	round(COALESCE(revenue.revenue, 0), 2) AS revenue,
	round(COALESCE(review.review_score, 0), 2) AS review_score,
	CASE
		WHEN o.order_delivered_customer_date IS NULL THEN NULL
		ELSE datediff('day', CAST(o.order_purchase_timestamp AS TIMESTAMP), CAST(o.order_delivered_customer_date AS TIMESTAMP))
	END AS delivery_days
FROM orders o
JOIN customers c ON c.customer_id = o.customer_id
LEFT JOIN revenue ON revenue.order_id = o.order_id
LEFT JOIN review ON review.order_id = o.order_id
LEFT JOIN category ON category.order_id = o.order_id
WHERE %s
ORDER BY %s %s, o.order_id ASC
LIMIT ? OFFSET ?`, where, sortExpr, direction)

	args = append(args, request.Limit, request.Offset)
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []map[string]any{}
	for rows.Next() {
		var orderID, purchaseDate, status, state, category string
		var revenue, review float64
		var delivery sql.NullInt64
		if err := rows.Scan(&orderID, &purchaseDate, &status, &state, &category, &revenue, &review, &delivery); err != nil {
			return nil, err
		}
		row := map[string]any{
			"order_id":      orderID,
			"purchase_date": purchaseDate,
			"status":        status,
			"state":         state,
			"category":      category,
			"revenue":       round(revenue),
			"review_score":  round(review),
			"delivery_days": nil,
		}
		if delivery.Valid {
			row["delivery_days"] = delivery.Int64
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func orderTableSortExpr(key string) string {
	switch key {
	case "order_id":
		return "o.order_id"
	case "purchase_date":
		return "CAST(o.order_purchase_timestamp AS TIMESTAMP)"
	case "status":
		return "o.order_status"
	case "state":
		return "c.customer_state"
	case "category":
		return "COALESCE(category.category, 'uncategorized')"
	case "revenue":
		return "COALESCE(revenue.revenue, 0)"
	case "review_score":
		return "COALESCE(review.review_score, 0)"
	case "delivery_days":
		return `CASE
			WHEN o.order_delivered_customer_date IS NULL THEN NULL
			ELSE datediff('day', CAST(o.order_purchase_timestamp AS TIMESTAMP), CAST(o.order_delivered_customer_date AS TIMESTAMP))
		END`
	default:
		return "CAST(o.order_purchase_timestamp AS TIMESTAMP)"
	}
}

func filterWhere(orderAlias string, filters dashboard.Filters) (string, []any) {
	filters = filters.WithDefaults()
	conditions := []string{"1 = 1"}
	args := []any{}

	if filters.State != "" && filters.State != "all" {
		conditions = append(conditions, "c.customer_state = ?")
		args = append(args, strings.ToUpper(filters.State))
	}

	switch filters.DateRange {
	case "2017":
		conditions = append(conditions, fmt.Sprintf("CAST(%s.order_purchase_timestamp AS TIMESTAMP) >= TIMESTAMP '2017-01-01' AND CAST(%s.order_purchase_timestamp AS TIMESTAMP) < TIMESTAMP '2018-01-01'", orderAlias, orderAlias))
	case "2018":
		conditions = append(conditions, fmt.Sprintf("CAST(%s.order_purchase_timestamp AS TIMESTAMP) >= TIMESTAMP '2018-01-01' AND CAST(%s.order_purchase_timestamp AS TIMESTAMP) < TIMESTAMP '2019-01-01'", orderAlias, orderAlias))
	case "recent":
		conditions = append(conditions, fmt.Sprintf("CAST(%s.order_purchase_timestamp AS TIMESTAMP) >= (SELECT max(CAST(order_purchase_timestamp AS TIMESTAMP)) - INTERVAL 90 DAY FROM orders)", orderAlias))
	}

	if filters.Category != "" && filters.Category != "all" {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM order_items filter_oi
			LEFT JOIN products filter_p ON filter_p.product_id = filter_oi.product_id
			LEFT JOIN translations filter_t ON filter_t.product_category_name = filter_p.product_category_name
			WHERE filter_oi.order_id = %s.order_id
			AND lower(COALESCE(filter_t.product_category_name_english, filter_p.product_category_name, '')) LIKE lower(?)
		)`, orderAlias))
		args = append(args, "%"+filters.Category+"%")
	}

	return strings.Join(conditions, " AND "), args
}

func sqlString(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), "'", "''")
}

func formatCurrency(value float64) string {
	if value >= 1000000 {
		return fmt.Sprintf("R$ %.1fm", value/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("R$ %.1fk", value/1000)
	}
	return fmt.Sprintf("R$ %.0f", value)
}

func formatInt(value int64) string {
	if value >= 1000000 {
		return fmt.Sprintf("%.1fm", float64(value)/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	}
	return fmt.Sprintf("%d", value)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}
