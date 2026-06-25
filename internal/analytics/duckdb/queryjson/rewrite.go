package queryjson

import (
	"fmt"
	"sort"
	"strings"
)

type rewriteEdit struct {
	start       int
	end         int
	replacement string
}

func RewriteSourceRefs(sql string, refs []TableRef, replacements map[string]string) (string, error) {
	edits := []rewriteEdit{}
	for _, ref := range refs {
		if strings.ToLower(ref.Schema) != "source" {
			continue
		}
		replacement, ok := replacements[ref.Table]
		if !ok {
			return "", fmt.Errorf("no replacement for source %q", ref.Table)
		}
		start, end, err := tableRefExtent(sql, ref)
		if err != nil {
			return "", err
		}
		edits = append(edits, rewriteEdit{start: start, end: end, replacement: replacement})
	}
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})
	out := sql
	for _, edit := range edits {
		out = out[:edit.start] + edit.replacement + out[edit.end:]
	}
	return out, nil
}

func tableRefExtent(sql string, ref TableRef) (int, int, error) {
	if ref.QueryLocation < 0 || ref.QueryLocation >= len(sql) {
		return 0, 0, fmt.Errorf("source %q has invalid query_location %d", ref.Table, ref.QueryLocation)
	}
	first, next, ok := readIdentifier(sql, ref.QueryLocation)
	if !ok {
		return 0, 0, fmt.Errorf("source %q query_location %d does not start with an identifier", ref.Table, ref.QueryLocation)
	}
	dot := skipSpaces(sql, next)
	if dot >= len(sql) || sql[dot] != '.' {
		return 0, 0, fmt.Errorf("source %q query_location %d is not a qualified relation", ref.Table, ref.QueryLocation)
	}
	secondStart := skipSpaces(sql, dot+1)
	second, end, ok := readIdentifier(sql, secondStart)
	if !ok {
		return 0, 0, fmt.Errorf("source %q query_location %d has no table identifier", ref.Table, ref.QueryLocation)
	}
	if !strings.EqualFold(first, ref.Schema) || second != ref.Table {
		return 0, 0, fmt.Errorf("source %q query_location %d resolved %s.%s", ref.Table, ref.QueryLocation, first, second)
	}
	return ref.QueryLocation, end, nil
}

func readIdentifier(sql string, index int) (string, int, bool) {
	if index >= len(sql) {
		return "", index, false
	}
	if sql[index] == '"' {
		var builder strings.Builder
		for cursor := index + 1; cursor < len(sql); cursor++ {
			if sql[cursor] == '"' {
				if cursor+1 < len(sql) && sql[cursor+1] == '"' {
					builder.WriteByte('"')
					cursor++
					continue
				}
				return builder.String(), cursor + 1, true
			}
			builder.WriteByte(sql[cursor])
		}
		return "", len(sql), false
	}
	if !isIdentifierStart(sql[index]) {
		return "", index, false
	}
	cursor := index + 1
	for cursor < len(sql) && isIdentifierPart(sql[cursor]) {
		cursor++
	}
	return sql[index:cursor], cursor, true
}

func skipSpaces(sql string, index int) int {
	for index < len(sql) {
		switch sql[index] {
		case ' ', '\n', '\r', '\t', '\f':
			index++
		default:
			return index
		}
	}
	return index
}

func isIdentifierStart(char byte) bool {
	return char == '_' || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
}

func isIdentifierPart(char byte) bool {
	return isIdentifierStart(char) || (char >= '0' && char <= '9')
}
