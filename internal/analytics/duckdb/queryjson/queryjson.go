package queryjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type SQLAnalysis struct {
	SourceRefs []string
	RawRefs    []string
	CTEs       []string
	Aliases    map[string]TableRef
	TableRefs  []TableRef
}

type TableRef struct {
	Schema        string
	Table         string
	Alias         string
	QueryLocation int
}

type ExplainAnalysis struct {
	Scans []Scan
}

type Scan struct {
	Operator    string
	Catalog     string
	Schema      string
	Table       string
	Projections []string
}

func AnalyzeSQL(input []byte) (SQLAnalysis, error) {
	var root any
	if err := json.Unmarshal(input, &root); err != nil {
		return SQLAnalysis{}, err
	}
	object, _ := root.(map[string]any)
	if errorValue, _ := object["error"].(bool); errorValue {
		message, _ := object["error_message"].(string)
		if message == "" {
			message = "unknown error"
		}
		return SQLAnalysis{}, fmt.Errorf("duckdb SQL JSON error: %s", message)
	}
	analysis := SQLAnalysis{Aliases: map[string]TableRef{}}
	sourceRefs := map[string]struct{}{}
	rawRefs := map[string]struct{}{}
	ctes := map[string]struct{}{}
	walkSQL(root, &analysis, sourceRefs, rawRefs, ctes)
	analysis.SourceRefs = sortedSet(sourceRefs)
	analysis.RawRefs = sortedSet(rawRefs)
	analysis.CTEs = sortedSet(ctes)
	return analysis, nil
}

func AnalyzeExplain(input []byte) (ExplainAnalysis, error) {
	var roots []planNode
	if err := json.Unmarshal(input, &roots); err != nil {
		return ExplainAnalysis{}, err
	}
	analysis := ExplainAnalysis{}
	for _, root := range roots {
		walkPlan(root, &analysis)
	}
	return analysis, nil
}

func walkSQL(value any, analysis *SQLAnalysis, sourceRefs, rawRefs, ctes map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		if cteMap, ok := typed["cte_map"].(map[string]any); ok {
			if entries, ok := cteMap["map"].([]any); ok {
				for _, entry := range entries {
					entryObject, ok := entry.(map[string]any)
					if !ok {
						continue
					}
					name, _ := entryObject["key"].(string)
					if name != "" {
						ctes[name] = struct{}{}
					}
				}
			} else {
				for name := range cteMap {
					ctes[name] = struct{}{}
				}
			}
		}
		if tableType, _ := typed["type"].(string); tableType == "BASE_TABLE" {
			schema, _ := typed["schema_name"].(string)
			table, _ := typed["table_name"].(string)
			alias, _ := typed["alias"].(string)
			location := queryLocation(typed["query_location"])
			ref := TableRef{Schema: schema, Table: table, Alias: alias, QueryLocation: location}
			analysis.TableRefs = append(analysis.TableRefs, ref)
			switch strings.ToLower(schema) {
			case "source":
				sourceRefs[table] = struct{}{}
			case "raw":
				rawRefs[table] = struct{}{}
			}
			if alias != "" {
				analysis.Aliases[alias] = ref
			}
		}
		for _, child := range typed {
			walkSQL(child, analysis, sourceRefs, rawRefs, ctes)
		}
	case []any:
		for _, child := range typed {
			walkSQL(child, analysis, sourceRefs, rawRefs, ctes)
		}
	}
}

func queryLocation(value any) int {
	switch typed := value.(type) {
	case float64:
		if typed > float64(lenSentinel()) {
			return -1
		}
		return int(typed)
	case int:
		return typed
	default:
		return -1
	}
}

func lenSentinel() int {
	return int(^uint(0) >> 1)
}

type planNode struct {
	Name      string         `json:"name"`
	Children  []planNode     `json:"children"`
	ExtraInfo map[string]any `json:"extra_info"`
}

func walkPlan(node planNode, analysis *ExplainAnalysis) {
	if tableText, ok := node.ExtraInfo["Table"].(string); ok && tableText != "" {
		catalog, schema, table := normalizeTableName(tableText)
		analysis.Scans = append(analysis.Scans, Scan{
			Operator:    node.Name,
			Catalog:     catalog,
			Schema:      schema,
			Table:       table,
			Projections: normalizeProjections(node.ExtraInfo["Projections"]),
		})
	}
	for _, child := range node.Children {
		walkPlan(child, analysis)
	}
}

func normalizeTableName(value string) (string, string, string) {
	parts := splitSQLName(value)
	if len(parts) == 0 {
		return "", "", ""
	}
	if len(parts) == 1 {
		return "", "", parts[0]
	}
	if len(parts) == 2 {
		return "", parts[0], parts[1]
	}
	return parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
}

func splitSQLName(value string) []string {
	parts := []string{}
	var builder strings.Builder
	quoted := false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if quoted {
			if char == '"' {
				if index+1 < len(value) && value[index+1] == '"' {
					builder.WriteByte('"')
					index++
					continue
				}
				quoted = false
				continue
			}
			builder.WriteByte(char)
			continue
		}
		switch char {
		case '"':
			quoted = true
		case '.':
			parts = append(parts, strings.TrimSpace(builder.String()))
			builder.Reset()
		default:
			builder.WriteByte(char)
		}
	}
	parts = append(parts, strings.TrimSpace(builder.String()))
	return parts
}

func normalizeProjections(value any) []string {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			result = appendProjection(result, text)
		}
		return result
	case string:
		return appendProjection(nil, typed)
	default:
		return nil
	}
}

func appendProjection(result []string, value string) []string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func MarshalForTest(value any) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
	return buffer.Bytes()
}
