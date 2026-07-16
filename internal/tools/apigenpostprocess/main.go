package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

func main() {
	// APIGen emits Record<string> as Go map[string]any while OpenAPI keeps string values.
	// Relax only the asset payload OpenAPI schema to match Go.
	if err := relaxOpenAPIYAML("api/gen/openapi.yaml"); err != nil {
		fatal(err)
	}
	if err := addArrowOpenAPIYAML("api/gen/openapi.yaml"); err != nil {
		fatal(err)
	}
	if err := addVisualUnionOpenAPIYAML("api/gen/openapi.yaml"); err != nil {
		fatal(err)
	}
	if err := relaxEmbeddedOpenAPI("internal/api/gen/server.apigen.gen.go"); err != nil {
		fatal(err)
	}
	if err := addArrowEmbeddedOpenAPI("internal/api/gen/server.apigen.gen.go"); err != nil {
		fatal(err)
	}
	if err := addVisualUnionEmbeddedOpenAPI("internal/api/gen/server.apigen.gen.go"); err != nil {
		fatal(err)
	}
	if err := useGeneratedProblemDetails("internal/api/gen/server.apigen.gen.go"); err != nil {
		fatal(err)
	}
	if err := widenGeneratedInt64Fields("internal/api/gen/request_models.gen.go"); err != nil {
		fatal(err)
	}
}

var arrowOperationIDs = []string{"querySemanticModel", "previewSemanticDataset", "queryDashboardTable"}

type visualSchema struct {
	shape        string
	responseName string
	datumName    string
	fields       []string
	required     []string
}

var visualSchemas = []visualSchema{
	{"category_value", "CategoryValueVisualDataResponse", "CategoryValueVisualDatum", []string{"label", "value", "selected"}, []string{"label", "value"}},
	{"category_series_value", "CategorySeriesValueVisualDataResponse", "CategorySeriesValueVisualDatum", []string{"label", "series", "value", "selected"}, []string{"label", "series", "value"}},
	{"category_multi_measure", "CategoryMultiMeasureVisualDataResponse", "CategoryMultiMeasureVisualDatum", []string{"label", "series", "value", "selected"}, []string{"label", "series", "value"}},
	{"category_delta", "CategoryDeltaVisualDataResponse", "CategoryDeltaVisualDatum", []string{"label", "value", "start", "end", "positive", "selected"}, []string{"label", "value", "start", "end", "positive"}},
	{"binned_measure", "BinnedMeasureVisualDataResponse", "BinnedMeasureVisualDatum", []string{"label", "binStart", "binEnd", "value"}, []string{"label", "binStart", "binEnd", "value"}},
	{"hierarchy", "HierarchyVisualDataResponse", "HierarchyVisualDatum", []string{"path", "value"}, []string{"path", "value"}},
	{"single_value", "SingleValueVisualDataResponse", "SingleValueVisualDatum", []string{"label", "value", "series", "selected"}, []string{"label", "value"}},
	{"matrix", "MatrixVisualDataResponse", "MatrixVisualDatum", []string{"row", "column", "value", "selected"}, []string{"row", "column", "value"}},
	{"graph", "GraphVisualDataResponse", "GraphVisualDatum", []string{"source", "target", "value"}, []string{"source", "target", "value"}},
	{"geo", "GeoVisualDataResponse", "GeoVisualDatum", []string{"name", "value", "selected"}, []string{"name", "value"}},
	{"ohlc", "OHLCVisualDataResponse", "OHLCVisualDatum", []string{"label", "open", "close", "low", "high"}, []string{"label", "open", "close", "low", "high"}},
	{"distribution", "DistributionVisualDataResponse", "DistributionVisualDatum", []string{"label", "min", "q1", "median", "q3", "max"}, []string{"label", "min", "q1", "median", "q3", "max"}},
}

func addVisualUnionEmbeddedOpenAPI(path string) error {
	return mutateEmbeddedOpenAPI(path, addVisualUnionSchemas)
}

