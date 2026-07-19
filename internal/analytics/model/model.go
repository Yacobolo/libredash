package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m *Model) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("semantic model name is required")
	}
	if len(m.Sources) == 0 {
		return fmt.Errorf("semantic model %q has no sources", m.Name)
	}
	for name, connection := range m.Connections {
		resolved, err := connection.Validate(name)
		if err != nil {
			return err
		}
		m.Connections[name] = resolved
	}
	if m.DefaultConnection != "" {
		if err := validateSemanticIdentifier(m.DefaultConnection); err != nil {
			return fmt.Errorf("default_connection %q is invalid: %w", m.DefaultConnection, err)
		}
		if _, ok := m.Connections[m.DefaultConnection]; !ok {
			return fmt.Errorf("default_connection %q references unknown connection", m.DefaultConnection)
		}
	}
	for name, source := range m.Sources {
		resolved, err := m.resolveSource(source)
		if err != nil {
			return fmt.Errorf("source %q: %w", name, err)
		}
		if err := resolved.Validate(name, m.Connections); err != nil {
			return err
		}
		for field, sourceField := range resolved.Fields {
			if err := validateSemanticIdentifier(field); err != nil {
				return fmt.Errorf("source %q field %q is invalid: %w", name, field, err)
			}
			sourceField.Field = name + "." + field
			sourceField.Table = name
			sourceField.Name = field
			resolved.Fields[field] = sourceField
		}
		m.Sources[name] = resolved
	}
	if len(m.Tables) == 0 {
		return fmt.Errorf("semantic model %q has no model tables", m.Name)
	}
	for name, table := range m.Tables {
		if err := validateSemanticIdentifier(name); err != nil {
			return fmt.Errorf("model table %q has invalid name: %w", name, err)
		}
		if table.SQL != "" && table.Transform.SQL == "" {
			table.Transform.SQL = table.SQL
		}
		if table.Source == "" && table.Transform.SQL == "" {
			return fmt.Errorf("model table %q requires source or transform.sql", name)
		}
		if table.Source != "" {
			if _, ok := m.Sources[table.Source]; !ok {
				return fmt.Errorf("model table %q references unknown source %q", name, table.Source)
			}
		}
		if len(table.SourceReads) > 0 {
			return fmt.Errorf("model table %q source_reads is no longer supported; source reads are inferred from transform.sql", name)
		}
		dependencies, err := m.modelTableSourceDependencies(name, table)
		if err != nil {
			return err
		}
		table.SourceDependencies = dependencies
		modelDependencies, err := m.modelTableModelDependencies(name, table)
		if err != nil {
			return err
		}
		table.ModelDependencies = modelDependencies
		if table.PrimaryKey == "" {
			return fmt.Errorf("model table %q requires primary_key", name)
		}
		if table.Grain == "" {
			table.Grain = table.PrimaryKey
		}
		if table.Grain == "" {
			return fmt.Errorf("model table %q requires grain", name)
		}
		for field, dimension := range table.Dimensions {
			if err := validateSemanticIdentifier(field); err != nil {
				return fmt.Errorf("model table %q field %q is invalid: %w", name, field, err)
			}
			dimension.Field = name + "." + field
			dimension.Table = name
			dimension.Name = field
			if dimension.Label == "" {
				dimension.Label = titleFromIdentifier(field)
			}
			table.Dimensions[field] = dimension
		}
		columns, err := m.resolveModelColumns(name, table)
		if err != nil {
			return err
		}
		table.Columns = columns
		m.Tables[name] = table
	}
	seenRelationships := map[string]struct{}{}
	for index, relationship := range m.Relationships {
		if relationship.ID == "" || relationship.From == "" || relationship.To == "" {
			return fmt.Errorf("relationship %d requires id, from, and to", index)
		}
		if _, exists := seenRelationships[relationship.ID]; exists {
			return fmt.Errorf("duplicate relationship id %q", relationship.ID)
		}
		seenRelationships[relationship.ID] = struct{}{}
	}
	if err := m.validateSemanticGraph(); err != nil {
		return err
	}
	return nil
}

