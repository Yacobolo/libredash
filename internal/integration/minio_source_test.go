package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analyticsduckdb "github.com/Yacobolo/leapview/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/leapview/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestMinIOParquetSourceRefreshContract(t *testing.T) {
	endpoint := strings.TrimRight(os.Getenv("LEAPVIEW_TEST_MINIO_ENDPOINT"), "/")
	if endpoint == "" {
		t.Skip("set LEAPVIEW_TEST_MINIO_ENDPOINT to run the MinIO integration test")
	}
	ctx := context.Background()
	const (
		bucket = "leapview-integration"
		key    = "orders/current.parquet"
		region = "us-east-1"
		user   = "leapview"
		secret = "leapview-integration-secret"
	)
	client := minIOClient(t, ctx, endpoint, region, user, secret)
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create MinIO bucket: %v", err)
	}

	putMinIOObject(t, ctx, client, bucket, "commerce/"+key, parquetFixture(t, 10, 20))
	credentialJSON := fmt.Sprintf(`{"access_key_id":%q,"secret_access_key":%q,"region":%q,"endpoint":%q,"url_style":"path","use_ssl":false}`,
		user, secret, region, strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://"))
	t.Setenv("LEAPVIEW_TEST_MINIO_CREDENTIALS", credentialJSON)
	model := minIOModel(bucket, key)
	if err := model.Validate(); err != nil {
		t.Fatalf("validate scoped MinIO model: %v", err)
	}

	escape := minIOModel(bucket, "../outside/orders.parquet")
	if err := escape.Validate(); err == nil || !strings.Contains(err.Error(), "escapes connection scope") {
		t.Fatalf("path escape validation error = %v", err)
	}

	db, err := analyticsduckdb.Open(ctx, filepath.Join(t.TempDir(), "materialized.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sources := analyticsduckdb.NewSourceRuntime(db)
	if _, err := analyticsmaterialize.Refresh(ctx, db, sources, model); err != nil {
		t.Fatalf("initial MinIO refresh: %v", err)
	}
	if got := materializedRevenue(t, ctx, db); got != 30 {
		t.Fatalf("initial materialized revenue = %v, want 30", got)
	}

	putMinIOObject(t, ctx, client, bucket, "commerce/"+key, parquetFixture(t, 40, 50))
	if got := materializedRevenue(t, ctx, db); got != 30 {
		t.Fatalf("external replacement changed served data before refresh: %v", got)
	}
	if _, err := analyticsmaterialize.Refresh(ctx, db, sources, model); err != nil {
		t.Fatalf("replacement MinIO refresh: %v", err)
	}
	if got := materializedRevenue(t, ctx, db); got != 90 {
		t.Fatalf("refreshed materialized revenue = %v, want 90", got)
	}

	putMinIOObject(t, ctx, client, bucket, "commerce/"+key, []byte("not parquet"))
	if _, err := analyticsmaterialize.Refresh(ctx, db, sources, model); err == nil {
		t.Fatal("broken external object refresh succeeded")
	}
	if got := materializedRevenue(t, ctx, db); got != 90 {
		t.Fatalf("failed refresh replaced prior materialization: %v", got)
	}
}

func minIOClient(t *testing.T, ctx context.Context, endpoint, region, user, secret string) *awss3.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(user, secret, "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return awss3.NewFromConfig(cfg, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
}

func putMinIOObject(t *testing.T, ctx context.Context, client *awss3.Client, bucket, key string, body []byte) {
	t.Helper()
	if _, err := client.PutObject(ctx, &awss3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)}); err != nil {
		t.Fatalf("put MinIO object: %v", err)
	}
}

func parquetFixture(t *testing.T, revenues ...int) []byte {
	t.Helper()
	dir := t.TempDir()
	db, err := analyticsduckdb.Open(context.Background(), filepath.Join(dir, "fixture.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	values := make([]string, 0, len(revenues))
	for index, revenue := range revenues {
		values = append(values, fmt.Sprintf("('o%d', %d)", index+1, revenue))
	}
	path := filepath.Join(dir, "orders.parquet")
	_, err = db.SQLDB().Exec(`CREATE TABLE orders(order_id VARCHAR, revenue DOUBLE); INSERT INTO orders VALUES ` + strings.Join(values, ",") + `; COPY orders TO '` + analyticsduckdb.SQLString(path) + `' (FORMAT PARQUET)`)
	closeErr := db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func minIOModel(bucket, key string) *semanticmodel.Model {
	scope := "s3://" + bucket + "/commerce/"
	return &semanticmodel.Model{
		Name:              "commerce",
		DefaultConnection: "lake",
		Connections: map[string]semanticmodel.Connection{
			"lake": {Kind: "s3", Scope: scope, Credentials: semanticmodel.ConnectionCredentials{Provider: "env", Secret: "LEAPVIEW_TEST_MINIO_CREDENTIALS"}},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Connection: "lake", Path: "s3://" + bucket + "/commerce/" + key, Format: "parquet"},
		},
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}, "revenue": {Expr: "revenue", Type: "number"}},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Fact: "orders", Label: "Revenue", Aggregation: "sum", Input: semanticmodel.MeasureInput{Field: "orders.revenue"}, Empty: "zero"},
		},
	}
}

func materializedRevenue(t *testing.T, ctx context.Context, db *analyticsduckdb.Database) float64 {
	t.Helper()
	var total float64
	if err := db.SQLDB().QueryRowContext(ctx, `SELECT SUM(revenue) FROM model.orders`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}