func addVisualUnionSchemas(doc map[string]any) error {
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	response, _ := schemas["DashboardVisualDataResponse"].(map[string]any)
	datum, _ := schemas["DashboardVisualDatum"].(map[string]any)
	properties, _ := datum["properties"].(map[string]any)
	if response == nil || len(properties) == 0 {
		return fmt.Errorf("dashboard visual OpenAPI base schemas missing")
	}
	mapping := make(map[string]any, len(visualSchemas))
	oneOf := make([]any, 0, len(visualSchemas))
	for _, visual := range visualSchemas {
		mapping[visual.shape] = "#/components/schemas/" + visual.responseName
		oneOf = append(oneOf, map[string]any{"$ref": "#/components/schemas/" + visual.responseName})
		datumProperties := make(map[string]any, len(visual.fields))
		for _, field := range visual.fields {
			property, ok := properties[field]
			if !ok {
				return fmt.Errorf("dashboard visual datum field %s missing", field)
			}
			datumProperties[field] = property
		}
		schemas[visual.datumName] = map[string]any{
			"type": "object", "required": stringSliceAny(visual.required), "properties": datumProperties, "additionalProperties": false,
		}
		schemas[visual.responseName] = map[string]any{
			"type": "object", "required": []any{"shape", "data"}, "additionalProperties": false,
			"properties": map[string]any{
				"shape": map[string]any{"type": "string", "enum": []any{visual.shape}},
				"data":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/" + visual.datumName}},
			},
		}
	}
	response["discriminator"] = map[string]any{"propertyName": "shape", "mapping": mapping}
	response["oneOf"] = oneOf
	return nil
}

func stringSliceAny(values []string) []any {
	out := make([]any, len(values))
	for index, value := range values {
		out[index] = value
	}
	return out
}

func addVisualUnionOpenAPIYAML(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	const responseMarker = "    DashboardVisualDataResponse:\n"
	start := strings.Index(text, responseMarker)
	if start < 0 {
		return fmt.Errorf("%s: dashboard visual response schema missing", path)
	}
	insertAt := start + len(responseMarker)
	var union strings.Builder
	union.WriteString("      discriminator:\n        propertyName: shape\n        mapping:\n")
	for _, visual := range visualSchemas {
		fmt.Fprintf(&union, "          %s: '#/components/schemas/%s'\n", visual.shape, visual.responseName)
	}
	union.WriteString("      oneOf:\n")
	for _, visual := range visualSchemas {
		fmt.Fprintf(&union, "        - $ref: '#/components/schemas/%s'\n", visual.responseName)
	}
	text = text[:insertAt] + union.String() + text[insertAt:]

	const datumMarker = "    DashboardVisualDatum:\n"
	datumAt := strings.Index(text, datumMarker)
	if datumAt < 0 {
		return fmt.Errorf("%s: dashboard visual datum schema missing", path)
	}
	var variants strings.Builder
	for _, visual := range visualSchemas {
		writeVisualResponseYAML(&variants, visual)
		writeVisualDatumYAML(&variants, visual)
	}
	text = text[:datumAt] + variants.String() + text[datumAt:]
	return os.WriteFile(path, []byte(text), 0o644)
}

func writeVisualResponseYAML(out *strings.Builder, visual visualSchema) {
	fmt.Fprintf(out, "    %s:\n      type: object\n      additionalProperties: false\n      required:\n        - shape\n        - data\n      properties:\n        shape:\n          type: string\n          enum:\n            - %s\n        data:\n          type: array\n          items:\n            $ref: '#/components/schemas/%s'\n", visual.responseName, visual.shape, visual.datumName)
}

func writeVisualDatumYAML(out *strings.Builder, visual visualSchema) {
	fmt.Fprintf(out, "    %s:\n      type: object\n      additionalProperties: false\n", visual.datumName)
	if len(visual.required) != 0 {
		out.WriteString("      required:\n")
		for _, field := range visual.required {
			fmt.Fprintf(out, "        - %s\n", field)
		}
	}
	out.WriteString("      properties:\n")
	for _, field := range visual.fields {
		fmt.Fprintf(out, "        %s:\n%s", field, visualFieldYAML(field))
	}
}

func visualFieldYAML(field string) string {
	switch field {
	case "selected", "positive":
		return "          type: boolean\n"
	case "start", "end", "binStart", "binEnd", "min", "q1", "median", "q3", "max":
		return "          type: number\n          format: double\n"
	case "path":
		return "          type: array\n          items:\n            type: string\n"
	case "value", "open", "close", "low", "high":
		return "          description: A JSON scalar value.\n"
	default:
		return "          type: string\n"
	}
}