func (m *Model) modelTableSourceDependencies(tableName string, table Table) ([]string, error) {
	sql := strings.TrimSpace(table.Transform.SQL)
	if sql == "" {
		sql = strings.TrimSpace(table.SQL)
	}
	hasSQL := sql != ""
	if hasSQL {
		if table.Source != "" {
			return nil, fmt.Errorf("model table %q uses transform.sql and must declare sources instead of source", tableName)
		}
		if err := validateModelSQLQuery(tableName, sql); err != nil {
			return nil, err
		}
	} else if table.Source == "" {
		return nil, fmt.Errorf("model table %q requires source or transform.sql", tableName)
	}
	seen := map[string]struct{}{}
	add := func(source string) error {
		source = strings.TrimSpace(source)
		if source == "" {
			return nil
		}
		if _, ok := m.Sources[source]; !ok {
			return fmt.Errorf("model table %q references unknown source %q", tableName, source)
		}
		seen[source] = struct{}{}
		return nil
	}
	if err := add(table.Source); err != nil {
		return nil, err
	}
	for _, source := range table.Sources {
		if err := add(source); err != nil {
			return nil, err
		}
	}
	inferred, rawRefs, unqualifiedRefs := m.modelSQLSourceRefs(sql)
	if len(rawRefs) > 0 {
		return nil, fmt.Errorf("model table %q model SQL must reference sources through source.<name>; raw.<name> is internal", tableName)
	}
	if len(unqualifiedRefs) > 0 {
		return nil, fmt.Errorf("model table %q SQL must reference sources through source.<name>; found unqualified relation %q", tableName, unqualifiedRefs[0])
	}
	for _, source := range inferred {
		if _, ok := m.Sources[source]; !ok {
			return nil, fmt.Errorf("model table %q SQL references unknown source %q", tableName, source)
		}
	}
	result := make([]string, 0, len(seen))
	for source := range seen {
		result = append(result, source)
	}
	sort.Strings(result)
	if hasSQL && !sameStringSet(result, inferred) {
		if len(result) == 0 && len(inferred) > 0 {
			return nil, fmt.Errorf("model table %q uses transform.sql and requires sources", tableName)
		}
		return nil, fmt.Errorf("model table %q SQL source references %v do not match declared sources %v", tableName, inferred, result)
	}
	return result, nil
}

func (m *Model) resolveModelColumns(tableName string, table Table) (map[string]ModelColumn, error) {
	if len(table.Columns) > 0 {
		columns := make(map[string]ModelColumn, len(table.Columns))
		for name, column := range table.Columns {
			if err := validateSemanticIdentifier(name); err != nil {
				return nil, fmt.Errorf("model table %q column %q is invalid: %w", tableName, name, err)
			}
			if column.SourceField == "" {
				column.SourceField = name
			}
			if table.Source != "" && table.Transform.SQL == "" {
				if err := validateSemanticIdentifier(column.SourceField); err != nil {
					return nil, fmt.Errorf("model table %q column %q source_field %q is invalid: %w", tableName, name, column.SourceField, err)
				}
			}
			column.Name = name
			column.Field = tableName + "." + name
			columns[name] = column
		}
		if err := validateRequiredModelColumns(tableName, table, columns); err != nil {
			return nil, err
		}
		return columns, nil
	}
	columns := map[string]ModelColumn{}
	add := func(name string) {
		if name == "" {
			return
		}
		columns[name] = ModelColumn{Name: name, Field: tableName + "." + name, SourceField: name}
	}
	add(table.PrimaryKey)
	for field := range table.Dimensions {
		add(field)
	}
	if m != nil {
		for _, measure := range m.Measures {
			if measure.Fact != tableName {
				continue
			}
			refs := []string{measure.Input.Field}
			if measure.Input.Expression != "" {
				if expression, err := ParseExpression(measure.Input.Expression); err == nil {
					refs = append(refs, expression.References()...)
				}
			}
			for _, ref := range refs {
				refTable, refField, ok := strings.Cut(ref, ".")
				if ok && refTable == tableName {
					add(refField)
				}
			}
		}
	}
	return columns, validateRequiredModelColumns(tableName, table, columns)
}

