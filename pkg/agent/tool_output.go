package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	toon "github.com/toon-format/toon-go"
)

type ToolOutputFormat string

const (
	ToolOutputTOON ToolOutputFormat = "toon"
	ToolOutputJSON ToolOutputFormat = "json"
)

type ToolOutputConfig struct {
	Format         ToolOutputFormat
	MaxStringChars int
	MaxArrayItems  int
	MaxObjectDepth int
}

type toolOutputTruncation struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Shown      int    `json:"shown,omitempty"`
	Total      int    `json:"total,omitempty"`
	ShownChars int    `json:"shown_chars,omitempty"`
	TotalChars int    `json:"total_chars,omitempty"`
	MaxDepth   int    `json:"max_depth,omitempty"`
}

func defaultToolOutputConfig(config ToolOutputConfig) ToolOutputConfig {
	if config.Format == "" {
		config.Format = ToolOutputTOON
	}
	if config.MaxStringChars == 0 {
		config.MaxStringChars = 2000
	}
	if config.MaxArrayItems == 0 {
		config.MaxArrayItems = 50
	}
	if config.MaxObjectDepth == 0 {
		config.MaxObjectDepth = 8
	}
	return config
}

func validateToolOutputConfig(config ToolOutputConfig) error {
	switch config.Format {
	case ToolOutputTOON, ToolOutputJSON:
	default:
		return NewError(ErrorCodeInvalidArgument, fmt.Sprintf("unsupported tool output format %q", config.Format), nil)
	}
	if config.MaxStringChars <= 0 {
		return NewError(ErrorCodeInvalidArgument, "max tool output string chars must be positive", nil)
	}
	if config.MaxArrayItems <= 0 {
		return NewError(ErrorCodeInvalidArgument, "max tool output array items must be positive", nil)
	}
	if config.MaxObjectDepth <= 0 {
		return NewError(ErrorCodeInvalidArgument, "max tool output object depth must be positive", nil)
	}
	return nil
}

func formatToolOutput(value any, config ToolOutputConfig) (string, error) {
	normalized, err := normalizeToolOutput(value)
	if err != nil {
		return "", err
	}
	normalized = wrapTopLevelToolOutput(normalized)
	var truncations []toolOutputTruncation
	truncated := truncateToolOutput(normalized, "$", 0, config, &truncations)
	if len(truncations) > 0 {
		if object, ok := truncated.(map[string]any); ok {
			object["_meta"] = map[string]any{
				"truncated":   true,
				"truncations": truncationsAsValues(truncations),
			}
		}
	}
	switch config.Format {
	case ToolOutputJSON:
		body, err := json.Marshal(truncated)
		if err != nil {
			return "", err
		}
		return string(body), nil
	default:
		return toon.MarshalString(truncated)
	}
}

func normalizeToolOutput(value any) (any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func wrapTopLevelToolOutput(value any) any {
	switch typed := value.(type) {
	case []any:
		return map[string]any{"count": json.Number(strconv.Itoa(len(typed))), "items": typed}
	case map[string]any:
		return typed
	default:
		return map[string]any{"value": typed}
	}
}

func truncateToolOutput(value any, path string, depth int, config ToolOutputConfig, truncations *[]toolOutputTruncation) any {
	if depth >= config.MaxObjectDepth {
		*truncations = append(*truncations, toolOutputTruncation{Path: path, Kind: "depth", MaxDepth: config.MaxObjectDepth})
		return fmt.Sprintf("[truncated: max depth %d]", config.MaxObjectDepth)
	}
	switch typed := value.(type) {
	case string:
		runes := []rune(typed)
		if len(runes) <= config.MaxStringChars {
			return typed
		}
		*truncations = append(*truncations, toolOutputTruncation{Path: path, Kind: "string", ShownChars: config.MaxStringChars, TotalChars: len(runes)})
		return string(runes[:config.MaxStringChars])
	case []any:
		total := len(typed)
		shown := total
		if shown > config.MaxArrayItems {
			shown = config.MaxArrayItems
			*truncations = append(*truncations, toolOutputTruncation{Path: path, Kind: "array", Shown: shown, Total: total})
		}
		out := make([]any, shown)
		for i := 0; i < shown; i++ {
			out[i] = truncateToolOutput(typed[i], fmt.Sprintf("%s[%d]", path, i), depth+1, config, truncations)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for _, key := range sortedToolOutputKeys(typed) {
			out[key] = truncateToolOutput(typed[key], path+"."+key, depth+1, config, truncations)
		}
		return out
	default:
		return typed
	}
}

func truncationsAsValues(truncations []toolOutputTruncation) []any {
	out := make([]any, len(truncations))
	for i, truncation := range truncations {
		item := map[string]any{
			"kind": truncation.Kind,
			"path": truncation.Path,
		}
		if truncation.Shown > 0 || truncation.Total > 0 {
			item["shown"] = truncation.Shown
			item["total"] = truncation.Total
		}
		if truncation.ShownChars > 0 || truncation.TotalChars > 0 {
			item["shown_chars"] = truncation.ShownChars
			item["total_chars"] = truncation.TotalChars
		}
		if truncation.MaxDepth > 0 {
			item["max_depth"] = truncation.MaxDepth
		}
		out[i] = item
	}
	return out
}

func sortedToolOutputKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
