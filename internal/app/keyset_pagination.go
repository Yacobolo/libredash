package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const defaultListLimit = 50
const maxListLimit = 200

type keysetPageCursor struct {
	Key string `json:"key"`
}

func keysetPage[T any](items []T, limitValue *int32, tokenValue *string, key func(T) string) ([]T, *string, error) {
	limit := defaultListLimit
	if limitValue != nil {
		if *limitValue < 1 || *limitValue > maxListLimit {
			return nil, nil, fmt.Errorf("limit must be between 1 and 200")
		}
		limit = int(*limitValue)
	}
	start := 0
	if tokenValue != nil && strings.TrimSpace(*tokenValue) != "" {
		cursor, err := decodeKeysetCursor(*tokenValue)
		if err != nil {
			return nil, nil, err
		}
		start = -1
		for index, item := range items {
			if key(item) == cursor.Key {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, nil, fmt.Errorf("cursor key is unavailable")
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := append(make([]T, 0, end-start), items[start:end]...)
	var next *string
	if end < len(items) && len(page) > 0 {
		value := encodeKeysetCursor(key(page[len(page)-1]))
		next = &value
	}
	return page, next, nil
}

func encodeKeysetCursor(key string) string {
	payload, _ := json.Marshal(keysetPageCursor{Key: key})
	return "k1." + base64.RawURLEncoding.EncodeToString(payload)
}

func decodeKeysetCursor(value string) (keysetPageCursor, error) {
	if !strings.HasPrefix(value, "k1.") {
		return keysetPageCursor{}, fmt.Errorf("invalid keyset cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "k1."))
	if err != nil {
		return keysetPageCursor{}, fmt.Errorf("invalid keyset cursor")
	}
	var cursor keysetPageCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Key == "" {
		return keysetPageCursor{}, fmt.Errorf("invalid keyset cursor")
	}
	return cursor, nil
}
