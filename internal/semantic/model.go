package semantic

import (
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/analytics/model"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*model.Model, error) {
	return nil, fmt.Errorf("legacy semantic model files are not supported; use libredash.dev/v1 SemanticModel resources")
}

func (m *semanticModelMeasures) UnmarshalYAML(value *yaml.Node) error {
	m.Items = map[string]model.MetricMeasure{}
	if value == nil || value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("semantic model measures must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		if key == "defaults" {
			if err := item.Decode(&m.Defaults); err != nil {
				return err
			}
			continue
		}
		if err := validateSemanticIdentifier(key); err != nil {
			return fmt.Errorf("measure %q is invalid: %w", key, err)
		}
		measure := model.MetricMeasure{}
		if item.Kind != yaml.ScalarNode || item.Tag != "!!null" {
			if err := item.Decode(&measure); err != nil {
				return err
			}
		}
		m.Items[key] = measure
	}
	return nil
}

func rejectSourceBusinessSemantics(content []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	sources := mappingValue(root, "sources")
	if sources == nil || sources.Kind != yaml.MappingNode {
		return nil
	}
	for index := 1; index < len(sources.Content); index += 2 {
		source := sources.Content[index]
		if source.Kind != yaml.MappingNode {
			continue
		}
		for child := 0; child+1 < len(source.Content); child += 2 {
			switch source.Content[child].Value {
			case "dimensions", "measures", "relationships", "primary_key", "grain":
				return fmt.Errorf("sources do not define business semantics")
			}
		}
	}
	return nil
}

func (file modelFile) compile() (*model.Model, error) {
	semanticModel := &model.Model{
		Name:              file.Name,
		Title:             file.Title,
		Description:       file.Description,
		DefaultConnection: file.DefaultConnection,
		Connections:       file.Connections,
		Sources:           file.Sources,
	}
	if len(file.SemanticModels) == 0 {
		return nil, fmt.Errorf("semantic model %q is not defined under semantic_models", file.Name)
	}
	spec, ok := file.SemanticModels[file.Name]
	if !ok && len(file.SemanticModels) == 1 {
		for _, candidate := range file.SemanticModels {
			spec = candidate
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("semantic model %q is not defined under semantic_models", file.Name)
	}
	if len(spec.Tables) == 0 {
		return nil, fmt.Errorf("semantic model %q requires tables", file.Name)
	}
	if spec.BaseTable == "" {
		return nil, fmt.Errorf("semantic model %q requires base_table", file.Name)
	}
	if len(file.Models) == 0 {
		return nil, fmt.Errorf("semantic model %q requires models", file.Name)
	}
	tables := map[string]model.Table{}
	for _, tableName := range spec.Tables {
		if err := validateSemanticIdentifier(tableName); err != nil {
			return nil, fmt.Errorf("semantic model table %q is invalid: %w", tableName, err)
		}
		modelTable, ok := file.Models[tableName]
		if !ok {
			return nil, fmt.Errorf("semantic model table %q references unknown model", tableName)
		}
		if modelTable.SQL != "" && modelTable.Transform.SQL == "" {
			modelTable.Transform.SQL = modelTable.SQL
		}
		modelTable.Kind = defaultString(modelTable.Kind, "model")
		if modelTable.Grain == "" {
			modelTable.Grain = modelTable.PrimaryKey
		}
		if modelTable.Dimensions == nil {
			modelTable.Dimensions = map[string]model.MetricDimension{}
		}
		if modelTable.Columns == nil {
			modelTable.Columns = map[string]model.ModelColumn{}
		}
		for columnName, column := range modelTable.Columns {
			if err := validateSemanticIdentifier(columnName); err != nil {
				return nil, fmt.Errorf("semantic model table %q column %q is invalid: %w", tableName, columnName, err)
			}
			column.Name = columnName
			column.Field = tableName + "." + columnName
			if column.SourceField == "" {
				column.SourceField = columnName
			}
			modelTable.Columns[columnName] = column
		}
		for field, dimension := range modelTable.Dimensions {
			if err := validateSemanticIdentifier(field); err != nil {
				return nil, fmt.Errorf("semantic model table %q field %q is invalid: %w", tableName, field, err)
			}
			dimension.Table = tableName
			dimension.Name = field
			dimension.Field = tableName + "." + field
			if dimension.Label == "" {
				dimension.Label = titleFromIdentifier(field)
			}
			modelTable.Dimensions[field] = dimension
		}
		if modelTable.Measures == nil {
			modelTable.Measures = map[string]model.MetricMeasure{}
		}
		tables[tableName] = modelTable
	}
	if _, ok := tables[spec.BaseTable]; !ok {
		return nil, fmt.Errorf("semantic model %q base_table %q references unknown table", file.Name, spec.BaseTable)
	}
	defaults := spec.Measures.Defaults
	if defaults.Table == "" && len(spec.Measures.Items) > 0 {
		return nil, fmt.Errorf("semantic model %q measure defaults require table", file.Name)
	}
	measures := map[string]model.MetricMeasure{}
	for name, measure := range spec.Measures.Items {
		tableName := defaultString(measure.Table, defaults.Table)
		table, ok := tables[tableName]
		if !ok {
			return nil, fmt.Errorf("semantic model measure %q references unknown table %q", name, tableName)
		}
		if measure.Expression == "" {
			measure.Expression = measure.Expr
		}
		if measure.Expression == "" {
			return nil, fmt.Errorf("semantic model measure %q requires expr", name)
		}
		measure.Field = name
		measure.Name = name
		measure.Table = tableName
		measure.Label = defaultString(measure.Label, titleFromIdentifier(name))
		measure.Grain = defaultString(measure.Grain, defaults.Grain)
		measure.Time = defaultString(measure.Time, defaults.Time)
		if len(measure.Grains) == 0 {
			measure.Grains = append([]string{}, defaults.Grains...)
		}
		table.Measures[name] = measure
		tables[tableName] = table
		measures[name] = measure
	}
	semanticModel.Tables = tables
	semanticModel.BaseTable = spec.BaseTable
	semanticModel.Relationships = spec.Relationships
	for index, relationship := range semanticModel.Relationships {
		if relationship.ID == "" {
			relationship.ID = relationshipID(relationship, index)
			semanticModel.Relationships[index] = relationship
		}
	}
	semanticModel.Measures = measures
	return semanticModel, nil
}

func relationshipID(relationship model.Relationship, index int) string {
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

func validateSemanticIdentifier(value string) error {
	if !semanticIdentifierPattern.MatchString(value) {
		return fmt.Errorf("must match %s", semanticIdentifierPattern.String())
	}
	return nil
}
