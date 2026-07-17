package http

import (
	"bytes"
	"fmt"
	stdhttp "net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

const arrowStreamMediaType = "application/vnd.apache.arrow.stream"

func writeSemanticQueryResponse(w stdhttp.ResponseWriter, r *stdhttp.Request, response api.SemanticQueryResponse) {
	if !acceptsMediaType(r.Header.Get("Accept"), arrowStreamMediaType) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, stdhttp.StatusOK, response)
		return
	}
	encoded, err := encodeSemanticArrow(response)
	if err != nil {
		writeJSONError(w, fmt.Errorf("encode Arrow response: %w", err), stdhttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", arrowStreamMediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Query-ID", response.QueryID)
	w.Header().Set("X-Serving-Snapshot", response.ServingSnapshot)
	if response.Page.NextCursor != "" {
		w.Header().Set("X-Next-Cursor", response.Page.NextCursor)
	}
	w.WriteHeader(stdhttp.StatusOK)
	_, _ = w.Write(encoded)
}

func acceptsMediaType(header, mediaType string) bool {
	for _, item := range strings.Split(header, ",") {
		value := strings.TrimSpace(strings.SplitN(item, ";", 2)[0])
		if value == mediaType {
			return true
		}
	}
	return false
}

func encodeSemanticArrow(response api.SemanticQueryResponse) ([]byte, error) {
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
			value := ""
			if columnIndex < len(row) {
				value = row[columnIndex]
			}
			builder.Append(value)
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
