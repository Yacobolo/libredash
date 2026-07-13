package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	// APIGen emits Record<string> as Go map[string]any while OpenAPI keeps string values.
	// Relax only the asset payload OpenAPI schema to match Go.
	if err := relaxOpenAPIYAML("api/gen/openapi.yaml"); err != nil {
		fatal(err)
	}
	if err := relaxEmbeddedOpenAPI("internal/api/gen/server.apigen.gen.go"); err != nil {
		fatal(err)
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