func validateRequiredModelColumns(tableName string, table Table, columns map[string]ModelColumn) error {
	require := func(field, reason string) error {
		if field == "" {
			return nil
		}
		if _, ok := columns[field]; !ok {
			return fmt.Errorf("model table %q column contract missing %s %q", tableName, reason, field)
		}
		return nil
	}
	if err := require(table.PrimaryKey, "primary_key"); err != nil {
		return err
	}
	for field := range table.Dimensions {
		if err := require(field, "field"); err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) modelTableModelDependencies(tableName string, table Table) ([]string, error) {
	sql := strings.TrimSpace(table.Transform.SQL)
	if sql == "" {
		sql = strings.TrimSpace(table.SQL)
	}
	if sql == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	for _, ref := range scanSQLRelationRefs(sql) {
		if ref.Namespace != "model" {
			continue
		}
		if ref.Name == tableName {
			return nil, fmt.Errorf("model table %q cannot read itself", tableName)
		}
		if _, ok := m.Tables[ref.Name]; !ok {
			return nil, fmt.Errorf("model table %q SQL references unknown model table %q", tableName, ref.Name)
		}
		seen[ref.Name] = struct{}{}
	}
	return sortedStringSet(seen), nil
}

func (m *Model) modelSQLSourceRefs(sql string) ([]string, []string, []string) {
	if sql == "" {
		return nil, nil, nil
	}
	sourceSeen := map[string]struct{}{}
	rawSeen := map[string]struct{}{}
	unqualifiedSeen := map[string]struct{}{}
	for _, ref := range scanSQLRelationRefs(sql) {
		switch ref.Namespace {
		case "source":
			sourceSeen[ref.Name] = struct{}{}
		case "raw":
			rawSeen[ref.Name] = struct{}{}
		case "":
			unqualifiedSeen[ref.Name] = struct{}{}
		}
	}
	sourceRefs := sortedStringSet(sourceSeen)
	rawRefs := sortedStringSet(rawSeen)
	unqualifiedRefs := sortedStringSet(unqualifiedSeen)
	return sourceRefs, rawRefs, unqualifiedRefs
}

func (m *Model) SQLSourceRefs(sql string) ([]string, []string, []string) {
	return m.modelSQLSourceRefs(sql)
}

func validateModelSQLQuery(tableName string, sql string) error {
	keyword, _, ok := firstSQLKeyword(sql)
	if !ok || (keyword != "select" && keyword != "with") {
		return fmt.Errorf("model table %q transform.sql must be a read-only SELECT or WITH query", tableName)
	}
	if keyword == "with" {
		start := scanSQLCTEs(sql, map[string]struct{}{}, &[]sqlRelationRef{})
		nextKeyword, _, ok := firstSQLKeyword(sql[start:])
		if !ok || nextKeyword != "select" {
			return fmt.Errorf("model table %q transform.sql must be a read-only SELECT or WITH query", tableName)
		}
	}
	return nil
}

func firstSQLKeyword(sql string) (string, int, bool) {
	for index := 0; index < len(sql); {
		switch sql[index] {
		case '\'':
			index = skipSQLSingleQuoted(sql, index)
			continue
		case '-':
			if index+1 < len(sql) && sql[index+1] == '-' {
				index = skipSQLLineComment(sql, index+2)
				continue
			}
		case '/':
			if index+1 < len(sql) && sql[index+1] == '*' {
				index = skipSQLBlockComment(sql, index+2)
				continue
			}
		}
		if isSQLIdentifierStart(sql[index]) {
			keyword, next, _ := readSQLIdentifier(sql, index)
			return strings.ToLower(keyword), next, true
		}
		index++
	}
	return "", len(sql), false
}

type sqlRelationRef struct {
	Namespace string
	Name      string
}

func scanSQLRelationRefs(sql string) []sqlRelationRef {
	return scanSQLRelationRefsWithLocals(sql, nil)
}

func scanSQLRelationRefsWithLocals(sql string, locals map[string]struct{}) []sqlRelationRef {
	refs := []sqlRelationRef{}
	localRefs := map[string]struct{}{}
	for name := range locals {
		localRefs[strings.ToLower(name)] = struct{}{}
	}
	start := scanSQLCTEs(sql, localRefs, &refs)
	for index := start; index < len(sql); {
		switch sql[index] {
		case '\'':
			index = skipSQLSingleQuoted(sql, index)
			continue
		case '-':
			if index+1 < len(sql) && sql[index+1] == '-' {
				index = skipSQLLineComment(sql, index+2)
				continue
			}
		case '/':
			if index+1 < len(sql) && sql[index+1] == '*' {
				index = skipSQLBlockComment(sql, index+2)
				continue
			}
		}
		if isSQLIdentifierStart(sql[index]) {
			keyword, next, _ := readSQLIdentifier(sql, index)
			if relationKeyword(strings.ToLower(keyword)) {
				relationRefs, relationNext := readSQLRelationList(sql, next, localRefs)
				refs = append(refs, relationRefs...)
				index = relationNext
				continue
			}
			index = next
			continue
		}
		index++
	}
	return refs
}

func scanSQLCTEs(sql string, locals map[string]struct{}, refs *[]sqlRelationRef) int {
	keyword, next, ok := firstSQLKeyword(sql)
	if !ok || keyword != "with" {
		return 0
	}
	index := skipSQLSpaces(sql, next)
	if recursive, afterRecursive, ok := readSQLIdentifier(sql, index); ok && strings.EqualFold(recursive, "recursive") {
		index = skipSQLSpaces(sql, afterRecursive)
	}
	for {
		name, afterName, ok := readSQLIdentifier(sql, index)
		if !ok {
			return index
		}
		locals[strings.ToLower(name)] = struct{}{}
		index = skipSQLSpaces(sql, afterName)
		if index < len(sql) && sql[index] == '(' {
			index = skipSQLBalanced(sql, index)
			index = skipSQLSpaces(sql, index)
		}
		asKeyword, afterAS, ok := readSQLIdentifier(sql, index)
		if !ok || !strings.EqualFold(asKeyword, "as") {
			return index
		}
		index = skipSQLSpaces(sql, afterAS)
		if index >= len(sql) || sql[index] != '(' {
			return index
		}
		inside, afterBody := readSQLBalancedContent(sql, index)
		*refs = append(*refs, scanSQLRelationRefsWithLocals(inside, locals)...)
		index = skipSQLSpaces(sql, afterBody)
		if index >= len(sql) || sql[index] != ',' {
			return index
		}
		index = skipSQLSpaces(sql, index+1)
	}
}

func relationKeyword(keyword string) bool {
	switch keyword {
	case "from", "join":
		return true
	default:
		return false
	}
}

func readSQLRelationList(sql string, index int, locals map[string]struct{}) ([]sqlRelationRef, int) {
	refs := []sqlRelationRef{}
	for {
		index = skipSQLSpaces(sql, index)
		if index >= len(sql) {
			return refs, index
		}
		if sql[index] == '(' {
			inside, next := readSQLBalancedContent(sql, index)
			refs = append(refs, scanSQLRelationRefsWithLocals(inside, locals)...)
			index = next
			return refs, index
		}
		ref, next, ok := readSQLRelationRef(sql, index, locals)
		if !ok {
			return refs, index
		}
		refs = append(refs, ref)
		index = skipSQLRelationAlias(sql, next)
		index = skipSQLSpaces(sql, index)
		if index >= len(sql) || sql[index] != ',' {
			return refs, index
		}
		index++
	}
}

func readSQLRelationRef(sql string, index int, locals map[string]struct{}) (sqlRelationRef, int, bool) {
	first, next, ok := readSQLIdentifier(sql, index)
	if !ok {
		return sqlRelationRef{}, index, false
	}
	dot := skipSQLSpaces(sql, next)
	if dot < len(sql) && sql[dot] == '.' {
		nameStart := skipSQLSpaces(sql, dot+1)
		name, afterName, ok := readSQLIdentifier(sql, nameStart)
		if !ok {
			return sqlRelationRef{}, index, false
		}
		namespace := strings.ToLower(first)
		if namespace == "source" || namespace == "raw" || namespace == "model" {
			return sqlRelationRef{Namespace: namespace, Name: name}, afterName, true
		}
		return sqlRelationRef{Name: name}, afterName, true
	}
	if _, ok := locals[strings.ToLower(first)]; ok {
		return sqlRelationRef{Namespace: "local", Name: first}, next, true
	}
	return sqlRelationRef{Name: first}, next, true
}

func readSQLIdentifier(sql string, index int) (string, int, bool) {
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
	if !isSQLIdentifierStart(sql[index]) {
		return "", index, false
	}
	cursor := index + 1
	for cursor < len(sql) && isSQLIdentifierPart(sql[cursor]) {
		cursor++
	}
	return sql[index:cursor], cursor, true
}

func skipSQLSingleQuoted(sql string, index int) int {
	for cursor := index + 1; cursor < len(sql); cursor++ {
		if sql[cursor] == '\'' {
			if cursor+1 < len(sql) && sql[cursor+1] == '\'' {
				cursor++
				continue
			}
			return cursor + 1
		}
	}
	return len(sql)
}

func skipSQLLineComment(sql string, index int) int {
	for index < len(sql) && sql[index] != '\n' && sql[index] != '\r' {
		index++
	}
	return index
}

func skipSQLBlockComment(sql string, index int) int {
	for index+1 < len(sql) {
		if sql[index] == '*' && sql[index+1] == '/' {
			return index + 2
		}
		index++
	}
	return len(sql)
}

func skipSQLBalanced(sql string, index int) int {
	_, next := readSQLBalancedContent(sql, index)
	return next
}

func readSQLBalancedContent(sql string, index int) (string, int) {
	depth := 0
	start := index + 1
	for index < len(sql) {
		switch sql[index] {
		case '\'':
			index = skipSQLSingleQuoted(sql, index)
			continue
		case '"':
			_, next, ok := readSQLIdentifier(sql, index)
			if ok {
				index = next
				continue
			}
		case '-':
			if index+1 < len(sql) && sql[index+1] == '-' {
				index = skipSQLLineComment(sql, index+2)
				continue
			}
		case '/':
			if index+1 < len(sql) && sql[index+1] == '*' {
				index = skipSQLBlockComment(sql, index+2)
				continue
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return sql[start:index], index + 1
			}
		}
		index++
	}
	return sql[start:], index
}

func skipSQLRelationAlias(sql string, index int) int {
	index = skipSQLSpaces(sql, index)
	if index >= len(sql) {
		return index
	}
	if sql[index] == '(' {
		return skipSQLBalanced(sql, index)
	}
	if sql[index] == '"' {
		_, next, ok := readSQLIdentifier(sql, index)
		if ok {
			return next
		}
		return index
	}
	if !isSQLIdentifierStart(sql[index]) {
		return index
	}
	value, next, _ := readSQLIdentifier(sql, index)
	lower := strings.ToLower(value)
	if lower == "as" {
		return skipSQLRelationAlias(sql, next)
	}
	if relationListTerminator(lower) {
		return index
	}
	return next
}

func relationListTerminator(value string) bool {
	switch value {
	case "set", "where", "group", "order", "having", "limit", "offset", "qualify", "union", "except", "intersect", "join", "left", "right", "full", "inner", "outer", "cross", "on", "using":
		return true
	default:
		return false
	}
}

func skipSQLSpaces(sql string, index int) int {
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

func isSQLIdentifierStart(char byte) bool {
	return char == '_' || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
}

func isSQLIdentifierPart(char byte) bool {
	return isSQLIdentifierStart(char) || (char >= '0' && char <= '9')
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (m *Model) validateSemanticGraph() error {
	for _, relationship := range m.Relationships {
		if relationship.Cardinality != "many_to_one" && relationship.Cardinality != "one_to_one" {
			return fmt.Errorf("unsafe relationship path: cardinality %q from %q to %q", relationship.Cardinality, relationship.From, relationship.To)
		}
		if _, err := m.validateRelationshipEndpoint("from", relationship.From); err != nil {
			return err
		}
		if _, err := m.validateRelationshipEndpoint("to", relationship.To); err != nil {
			return err
		}
	}
	return m.validateSemanticDefinitions()
}

func (m *Model) validateRelationshipEndpoint(role string, endpoint string) (string, error) {
	tableName, fieldName, err := splitSemanticField(endpoint)
	if err != nil {
		return "", fmt.Errorf("relationship %s %q: %w", role, endpoint, err)
	}
	table, ok := m.Tables[tableName]
	if !ok {
		return "", fmt.Errorf("relationship %s %q references unknown table %q", role, endpoint, tableName)
	}
	if _, ok := table.Dimensions[fieldName]; !ok {
		return "", fmt.Errorf("relationship %s %q references unknown field %q on table %q", role, endpoint, fieldName, tableName)
	}
	return tableName, nil
}

func relationshipID(relationship Relationship, index int) string {
	from := strings.ReplaceAll(relationship.From, ".", "_")
	to := strings.ReplaceAll(relationship.To, ".", "_")
	if from == "" || to == "" {
		return fmt.Sprintf("relationship_%d", index+1)
	}
	return from + "__" + to
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func titleFromIdentifier(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (m *Model) resolveSource(source Source) (Source, error) {
	switch source.Kind() {
	case KindPath, KindObject:
		if source.Connection == "" {
			source.Connection = m.DefaultConnection
		}
		if source.Connection == "" {
			return source, fmt.Errorf("requires connection")
		}
		connection, ok := m.Connections[source.Connection]
		if !ok {
			return source, fmt.Errorf("references unknown connection %q", source.Connection)
		}
		if source.Path != "" {
			if len(connection.Defaults.Options) > 0 {
				options := make(map[string]any, len(connection.Defaults.Options)+len(source.Options))
				for key, value := range connection.Defaults.Options {
					options[key] = value
				}
				for key, value := range source.Options {
					options[key] = value
				}
				source.Options = options
			}
			if source.Format == "" {
				format, ok := InferFormat(source.Path)
				if !ok {
					return source, fmt.Errorf("path %q requires format", source.Path)
				}
				source.Format = format
			}
		}
		return source, nil
	default:
		return source, nil
	}
}

func (s Source) Validate(name string, connections map[string]Connection) error {
	if err := validateSemanticIdentifier(name); err != nil {
		return fmt.Errorf("source %q has invalid name: %w", name, err)
	}
	for key := range s.Options {
		if err := validateSemanticIdentifier(key); err != nil {
			return fmt.Errorf("source %q option %q is invalid: %w", name, key, err)
		}
	}
	switch s.Kind() {
	case KindPath:
		if s.Connection == "" {
			return fmt.Errorf("source %q requires connection", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		connectionSpec, ok := LookupConnection(connection.Kind)
		if !ok || !connectionSpec.AllowsPathSource {
			return fmt.Errorf("source %q path cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
		if connection.Kind == "managed" && !IsLocalPath(s.Path) {
			return fmt.Errorf("source %q %s connection %q cannot use remote path %q", name, connection.Kind, s.Connection, s.Path)
		}
		if connection.Kind == "managed" {
			cleaned := filepath.Clean(s.Path)
			if filepath.IsAbs(s.Path) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
				return fmt.Errorf("source %q managed path %q must be relative and cannot contain traversal", name, s.Path)
			}
		}
		if !sourceWithinConnectionScope(connection, s.Path) {
			return fmt.Errorf("source %q path %q escapes connection scope", name, s.Path)
		}
		if connectionSpec.AllowsPathSource && connection.Kind != "managed" && IsLocalPath(s.Path) && connection.Scope == "" {
			return fmt.Errorf("source %q remote connection %q requires scope for relative path %q", name, s.Connection, s.Path)
		}
		if s.Format == "" {
			return fmt.Errorf("source %q path requires format", name)
		}
		formatSpec, ok := LookupFormat(s.Format)
		if !ok {
			return fmt.Errorf("source %q has unsupported format %q", name, s.Format)
		}
		if !formatSpec.AllowsOptions && len(s.Options) > 0 {
			return fmt.Errorf("source %q %s path cannot set options", name, s.Format)
		}
	case KindObject:
		if s.Connection == "" {
			return fmt.Errorf("source %q object requires connection", name)
		}
		if s.Format != "" || len(s.Options) > 0 {
			return fmt.Errorf("source %q object cannot set format or options", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		connectionSpec, ok := LookupConnection(connection.Kind)
		if !ok || !connectionSpec.AllowsObjectSource {
			return fmt.Errorf("source %q object cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
	default:
		return fmt.Errorf("source %q requires exactly one of path or object", name)
	}
	return nil
}

func sourceWithinConnectionScope(connection Connection, sourcePath string) bool {
	scope := firstNonEmpty(connection.Scope, connection.Root)
	if scope == "" {
		return true
	}
	if !IsLocalPath(scope) || !IsLocalPath(sourcePath) {
		fullPath := sourcePath
		if IsLocalPath(sourcePath) {
			fullPath = JoinScope(scope, sourcePath)
		}
		return WithinScope(scope, fullPath)
	}
	cleanScope := filepath.Clean(scope)
	cleanPath := filepath.Clean(sourcePath)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Clean(filepath.Join(cleanScope, cleanPath))
	}
	rel, err := filepath.Rel(cleanScope, cleanPath)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c Connection) Validate(name string) (Connection, error) {
	if err := validateSemanticIdentifier(name); err != nil {
		return c, fmt.Errorf("connection %q has invalid name: %w", name, err)
	}
	if c.Kind == "" {
		return c, fmt.Errorf("connection %q requires kind", name)
	}
	if err := validateConnectionCredentials(name, c.Kind, c.Scope, c.Credentials); err != nil {
		return c, err
	}
	connectionSpec, ok := LookupConnection(c.Kind)
	if !ok {
		return c, fmt.Errorf("connection %q has unsupported kind %q", name, c.Kind)
	}
	if c.Kind == "managed" && (strings.TrimSpace(c.Root) != "" || strings.TrimSpace(c.Scope) != "") {
		return c, fmt.Errorf("connection %q managed physical location is supplied by the active revision and cannot be authored", name)
	}
	if connectionSpec.RequiresPath {
		if c.Path == "" {
			return c, fmt.Errorf("connection %q %s requires path", name, c.Kind)
		}
	} else if c.Path != "" && !connectionSpec.AllowsPath {
		return c, fmt.Errorf("connection %q path is only supported for path-backed connections", name)
	}
	auth, err := validateConnectionAuth(name, c, connectionSpec)
	if err != nil {
		return c, err
	}
	c.Auth = auth
	for key := range c.Options {
		if !connectionAllowsOption(connectionSpec, key) {
			return c, fmt.Errorf("connection %q has unsupported option %q", name, key)
		}
	}
	if err := validateConnectionOptions(name, c); err != nil {
		return c, err
	}
	for key := range c.Defaults.Options {
		if err := validateSemanticIdentifier(key); err != nil {
			return c, fmt.Errorf("connection %q default option %q is invalid: %w", name, key, err)
		}
	}
	return c, nil
}

func (s Source) Role() string {
	switch s.Kind() {
	case KindPath:
		return s.Format
	case KindObject:
		return "object"
	default:
		return "source"
	}
}

func (s Source) Kind() string {
	count := 0
	kind := ""
	if s.Path != "" {
		count++
		kind = KindPath
	}
	if s.Object != "" {
		count++
		kind = KindObject
	}
	if count != 1 {
		return ""
	}
	return kind
}

func connectionAllowsOption(connection ConnectionSpec, option string) bool {
	for _, allowed := range connection.AllowedOptions {
		if option == allowed {
			return true
		}
	}
	return false
}

func validateConnectionOptions(name string, connection Connection) error {
	switch connection.Kind {
	case "quack":
		if !strings.HasPrefix(connection.Path, "quack:") {
			return fmt.Errorf("connection %q quack path must start with quack:", name)
		}
		if value, ok := connection.Options["disable_ssl"]; ok {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("connection %q disable_ssl option must be a boolean", name)
			}
		}
	}
	return nil
}

func validateConnectionCredentials(name, kind, scope string, credentials ConnectionCredentials) error {
	if credentials.Provider == "" && credentials.Secret == "" && credentials.Region == "" && credentials.Endpoint == "" && credentials.AccountName == "" {
		return nil
	}
	if credentials.Provider == "" {
		return fmt.Errorf("connection %q credentials require provider", name)
	}
	switch credentials.Provider {
	case "none":
		if credentials.Secret != "" || credentials.Region != "" || credentials.Endpoint != "" || credentials.AccountName != "" {
			return fmt.Errorf("connection %q none credentials cannot set credential values", name)
		}
	case "env":
		if credentials.Secret == "" {
			return fmt.Errorf("connection %q env credentials require secret", name)
		}
		if _, ok := os.LookupEnv(credentials.Secret); !ok {
			return fmt.Errorf("connection %q env credential %q is not set", name, credentials.Secret)
		}
		if credentials.Region != "" || credentials.Endpoint != "" || credentials.AccountName != "" {
			return fmt.Errorf("connection %q env credentials cannot set ambient metadata", name)
		}
	case "ambient":
		if credentials.Secret != "" {
			return fmt.Errorf("connection %q ambient credentials cannot set secret", name)
		}
		if strings.TrimSpace(scope) == "" {
			return fmt.Errorf("connection %q ambient credentials require a path scope", name)
		}
		switch kind {
		case "s3":
			if credentials.AccountName != "" {
				return fmt.Errorf("connection %q s3 ambient credentials cannot set accountName", name)
			}
		case "azure_blob":
			if strings.TrimSpace(credentials.AccountName) == "" {
				return fmt.Errorf("connection %q azure_blob ambient credentials require accountName", name)
			}
			if credentials.Region != "" || credentials.Endpoint != "" {
				return fmt.Errorf("connection %q azure_blob ambient credentials accept only accountName", name)
			}
		default:
			return fmt.Errorf("connection %q kind %q does not support ambient credentials", name, kind)
		}
	default:
		return fmt.Errorf("connection %q has unsupported credentials provider %q", name, credentials.Provider)
	}
	return nil
}

func validateConnectionAuth(name string, connection Connection, spec ConnectionSpec) (ConnectionAuth, error) {
	if len(connection.Auth) == 0 {
		if connection.Credentials.Provider == "ambient" {
			return nil, nil
		}
		if connection.Credentials.Provider == "none" && connection.Kind == "s3" {
			return nil, nil
		}
		if connection.Credentials.Provider != "" && connection.Credentials.Provider != "none" {
			resolved, err := ResolveConnectionAuth(connection)
			if err != nil {
				return nil, fmt.Errorf("connection %q credentials: %w", name, err)
			}
			for key := range resolved {
				if err := validateSemanticIdentifier(key); err != nil {
					return nil, fmt.Errorf("connection %q credential key %q is invalid: %w", name, key, err)
				}
				if !connectionAllowsAuthKey(spec, key) {
					return nil, fmt.Errorf("connection %q credentials include unsupported auth key %q", name, key)
				}
			}
			if !connectionHasRequiredAuth(resolved, spec.RequiredAuthSets) {
				return nil, fmt.Errorf("connection %q %s credentials are missing required values", name, connection.Kind)
			}
			return nil, nil
		}
		if connection.Kind == "ducklake" && duckLakeNeedsAuth(connection) {
			return nil, fmt.Errorf("connection %q ducklake remote path requires auth", name)
		}
		if connection.Kind == "sqlite" && connection.Options["path"] != nil {
			return nil, nil
		}
		if spec.AllowNoAuth {
			return nil, nil
		}
		return nil, fmt.Errorf("connection %q %s requires auth", name, connection.Kind)
	}
	resolved := make(ConnectionAuth, len(connection.Auth))
	for key, value := range connection.Auth {
		if err := validateSemanticIdentifier(key); err != nil {
			return nil, fmt.Errorf("connection %q auth key %q is invalid: %w", name, key, err)
		}
		if !connectionAllowsAuthKey(spec, key) {
			return nil, fmt.Errorf("connection %q has unsupported auth key %q", name, key)
		}
		resolvedValue, err := resolveAuthValue(name, key, value)
		if err != nil {
			return nil, err
		}
		resolved[key] = resolvedValue
	}
	if !connectionHasRequiredAuth(resolved, spec.RequiredAuthSets) {
		return nil, fmt.Errorf("connection %q %s auth is missing required credentials", name, connection.Kind)
	}
	return resolved, nil
}

func ResolveConnectionAuth(connection Connection) (ConnectionAuth, error) {
	if len(connection.Auth) > 0 {
		return connection.Auth, nil
	}
	if connection.Credentials.Provider == "" || connection.Credentials.Provider == "none" {
		return nil, nil
	}
	switch connection.Credentials.Provider {
	case "env":
		value, ok := os.LookupEnv(connection.Credentials.Secret)
		if !ok {
			return nil, fmt.Errorf("env credential %q is not set", connection.Credentials.Secret)
		}
		var object map[string]any
		if err := json.Unmarshal([]byte(value), &object); err == nil {
			return ConnectionAuth(object), nil
		}
		spec, ok := LookupConnection(connection.Kind)
		if !ok {
			return nil, fmt.Errorf("unsupported connection kind %q", connection.Kind)
		}
		for _, key := range []string{"connection_string", "token"} {
			if connectionAllowsAuthKey(spec, key) {
				return ConnectionAuth{key: value}, nil
			}
		}
		return nil, fmt.Errorf("env credential %q must be a JSON object for connection kind %q", connection.Credentials.Secret, connection.Kind)
	case "ambient":
		auth := ConnectionAuth{}
		if connection.Credentials.Region != "" {
			auth["region"] = connection.Credentials.Region
		}
		if connection.Credentials.Endpoint != "" {
			auth["endpoint"] = connection.Credentials.Endpoint
		}
		if connection.Credentials.AccountName != "" {
			auth["account_name"] = connection.Credentials.AccountName
		}
		return auth, nil
	default:
		return nil, fmt.Errorf("unsupported credentials provider %q", connection.Credentials.Provider)
	}
}

func ConnectionCredentialsConfigured(connection Connection) bool {
	return len(connection.Auth) > 0 || connection.Credentials.Provider != "" && connection.Credentials.Provider != "none"
}

func connectionAllowsAuthKey(connection ConnectionSpec, key string) bool {
	for _, allowed := range connection.AuthKeys {
		if key == allowed {
			return true
		}
	}
	return false
}

func connectionHasRequiredAuth(auth ConnectionAuth, requiredSets [][]string) bool {
	if len(requiredSets) == 0 {
		return true
	}
	for _, required := range requiredSets {
		missing := false
		for _, key := range required {
			value, ok := auth[key]
			if !ok || fmt.Sprint(value) == "" {
				missing = true
				break
			}
		}
		if !missing {
			return true
		}
	}
	return false
}

func resolveAuthValue(connectionName, key string, value any) (any, error) {
	switch typed := value.(type) {
	case string:
		if matches := envReferencePattern.FindStringSubmatch(typed); matches != nil {
			envName := matches[1]
			resolved, ok := os.LookupEnv(envName)
			if !ok || resolved == "" {
				return nil, fmt.Errorf("connection %q auth key %q references missing environment variable %s", connectionName, key, envName)
			}
			return resolved, nil
		}
		if typed == "" {
			return nil, fmt.Errorf("connection %q auth key %q cannot be empty", connectionName, key)
		}
		return typed, nil
	case bool, int, int64, float64:
		return typed, nil
	default:
		return nil, fmt.Errorf("connection %q auth key %q has unsupported value type %T", connectionName, key, value)
	}
}

func duckLakeNeedsAuth(connection Connection) bool {
	if connection.Scope != "" && !IsLocalPath(connection.Scope) {
		return true
	}
	if connection.Path != "" && !IsLocalPath(connection.Path) {
		return true
	}
	if dataPath, ok := connection.Options["data_path"]; ok && !IsLocalPath(fmt.Sprint(dataPath)) {
		return true
	}
	return false
}

func validateSemanticIdentifier(value string) error {
	if !semanticIdentifierPattern.MatchString(value) {
		return fmt.Errorf("must match %s", semanticIdentifierPattern.String())
	}
	return nil
}

func (m *Model) TableNames() []string {
	names := make([]string, 0, len(m.Tables))
	for name := range m.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
