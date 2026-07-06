package tools

import (
	"sort"
	"strings"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

const ExtensionKey = "x-agent"

const QueryVisualToolName = "query_visual"

type Extension struct {
	Enabled      bool
	Name         string
	Risk         string
	Tags         []string
	DefaultLimit int
	Output       Output
}

type Output struct {
	ItemsPath   string
	Fields      []string
	CursorPath  string
	Count       bool
	RootFields  []string
	Collections []OutputCollection
	Maps        []OutputMap
}

type OutputCollection struct {
	Path   string
	As     string
	Fields []string
	Count  bool
}

type OutputMap struct {
	Path       string
	As         string
	Fields     []string
	Collection OutputCollection
}

type APIGenOperation struct {
	Contract  apigenapi.GenOperationContract
	Extension Extension
}

func APIGenOperations() []APIGenOperation {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		return nil
	}
	paths, _ := spec["paths"].(map[string]any)
	contracts := apigenapi.GetAPIGenOperationContracts()
	operations := make([]APIGenOperation, 0, len(contracts))
	for _, contract := range contracts {
		extension, ok := ParseExtension(contract.Extensions[ExtensionKey])
		if !ok || !operationAllowed(contract, extension) || !hasOpenAPIOperation(paths, contract) {
			continue
		}
		operations = append(operations, APIGenOperation{Contract: contract, Extension: extension})
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Extension.Name < operations[j].Extension.Name
	})
	return operations
}

func APIGenToolNames() []string {
	operations := APIGenOperations()
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Extension.Name)
	}
	return names
}

func ManualToolNames() []string {
	return []string{QueryVisualToolName}
}

func ToolNames() []string {
	names := append([]string{}, APIGenToolNames()...)
	names = append(names, ManualToolNames()...)
	sort.Strings(names)
	return names
}

func IsKnownTool(name string) bool {
	for _, tool := range ToolNames() {
		if tool == name {
			return true
		}
	}
	return false
}

func ParseExtension(value any) (Extension, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return Extension{}, false
	}
	extension := Extension{
		Enabled:      boolFromMap(raw, "enabled"),
		Name:         stringFromMap(raw, "name"),
		Risk:         stringFromMap(raw, "risk"),
		DefaultLimit: intFromMap(raw, "defaultLimit"),
		Output:       outputFromMap(raw, "output"),
	}
	if tags, ok := raw["tags"].([]any); ok {
		for _, tag := range tags {
			if text, ok := tag.(string); ok && text != "" {
				extension.Tags = append(extension.Tags, text)
			}
		}
	}
	return extension, true
}

func outputFromMap(values map[string]any, key string) Output {
	raw, ok := values[key].(map[string]any)
	if !ok {
		return Output{}
	}
	output := Output{
		ItemsPath:   stringFromMap(raw, "itemsPath"),
		Fields:      stringsFromMap(raw, "fields"),
		CursorPath:  stringFromMap(raw, "cursorPath"),
		Count:       boolFromMap(raw, "count"),
		RootFields:  stringsFromMap(raw, "rootFields"),
		Collections: outputCollectionsFromMap(raw, "collections"),
		Maps:        outputMapsFromMap(raw, "maps"),
	}
	return output
}

func outputCollectionsFromMap(values map[string]any, key string) []OutputCollection {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	collections := make([]OutputCollection, 0, len(raw))
	for _, value := range raw {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		collection := outputCollectionFromMap(object)
		if collection.Path != "" && collection.As != "" {
			collections = append(collections, collection)
		}
	}
	return collections
}

func outputMapsFromMap(values map[string]any, key string) []OutputMap {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	maps := make([]OutputMap, 0, len(raw))
	for _, value := range raw {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		outputMap := OutputMap{
			Path:   stringFromMap(object, "path"),
			As:     stringFromMap(object, "as"),
			Fields: stringsFromMap(object, "fields"),
		}
		if collection, ok := object["collection"].(map[string]any); ok {
			outputMap.Collection = outputCollectionFromMap(collection)
		}
		if outputMap.Path != "" && outputMap.As != "" {
			maps = append(maps, outputMap)
		}
	}
	return maps
}

func outputCollectionFromMap(values map[string]any) OutputCollection {
	return OutputCollection{
		Path:   stringFromMap(values, "path"),
		As:     stringFromMap(values, "as"),
		Fields: stringsFromMap(values, "fields"),
		Count:  boolFromMap(values, "count"),
	}
}

func operationAllowed(contract apigenapi.GenOperationContract, extension Extension) bool {
	if !extension.Enabled || extension.Name == "" || extension.Risk != "read" {
		return false
	}
	if contract.Manual {
		return false
	}
	if contract.Method != "GET" && contract.Method != "POST" {
		return false
	}
	switch operationPrivilege(contract) {
	case "USE_WORKSPACE", "VIEW_ITEM", "QUERY_DATA", "PREVIEW_DATA", "REFRESH_DATA":
		return true
	default:
		return false
	}
}

func operationPrivilege(contract apigenapi.GenOperationContract) string {
	raw, _ := contract.Extensions["x-authz"].(map[string]any)
	return stringFromMap(raw, "privilege")
}

func hasOpenAPIOperation(paths map[string]any, contract apigenapi.GenOperationContract) bool {
	pathItem, ok := paths[contract.Path].(map[string]any)
	if !ok {
		return false
	}
	_, ok = pathItem[strings.ToLower(contract.Method)].(map[string]any)
	return ok
}

func boolFromMap(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func stringFromMap(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func stringsFromMap(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		if text, ok := value.(string); ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

func intFromMap(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
