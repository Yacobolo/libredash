package reportmodel

import (
	"fmt"
	"sort"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard/report"
)

func FilterAppliesToTarget(d *report.Dashboard, model *semanticmodel.Model, filter report.FilterDefinition, targetKind, targetID string) (bool, error) {
	targeted := !filter.Targets.IsEmpty()
	if targeted && !filter.Targets.Contains(targetKind, targetID) {
		return false, nil
	}
	facts, err := TargetFacts(d, model, targetKind, targetID)
	if err != nil {
		return false, err
	}
	if dimension, ok := model.Dimensions[filter.Dimension]; ok {
		for _, fact := range facts {
			if _, ok := dimension.Bindings[fact]; !ok {
				if targeted {
					return false, fmt.Errorf("semantic dimension %q has no binding for fact %q", filter.Dimension, fact)
				}
				return false, nil
			}
		}
		return true, nil
	}
	if len(facts) != 1 {
		if filter.Fact == "" {
			return false, nil
		}
		for _, fact := range facts {
			if fact == filter.Fact {
				return model.CanReachField(fact, filter.Dimension) == nil, nil
			}
		}
		return false, nil
	}
	if err := model.CanReachField(facts[0], filter.Dimension); err != nil {
		if targeted {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func TargetFacts(d *report.Dashboard, model *semanticmodel.Model, targetKind, targetID string) ([]string, error) {
	var table string
	var measures []report.FieldRef
	switch targetKind {
	case "visual":
		if visual, ok := d.Visuals[targetID]; ok {
			if visual.Chart != nil {
				table, measures = visual.Chart.Query.Table, visual.Chart.Query.Measures
			} else if visual.Tabular != nil {
				table, measures = visual.Tabular.Query.Table, visual.Tabular.Query.Measures
			}
		} else {
			return nil, fmt.Errorf("unknown target visual %q", targetID)
		}
	default:
		return nil, fmt.Errorf("unknown target kind %q", targetKind)
	}
	if table != "" {
		if _, ok := model.Tables[table]; !ok {
			return nil, fmt.Errorf("query references unknown table %q", table)
		}
		return []string{table}, nil
	}
	factSet := map[string]struct{}{}
	var addMember func(string) error
	visiting := map[string]bool{}
	addMember = func(name string) error {
		if measure, ok := model.Measures[name]; ok {
			factSet[measure.Fact] = struct{}{}
			return nil
		}
		metric, ok := model.Metrics[name]
		if !ok {
			return fmt.Errorf("unknown measure or metric %q", name)
		}
		if visiting[name] {
			return fmt.Errorf("metric dependency cycle includes %q", name)
		}
		visiting[name] = true
		expression, err := semanticmodel.ParseExpression(metric.Expression)
		if err != nil {
			return err
		}
		for _, ref := range expression.References() {
			if err := addMember(ref); err != nil {
				return err
			}
		}
		delete(visiting, name)
		return nil
	}
	for _, measure := range measures {
		if err := addMember(measure.Field); err != nil {
			return nil, err
		}
	}
	facts := make([]string, 0, len(factSet))
	for fact := range factSet {
		facts = append(facts, fact)
	}
	sort.Strings(facts)
	if len(facts) == 0 {
		return nil, fmt.Errorf("query requires at least one fact")
	}
	return facts, nil
}

func TargetBaseTable(d *report.Dashboard, model *semanticmodel.Model, targetKind, targetID string) (string, error) {
	facts, err := TargetFacts(d, model, targetKind, targetID)
	if err != nil {
		return "", err
	}
	if len(facts) != 1 {
		return "", fmt.Errorf("target uses multiple facts")
	}
	return facts[0], nil
}
