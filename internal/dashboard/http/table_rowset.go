package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

const dashboardArrowMediaType = "application/vnd.apache.arrow.stream"

func dashboardTableRowset(table dashboard.Table, block string, start, limit int, scope, snapshot string) api.DashboardTableQueryResponse {
	rows := table.Blocks[block].Rows
	if len(rows) > limit {
		rows = rows[:limit]
	}
	columns := make([]api.QueryColumn, len(table.Columns))
	encodedRows := make([][]string, 0, len(rows))
	for index, column := range table.Columns {
		columns[index] = api.QueryColumn{Name: column.Key, Type: dashboardColumnType(rows, column.Key), Nullable: dashboardColumnNullable(rows, column.Key)}
	}
	for _, row := range rows {
		values := make([]string, len(table.Columns))
		for index, column := range table.Columns {
			values[index] = dashboardCellString(row[column.Key])
		}
		encodedRows = append(encodedRows, values)
	}
	next := ""
	if start+len(encodedRows) < table.AvailableRows {
		next = encodeIndexCursor(start+len(encodedRows), scope, snapshot)
	}
	queryDigest := sha256String(scope)
	return api.DashboardTableQueryResponse{
		QueryID: "query_" + queryDigest[:24], ServingSnapshot: snapshot, Title: table.Title,
		Columns: columns, Rows: encodedRows, AvailableRows: table.AvailableRows, Page: api.PageInfo{NextCursor: next},
	}
}

func writeDashboardTableRowset(w stdhttp.ResponseWriter, r *stdhttp.Request, response api.DashboardTableQueryResponse) {
	if requestID := strings.TrimSpace(r.Header.Get("X-Request-ID")); requestID != "" {
		response.QueryID = requestID
	}
	w.Header().Set("Cache-Control", "no-store")
	if !acceptsDashboardMediaType(r.Header.Get("Accept"), dashboardArrowMediaType) {
		writeJSON(w, stdhttp.StatusOK, response)
		return
	}
	payload, err := encodeDashboardTableArrow(response)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", dashboardArrowMediaType)
	w.Header().Set("X-Query-ID", response.QueryID)
	w.Header().Set("X-Serving-Snapshot", response.ServingSnapshot)
	if response.Page.NextCursor != "" {
		w.Header().Set("X-Next-Cursor", response.Page.NextCursor)
	}
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write(payload)
}

func acceptsDashboardMediaType(header, mediaType string) bool {
	for _, item := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(item, ";", 2)[0]), mediaType) {
			return true
		}
	}
	return false
}

func encodeDashboardTableArrow(response api.DashboardTableQueryResponse) ([]byte, error) {
	metadata := arrow.NewMetadata(
		[]string{"libredash.query_id", "libredash.serving_snapshot", "libredash.next_cursor"},
		[]string{response.QueryID, response.ServingSnapshot, response.Page.NextCursor},
	)
	fields := make([]arrow.Field, len(response.Columns))
	for index, column := range response.Columns {
		fields[index] = arrow.Field{Name: column.Name, Type: arrow.BinaryTypes.String, Nullable: column.Nullable, Metadata: arrow.NewMetadata([]string{"libredash.logical_type"}, []string{column.Type})}
	}
	schema := arrow.NewSchema(fields, &metadata)
	allocator := memory.NewGoAllocator()
	arrays := make([]arrow.Array, len(fields))
	for columnIndex := range fields {
		builder := array.NewStringBuilder(allocator)
		for _, row := range response.Rows {
			if columnIndex >= len(row) {
				builder.AppendNull()
				continue
			}
			builder.Append(row[columnIndex])
		}
		arrays[columnIndex] = builder.NewArray()
		builder.Release()
	}
	defer func() {
		for _, values := range arrays {
			values.Release()
		}
	}()
	record := array.NewRecord(schema, arrays, int64(len(response.Rows)))
	defer record.Release()
	var output bytes.Buffer
	writer := ipc.NewWriter(&output, ipc.WithSchema(schema))
	if err := writer.Write(record); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func dashboardColumnType(rows []map[string]any, key string) string {
	for _, row := range rows {
		value := row[key]
		if value == nil {
			continue
		}
		switch value.(type) {
		case bool:
			return "boolean"
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return "int64"
		case float32, float64:
			return "float64"
		case time.Time:
			return "timestamp"
		case json.RawMessage, map[string]any, []any:
			return "json"
		default:
			return "string"
		}
	}
	return "string"
}

func dashboardColumnNullable(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if value, ok := row[key]; !ok || value == nil {
			return true
		}
	}
	return len(rows) == 0
}

func dashboardCellString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.FormatInt(int64(typed), 10)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		encoded, err := json.Marshal(typed)
		if err == nil && (len(encoded) == 0 || encoded[0] == '{' || encoded[0] == '[') {
			return string(encoded)
		}
		return fmt.Sprint(typed)
	}
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:])
}
