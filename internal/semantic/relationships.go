package semantic

import (
	"fmt"
	"sort"
)

func (m *Model) SafeRelationshipPath(base, target string) ([]Relationship, error) {
	if base == target {
		return nil, nil
	}
	matches := m.safeRelationshipPaths(base, target)
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no safe relationship path from %q to %q", base, target)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous relationship path from %q to %q", base, target)
	}
}

func (m *Model) CanReachField(baseTable, field string) error {
	dimension, err := m.ResolveDimension(field)
	if err != nil {
		return err
	}
	_, err = m.SafeRelationshipPath(baseTable, dimension.Table)
	return err
}

func (m *Model) safeEdgesFrom(table string) []relationshipEdge {
	edges := []relationshipEdge{}
	for _, relationship := range m.Relationships {
		edge, ok := semanticSafeEdgeFrom(table, relationship)
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

func (m *Model) safeRelationshipPaths(base, target string) [][]Relationship {
	matches := [][]Relationship{}
	var walk func(candidate relationshipPathCandidate)
	walk = func(candidate relationshipPathCandidate) {
		if len(matches) > 1 {
			return
		}
		for _, edge := range m.safeEdgesFrom(candidate.Table) {
			if len(matches) > 1 {
				return
			}
			if candidate.Visited[edge.Table] {
				continue
			}
			path := append(append([]Relationship{}, candidate.Path...), edge.Relationship)
			if edge.Table == target {
				matches = append(matches, path)
				continue
			}
			visited := map[string]bool{}
			for table, value := range candidate.Visited {
				visited[table] = value
			}
			visited[edge.Table] = true
			walk(relationshipPathCandidate{Table: edge.Table, Path: path, Visited: visited})
		}
	}
	walk(relationshipPathCandidate{Table: base, Visited: map[string]bool{base: true}})
	return matches
}

func semanticSafeEdgeFrom(table string, relationship Relationship) (relationshipEdge, bool) {
	if !relationship.Active {
		return relationshipEdge{}, false
	}
	fromTable, _, err := splitSemanticField(relationship.From)
	if err != nil {
		return relationshipEdge{}, false
	}
	toTable, _, err := splitSemanticField(relationship.To)
	if err != nil {
		return relationshipEdge{}, false
	}
	if fromTable == table && semanticSafeCardinality(relationship.Cardinality) {
		return relationshipEdge{Table: toTable, Relationship: relationship}, true
	}
	if relationship.Cardinality == "one_to_one" && toTable == table {
		return relationshipEdge{Table: fromTable, Relationship: relationship}, true
	}
	return relationshipEdge{}, false
}

func semanticSafeCardinality(cardinality string) bool {
	return cardinality == "many_to_one" || cardinality == "one_to_one"
}

type relationshipPathCandidate struct {
	Table   string
	Path    []Relationship
	Visited map[string]bool
}

type relationshipEdge struct {
	Table        string
	Relationship Relationship
}