func addArrowOpenAPIYAML(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	const media = "            application/vnd.apache.arrow.stream:\n              schema:\n                type: string\n                format: binary\n"
	for _, operationID := range arrowOperationIDs {
		marker := "      operationId: " + operationID + "\n"
		start := strings.Index(text, marker)
		if start < 0 {
			return fmt.Errorf("%s: Arrow operation %s not found", path, operationID)
		}
		relativeEnd := strings.Index(text[start:], "        '400':\n")
		if relativeEnd < 0 {
			return fmt.Errorf("%s: Arrow operation %s has no 400 response boundary", path, operationID)
		}
		end := start + relativeEnd
		section := text[start:end]
		if strings.Contains(section, "application/vnd.apache.arrow.stream") {
			continue
		}
		if !strings.Contains(section, "            application/json:\n") {
			return fmt.Errorf("%s: Arrow operation %s has no JSON success content", path, operationID)
		}
		text = text[:end] + media + text[end:]
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func addArrowEmbeddedOpenAPI(path string) error {
	return mutateEmbeddedOpenAPI(path, func(doc map[string]any) error {
		paths, ok := doc["paths"].(map[string]any)
		if !ok {
			return fmt.Errorf("OpenAPI paths missing")
		}
		remaining := make(map[string]bool, len(arrowOperationIDs))
		for _, operationID := range arrowOperationIDs {
			remaining[operationID] = true
		}
		for _, rawPath := range paths {
			pathItem, _ := rawPath.(map[string]any)
			for _, rawOperation := range pathItem {
				operation, _ := rawOperation.(map[string]any)
				operationID, _ := operation["operationId"].(string)
				if !remaining[operationID] {
					continue
				}
				responses, _ := operation["responses"].(map[string]any)
				response, _ := responses["200"].(map[string]any)
				content, _ := response["content"].(map[string]any)
				if content == nil {
					return fmt.Errorf("Arrow operation %s has no success content", operationID)
				}
				content["application/vnd.apache.arrow.stream"] = map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}
				delete(remaining, operationID)
			}
		}
		if len(remaining) != 0 {
			return fmt.Errorf("Arrow operations missing from embedded OpenAPI: %v", remaining)
		}
		return nil
	})
}

func mutateEmbeddedOpenAPI(path string, mutate func(map[string]any) error) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const prefix = "const embeddedOpenAPISpecJSON = `"
	text := string(raw)
	start := strings.Index(text, prefix)
	if start < 0 {
		return fmt.Errorf("%s: embedded OpenAPI constant not found", path)
	}
	jsonStart := start + len(prefix)
	end := strings.Index(text[jsonStart:], "`")
	if end < 0 {
		return fmt.Errorf("%s: embedded OpenAPI constant is unterminated", path)
	}
	jsonEnd := jsonStart + end
	var doc map[string]any
	if err := json.Unmarshal([]byte(text[jsonStart:jsonEnd]), &doc); err != nil {
		return fmt.Errorf("%s: decode embedded OpenAPI: %w", path, err)
	}
	if err := mutate(doc); err != nil {
		return err
	}
	next, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%s: encode embedded OpenAPI: %w", path, err)
	}
	return os.WriteFile(path, []byte(text[:jsonStart]+string(next)+text[jsonEnd:]), 0o644)
}

func useGeneratedProblemDetails(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	const oldWriter = `w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(Error{Code: apigenchi.SafeIntToInt32(statusCode), Message: apigenErrorMessage(statusCode, message)})`
	const newWriter = `w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ProblemDetails{
		Type: "https://libredash.dev/problems/http-error", Title: http.StatusText(statusCode),
		Status: apigenchi.SafeIntToInt32(statusCode), Detail: apigenErrorMessage(statusCode, message),
		Instance: "", Code: fmt.Sprintf("HTTP_%d", statusCode), RequestId: w.Header().Get("X-Request-ID"),
		Errors: []ProblemFieldError{},
	})`
	if count := strings.Count(text, oldWriter); count != 1 {
		return fmt.Errorf("%s: expected one generated error writer, found %d", path, count)
	}
	text = strings.Replace(text, oldWriter, newWriter, 1)
	if count := strings.Count(text, "Body Error"); count != 7 {
		return fmt.Errorf("%s: expected seven generated shared error bodies, found %d", path, count)
	}
	text = strings.ReplaceAll(text, "Body Error", "Body ProblemDetails")
	return os.WriteFile(path, []byte(text), 0o644)
}

