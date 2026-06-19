package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/semantic"
)

type Planner struct {
	Model *semantic.Model
	Views map[string]*semantic.MetricView
}

type tableAlias struct {
	Table string
	Alias string
	Path  []semantic.Relationship
}

func NewPlanner(model *semantic.Model, views map[string]*semantic.MetricView) *Planner {
	return &Planner{Model: model, Views: views}
}

func (p *Planner) metricView(id string) (*semantic.MetricView, error) {
	view, ok := p.Views[id]
	if !ok {
		return nil, fmt.Errorf("unknown metric view %q", id)
	}
	if view.SemanticModel != p.Model.Name {
		return nil, fmt.Errorf("metric view %q belongs to model %q, want %q", id, view.SemanticModel, p.Model.Name)
	}
	return view, nil
}

func (p *Planner) aliases(view *semantic.MetricView, fields []string) (map[string]tableAlias, error) {
	aliases := map[string]tableAlias{
		view.BaseTable: {Table: view.BaseTable, Alias: "t0"},
	}
	nextAlias := 1
	for _, field := range fields {
		table, _, err := splitField(field)
		if err != nil {
			return nil, err
		}
		if _, ok := aliases[table]; ok {
			continue
		}
		path, err := p.relationshipPath(view.BaseTable, table)
		if err != nil {
			return nil, err
		}
		for _, step := range pathTables(view.BaseTable, path) {
			if _, ok := aliases[step.Table]; ok {
				continue
			}
			aliases[step.Table] = tableAlias{Table: step.Table, Alias: fmt.Sprintf("t%d", nextAlias), Path: step.Path}
			nextAlias++
		}
	}
	return aliases, nil
}

func (p *Planner) relationshipPath(base, target string) ([]semantic.Relationship, error) {
	if base == target {
		return nil, nil
	}

	frontier := []pathCandidate{{Table: base, Visited: map[string]bool{base: true}}}
	for len(frontier) > 0 {
		next := []pathCandidate{}
		matches := [][]semantic.Relationship{}
		for _, candidate := range frontier {
			for _, edge := range p.safeEdgesFrom(candidate.Table) {
				if candidate.Visited[edge.Table] {
					continue
				}
				path := append(append([]semantic.Relationship{}, candidate.Path...), edge.Relationship)
				if edge.Table == target {
					matches = append(matches, path)
					continue
				}
				visited := copyVisited(candidate.Visited)
				visited[edge.Table] = true
				next = append(next, pathCandidate{Table: edge.Table, Path: path, Visited: visited})
			}
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("ambiguous relationship path from %q to %q", base, target)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		frontier = next
	}
	return nil, fmt.Errorf("no safe relationship path from %q to %q", base, target)
}

func (p *Planner) safeEdgesFrom(table string) []relationshipEdge {
	edges := []relationshipEdge{}
	for _, relationship := range p.Model.Relationships {
		edge, ok := safeEdgeFrom(table, relationship)
		if ok {
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Table != edges[j].Table {
			return edges[i].Table < edges[j].Table
		}
		if edges[i].Relationship.ID != edges[j].Relationship.ID {
			return edges[i].Relationship.ID < edges[j].Relationship.ID
		}
		if edges[i].Relationship.From != edges[j].Relationship.From {
			return edges[i].Relationship.From < edges[j].Relationship.From
		}
		return edges[i].Relationship.To < edges[j].Relationship.To
	})
	return edges
}

func safeEdgeFrom(table string, relationship semantic.Relationship) (relationshipEdge, bool) {
	if !relationship.Active {
		return relationshipEdge{}, false
	}
	fromTable, _, err := splitField(relationship.From)
	if err != nil {
		return relationshipEdge{}, false
	}
	toTable, _, err := splitField(relationship.To)
	if err != nil {
		return relationshipEdge{}, false
	}
	if fromTable == table && safeCardinality(relationship.Cardinality) {
		return relationshipEdge{Table: toTable, Relationship: relationship}, true
	}
	if relationship.Cardinality == "one_to_one" && toTable == table {
		return relationshipEdge{Table: fromTable, Relationship: relationship}, true
	}
	return relationshipEdge{}, false
}

func pathTables(base string, path []semantic.Relationship) []tablePath {
	current := base
	tables := []tablePath{}
	for index, relationship := range path {
		fromTable, _, err := splitField(relationship.From)
		if err != nil {
			return tables
		}
		toTable, _, err := splitField(relationship.To)
		if err != nil {
			return tables
		}
		next := ""
		switch {
		case current == fromTable:
			next = toTable
		case relationship.Cardinality == "one_to_one" && current == toTable:
			next = fromTable
		default:
			return tables
		}
		tables = append(tables, tablePath{Table: next, Path: append([]semantic.Relationship{}, path[:index+1]...)})
		current = next
	}
	return tables
}

func copyVisited(values map[string]bool) map[string]bool {
	next := make(map[string]bool, len(values)+1)
	for key, value := range values {
		next[key] = value
	}
	return next
}

type pathCandidate struct {
	Table   string
	Path    []semantic.Relationship
	Visited map[string]bool
}

type relationshipEdge struct {
	Table        string
	Relationship semantic.Relationship
}

type tablePath struct {
	Table string
	Path  []semantic.Relationship
}

func safeCardinality(cardinality string) bool {
	return cardinality == "many_to_one" || cardinality == "one_to_one"
}

func splitField(field string) (string, string, error) {
	parts := strings.Split(field, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("field %q must be qualified as table.field", field)
	}
	return parts[0], parts[1], nil
}
