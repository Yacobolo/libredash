package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
)

const (
	generatedModelsPath     = "internal/ui/signals/models.gen.go"
	generatedTypescriptPath = "web/generated/signals/index.ts"
)

var goInitialisms = map[string]string{
	"api":   "API",
	"html":  "HTML",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"ip":    "IP",
	"json":  "JSON",
	"ms":    "MS",
	"nan":   "NaN",
	"rpc":   "RPC",
	"sql":   "SQL",
	"sso":   "SSO",
	"tcp":   "TCP",
	"tls":   "TLS",
	"ui":    "UI",
	"uri":   "URI",
	"url":   "URL",
	"uuid":  "UUID",
	"xml":   "XML",
}

func main() {
	if err := postprocessGeneratedModels(generatedModelsPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := postprocessGeneratedTypescript(generatedTypescriptPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// postprocessGeneratedTypescript narrows interaction values from TypeSpec's
// unknown to the JSON-scalar contract enforced by the dashboard runtime. APIGen
// v0.4 does not emit non-literal TypeSpec unions, so keeping this small,
// fail-closed specialization next to the existing Go postprocessor preserves a
// useful client contract without weakening the wire format or forking APIGen.
func postprocessGeneratedTypescript(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated UI signal TypeScript: %w", err)
	}
	content := string(data)
	for _, model := range []string{"DashboardInteractionCommandMapping", "DashboardInteractionSelectionMapping"} {
		startMarker := "export interface " + model + " {\n"
		start := strings.Index(content, startMarker)
		if start < 0 {
			return fmt.Errorf("generated UI signal TypeScript is missing %s", model)
		}
		endOffset := strings.Index(content[start:], "\n}\n")
		if endOffset < 0 {
			return fmt.Errorf("generated UI signal TypeScript has an unterminated %s", model)
		}
		end := start + endOffset
		block := content[start:end]
		const (
			generated = "  value: unknown"
			typed     = "  value: string | number | boolean | null"
		)
		switch {
		case strings.Count(block, generated) == 1 && !strings.Contains(block, typed):
			block = strings.Replace(block, generated, typed, 1)
		case strings.Count(block, typed) == 1 && !strings.Contains(block, generated):
			// Already postprocessed.
		default:
			return fmt.Errorf("generated UI signal TypeScript %s must contain exactly one interaction value field", model)
		}
		content = content[:start] + block + content[end:]
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write generated UI signal TypeScript: %w", err)
	}
	return nil
}

func postprocessGeneratedModels(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated UI signal models: %w", err)
	}
	// APIGen v0.5 emits tag.Value for discriminated unions even when the
	// configured discriminator is named type. Keep the workaround scoped to the
	// generated DashboardVisual decoder until the upstream emitter is fixed.
	const visualDecoder = "func (value *DashboardVisual) UnmarshalJSON"
	if start := bytes.Index(data, []byte(visualDecoder)); start >= 0 {
		endMarker := []byte("\n}\n\ntype DashboardVisualBase")
		end := bytes.Index(data[start:], endMarker)
		if end < 0 {
			return fmt.Errorf("generated DashboardVisual decoder is unterminated")
		}
		end += start + len("\n}\n")
		decoder := bytes.ReplaceAll(data[start:end], []byte("tag.Value"), []byte("tag.Type"))
		data = append(append(append([]byte(nil), data[:start]...), decoder...), data[end:]...)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse generated UI signal models: %w", err)
	}

	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Tag == nil {
			return true
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return true
		}
		jsonName := reflect.StructTag(tag).Get("json")
		if before, _, ok := strings.Cut(jsonName, ","); ok {
			jsonName = before
		}
		if jsonName != "" && jsonName != "-" {
			field.Names[0].Name = goFieldName(jsonName)
		}
		return true
	})

	var output bytes.Buffer
	if err := format.Node(&output, fset, file); err != nil {
		return fmt.Errorf("format generated UI signal models: %w", err)
	}
	if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write generated UI signal models: %w", err)
	}
	return nil
}

func goFieldName(value string) string {
	parts := splitIdentifier(value)
	for i, part := range parts {
		if initialism, ok := goInitialisms[strings.ToLower(part)]; ok {
			parts[i] = initialism
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func splitIdentifier(value string) []string {
	var parts []string
	var current strings.Builder
	previousLower := false
	for _, char := range strings.TrimSpace(value) {
		if char == '-' || char == '_' || char == '.' || char == '/' || char == ' ' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			previousLower = false
			continue
		}
		isUpper := 'A' <= char && char <= 'Z'
		if isUpper && previousLower && current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
		current.WriteRune(char)
		previousLower = 'a' <= char && char <= 'z'
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