func widenGeneratedInt64Fields(path string) error {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	widened := 0
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structure.Fields.List {
				if len(field.Names) != 1 || !generatedInt64Field(typeSpec.Name.Name, field.Names[0].Name) {
					continue
				}
				identifier := integerIdentifier(field.Type)
				if identifier == nil || identifier.Name != "int32" {
					continue
				}
				identifier.Name = "int64"
				widened++
			}
		}
	}
	if widened != 8 {
		return fmt.Errorf("%s: widened %d generated int64 fields, want 8", path, widened)
	}
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := format.Node(output, set, file); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func generatedInt64Field(typeName, fieldName string) bool {
	if typeName == "ReleaseArtifactResponse" {
		return fieldName == "SizeBytes"
	}
	if !strings.HasPrefix(typeName, "ManagedData") {
		return false
	}
	switch fieldName {
	case "Size", "Offset", "MinimumPartSize", "MaximumPartSize":
		return true
	default:
		return false
	}
}

func integerIdentifier(expression ast.Expr) *ast.Ident {
	switch value := expression.(type) {
	case *ast.Ident:
		return value
	case *ast.StarExpr:
		identifier, _ := value.X.(*ast.Ident)
		return identifier
	default:
		return nil
	}
}

func relaxOpenAPIYAML(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const before = `        payload:
          type: object
          additionalProperties:
            type: string
          example:
            key: example
`
	const after = `        payload:
          type: object
          additionalProperties: {}
          example:
            key: example
`
	text := string(raw)
	const sectionStart = "    AssetResponse:\n"
	const sectionEnd = "    AssetSummaryResponse:\n"
	start := strings.Index(text, sectionStart)
	if start < 0 {
		return fmt.Errorf("%s: AssetResponse schema section not found", path)
	}
	end := strings.Index(text[start:], sectionEnd)
	if end < 0 {
		return fmt.Errorf("%s: AssetSummaryResponse schema section not found after AssetResponse", path)
	}
	end += start
	section := text[start:end]
	if count := strings.Count(section, before); count != 1 {
		return fmt.Errorf("%s: expected one AssetResponse payload schema block, found %d", path, count)
	}
	section = strings.Replace(section, before, after, 1)
	return os.WriteFile(path, []byte(text[:start]+section+text[end:]), 0o644)
}

func relaxEmbeddedOpenAPI(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const prefix = "const embeddedOpenAPISpecJSON = `"
	text := string(raw)
	start := strings.Index(text, prefix)
	if start < 0 {
		return fmt.Errorf("%s: embedded OpenAPI constant not found", path)
	}
	jsonStart := start + len(prefix)
	end := strings.Index(text[jsonStart:], "`")
	if end < 0 {
		return fmt.Errorf("%s: embedded OpenAPI constant is unterminated", path)
	}
	jsonEnd := jsonStart + end
	encoded := text[jsonStart:jsonEnd]

	var doc map[string]any
	if err := json.Unmarshal([]byte(encoded), &doc); err != nil {
		return fmt.Errorf("%s: decode embedded OpenAPI: %w", path, err)
	}
	if err := relaxAssetResponsePayload(doc); err != nil {
		return err
	}
	next, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%s: encode embedded OpenAPI: %w", path, err)
	}
	return os.WriteFile(path, []byte(text[:jsonStart]+string(next)+text[jsonEnd:]), 0o644)
}

func relaxAssetResponsePayload(doc map[string]any) error {
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI components missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI components.schemas missing")
	}
	assetResponse, ok := schemas["AssetResponse"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI AssetResponse schema missing")
	}
	properties, ok := assetResponse["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI AssetResponse properties missing")
	}
	payload, ok := properties["payload"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI AssetResponse.payload schema missing")
	}
	additional, ok := payload["additionalProperties"].(map[string]any)
	if !ok || additional["type"] != "string" {
		return fmt.Errorf("OpenAPI AssetResponse.payload did not have generated string-only additionalProperties")
	}
	payload["additionalProperties"] = map[string]any{}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
