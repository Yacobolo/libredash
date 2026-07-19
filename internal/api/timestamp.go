package api

import (
	"fmt"
	"strings"
	"time"
)

var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

// NormalizeTimestamp converts persisted timestamps to the RFC3339 UTC shape
// required by the public API contract.
func NormalizeTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, layout := range timestampLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}
	return "", fmt.Errorf("timestamp %q is not RFC3339 or a supported persisted timestamp", value)
}
