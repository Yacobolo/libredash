package query

import (
	"fmt"
	"sort"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
)

// planMultiFactBundle builds one expanded grouping aggregate per participating
// fact, then applies the ordinary null-safe full-outer stitch before decoding
// consumer branches. Each governed fact relation is scanned exactly once.
func (p *Planner) planMultiFactBundle(requests []BundleRequest, resolutions []aggregateResolution) (BundlePlan, error) {
	dimensions, branchDimensions, err := multiFactBundleDimensions(resolutions)
	if err != nil {
		return BundlePlan{}, err
	}
	groupSets, branchGroups := bundleGroups(branchDimensions)
	measures := map[string]ResolvedMeasure{}
	metrics := map[string]semanticmodel.Expression{}
	for _, resolved := range resolutions {
		for name, measure := range resolved.Measures {
			measures[name] = measure
		}
		for name, metric := range resolved.Metrics {
			metrics[name] = metric
		}
	}
	measureNames := sortedMeasureNames(measures)
	measureColumns := map[string]string{}
	for i, name := range measureNames {
		measureColumns[name] = fmt.Sprintf("__m%d", i)
	}

	ctes := []string{}
	args := []any{}
	dependencies := map[string]struct{}{}
	for factIndex, fact := range resolutions[0].Facts {
		factCTEs, factArgs, factDependencies, compileErr := p.compileMultiFactBundleFact(requests, resolutions, dimensions, groupSets, branchGroups, measures, measureNames, measureColumns, fact, factIndex)
		if compileErr != nil {
			return BundlePlan{}, compileErr
		}
		ctes = append(ctes, factCTEs...)
		args = append(args, factArgs...)
		for _, dependency := range factDependencies {
			dependencies[dependency] = struct{}{}
		}
	}
	stitched, stitchCTEs := stitchBundleFacts(resolutions[0].Facts, dimensions, measures, measureNames, measureColumns)
	ctes = append(ctes, stitchCTEs...)

	memberNames, memberColumns := bundleMemberColumns(resolutions)
	memberExpr, err := renderBundleMembers(measures, metrics, measureColumns, memberNames)
	if err != nil {
		return BundlePlan{}, err
	}
	physicalColumns := []string{BundleBranchColumn}
	for i := range dimensions {
		physicalColumns = append(physicalColumns, fmt.Sprintf("__d%d", i))
	}
	physicalColumns = append(physicalColumns, memberColumns...)
	branches, branchSQL, err := renderMultiFactBundleBranches(requests, resolutions, branchDimensions, branchGroups, dimensions, memberNames, memberColumns, memberExpr, physicalColumns, stitched)
	if err != nil {
		return BundlePlan{}, err
	}
	physicalColumns = append(physicalColumns, BundleRowColumn)
	deps := make([]string, 0, len(dependencies))
	for dependency := range dependencies {
		deps = append(deps, dependency)
	}
	sort.Strings(deps)
	sql := "WITH " + strings.Join(ctes, ",\n") + "\n" + bundleUnionSQL(branchSQL) + "\nORDER BY " + BundleBranchColumn + " ASC, " + BundleRowColumn + " ASC"
	return BundlePlan{Plan: Plan{SQL: sql, Args: args, Columns: physicalColumns, Mode: "multi_fact", Facts: append([]string{}, resolutions[0].Facts...), PhysicalDependencies: deps}, Branches: branches}, nil
}

func multiFactBundleDimensions(resolutions []aggregateResolution) ([]bundleDimension, [][]int, error) {
	dimensions := []bundleDimension{}
	indexes := map[string]int{}
	branches := make([][]int, len(resolutions))
	for branchIndex, resolved := range resolutions {
		for _, dimension := range resolved.Dimensions {
			if !dimension.Semantic {
				return nil, nil, fmt.Errorf("multi-fact bundle dimension %q is not conformed", dimension.Name)
			}
			key := bundleDimensionKey(dimension)
			index, ok := indexes[key]
			if !ok {
				index = len(dimensions)
				indexes[key] = index
				dimensions = append(dimensions, bundleDimension{dimension: dimension, physical: fmt.Sprintf("__d%d", index)})
			}
			branches[branchIndex] = append(branches[branchIndex], index)
		}
	}
	return dimensions, branches, nil
}

func bundleGroups(branchDimensions [][]int) ([][]int, []int) {
	groups := [][]int{}
	byKey := map[string]int{}
	branchGroups := make([]int, len(branchDimensions))
	for branchIndex, indexes := range branchDimensions {
		parts := make([]string, len(indexes))
		for i, index := range indexes {
			parts[i] = fmt.Sprint(index)
		}
		key := strings.Join(parts, ",")
		group, ok := byKey[key]
		if !ok {
			group = len(groups)
			byKey[key] = group
			groups = append(groups, append([]int{}, indexes...))
		}
		branchGroups[branchIndex] = group
	}
	return groups, branchGroups
}

func (p *Planner) compileMultiFactBundleFact(requests []BundleRequest, resolutions []aggregateResolution, dimensions []bundleDimension, groupSets [][]int, branchGroups []int, measures map[string]ResolvedMeasure, measureNames []string, measureColumns map[string]string, fact string, factIndex int) ([]string, []any, []string, error) {
	bindings := []physicalFieldBinding{}
	dependencies := map[string]struct{}{fact: {}}
	for _, item := range dimensions {
		field, path, err := p.aggregateDimensionBinding(fact, item.dimension)
		if err != nil {
			return nil, nil, nil, err
		}
		bindings = append(bindings, physicalFieldBinding{Field: field, Path: path})
		dependencies[field] = struct{}{}
		addPathDependencies(dependencies, path)
	}
	for _, measure := range measures {
		if measure.Fact != fact {
			continue
		}
		for _, field := range measurePhysicalFields(measure) {
			physical, err := p.Model.ResolveDimension(field)
			if err != nil {
				return nil, nil, nil, err
			}
			path, err := p.relationshipPath(fact, physical.Table)
			if err != nil {
				return nil, nil, nil, err
			}
			bindings = append(bindings, physicalFieldBinding{Field: field, Path: path})
			dependencies[field] = struct{}{}
			addPathDependencies(dependencies, path)
		}
	}
	filterBindings, err := p.factFilterFields(requests[0].Request.Filters, resolutions[0], fact)
	if err != nil {
		return nil, nil, nil, err
	}
	bindings = append(bindings, filterBindings...)
	for _, binding := range filterBindings {
		dependencies[binding.Field] = struct{}{}
		addPathDependencies(dependencies, binding.Path)
	}
	aliases, err := p.aliasesForFact(fact, bindings)
	if err != nil {
		return nil, nil, nil, err
	}
	from, err := joinPathSQL(p.Model, aliases)
	if err != nil {
		return nil, nil, nil, err
	}
	baseSelects := []string{}
	baseArgs := []any{}
	for dimensionIndex, item := range dimensions {
		field, path, err := p.aggregateDimensionBinding(fact, item.dimension)
		if err != nil {
			return nil, nil, nil, err
		}
		physical, err := p.Model.ResolveDimension(field)
		if err != nil {
			return nil, nil, nil, err
		}
		expr, err := dimensionExprForPath(physical, aliases, path)
		if err != nil {
			return nil, nil, nil, err
		}
		expr = canonicalDimensionExpr(expr, item.dimension.Type)
		if item.dimension.Grain != "" {
			expr = "DATE_TRUNC('" + item.dimension.Grain + "', " + expr + ")"
		}
		baseSelects = append(baseSelects, expr+fmt.Sprintf(" AS __d%d", dimensionIndex))
	}
	factAliases, err := aliases.context(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	for measureIndex, name := range measureNames {
		measure := measures[name]
		if measure.Fact != fact {
			continue
		}
		if measure.Aggregation != "count" {
			raw, err := rawMeasureExpr(p.Model, measure, factAliases)
			if err != nil {
				return nil, nil, nil, err
			}
			baseSelects = append(baseSelects, raw+fmt.Sprintf(" AS __v%d", measureIndex))
		}
		if len(measure.Filters) > 0 {
			parts := []string{}
			for _, filter := range measure.Filters {
				physical, err := p.Model.ResolveDimension(filter.Field)
				if err != nil {
					return nil, nil, nil, err
				}
				path, err := p.relationshipPath(fact, physical.Table)
				if err != nil {
					return nil, nil, nil, err
				}
				filterExpr, err := dimensionExprForPath(physical, aliases, path)
				if err != nil {
					return nil, nil, nil, err
				}
				part, filterArgs, err := filterSQL(filterExpr, Filter{Operator: filter.Operator, Values: filter.Values})
				if err != nil {
					return nil, nil, nil, err
				}
				if part != "" {
					parts = append(parts, part)
					baseArgs = append(baseArgs, filterArgs...)
				}
			}
			baseSelects = append(baseSelects, "("+strings.Join(parts, " AND ")+") AS "+fmt.Sprintf("__f%d", measureIndex))
		}
	}
	// A scalar count-only fact has neither dimension nor raw input columns. It
	// still needs a syntactically valid governed base whose rows COUNT(*) can
	// consume, including when the physical fact is empty.
	if len(baseSelects) == 0 {
		baseSelects = append(baseSelects, "1 AS __row")
	}
	where, whereArgs, err := p.factWhereParts(requests[0].Request.Filters, resolutions[0], fact, aliases)
	if err != nil {
		return nil, nil, nil, err
	}
	baseArgs = append(baseArgs, whereArgs...)
	baseName := fmt.Sprintf("bundle_base_%d", factIndex)
	expandedName := fmt.Sprintf("bundle_expanded_%d", factIndex)
	aggregateName := fmt.Sprintf("bundle_fact_%d", factIndex)
	base := baseName + " AS (\n  SELECT " + strings.Join(baseSelects, ", ") + "\n  FROM " + strings.ReplaceAll(from, "\n", "\n  ")
	if len(where) > 0 {
		base += "\n  WHERE " + strings.Join(where, " AND ")
	}
	base += "\n)"
	groupIDs := make([]string, len(groupSets))
	for i := range groupSets {
		groupIDs[i] = fmt.Sprint(i)
	}
	expanded := expandedName + " AS (\n  SELECT " + baseName + ".*, __bundle_group\n  FROM " + baseName + "\n  CROSS JOIN UNNEST([" + strings.Join(groupIDs, ", ") + "]) AS groups(__bundle_group)\n)"
	selects := []string{"__bundle_group"}
	for dimensionIndex := range dimensions {
		groups := []int{}
		for groupIndex, indexes := range groupSets {
			if containsInt(indexes, dimensionIndex) {
				groups = append(groups, groupIndex)
			}
		}
		selects = append(selects, fmt.Sprintf("CASE WHEN %s THEN __d%d END AS __d%d", integerPredicate("__bundle_group", groups), dimensionIndex, dimensionIndex))
	}
	for measureIndex, name := range measureNames {
		measure := measures[name]
		if measure.Fact != fact {
			continue
		}
		input := fmt.Sprintf("__v%d", measureIndex)
		expr := ""
		switch measure.Aggregation {
		case "count":
			expr = "COUNT(*)"
		case "count_distinct":
			expr = "COUNT(DISTINCT " + input + ")"
		case "sum", "avg", "min", "max":
			expr = strings.ToUpper(measure.Aggregation) + "(" + input + ")"
		default:
			return nil, nil, nil, fmt.Errorf("measure %q has unsupported aggregation %q", name, measure.Aggregation)
		}
		groups := []int{}
		seen := map[int]bool{}
		for branchIndex, resolved := range resolutions {
			if _, selected := resolved.Measures[name]; selected && !seen[branchGroups[branchIndex]] {
				seen[branchGroups[branchIndex]] = true
				groups = append(groups, branchGroups[branchIndex])
			}
		}
		filterParts := []string{integerPredicate("__bundle_group", groups)}
		if len(measure.Filters) > 0 {
			filterParts = append(filterParts, fmt.Sprintf("__f%d", measureIndex))
		}
		expr += " FILTER (WHERE " + strings.Join(filterParts, " AND ") + ")"
		if measure.Empty == "zero" && measure.Aggregation != "count" && measure.Aggregation != "count_distinct" {
			expr = "COALESCE(" + expr + ", 0)"
		}
		selects = append(selects, expr+" AS "+measureColumns[name])
	}
	positions := make([]string, len(dimensions)+1)
	for i := range positions {
		positions[i] = fmt.Sprint(i + 1)
	}
	aggregate := aggregateName + " AS (\n  SELECT " + strings.Join(selects, ", ") + "\n  FROM " + expandedName + "\n  GROUP BY " + strings.Join(positions, ", ") + "\n)"
	deps := make([]string, 0, len(dependencies))
	for dependency := range dependencies {
		deps = append(deps, dependency)
	}
	sort.Strings(deps)
	return []string{base, expanded, aggregate}, baseArgs, deps, nil
}

func stitchBundleFacts(facts []string, dimensions []bundleDimension, measures map[string]ResolvedMeasure, measureNames []string, measureColumns map[string]string) (string, []string) {
	left := "bundle_fact_0"
	available := map[string]bool{}
	for _, name := range measureNames {
		if measures[name].Fact == facts[0] {
			available[name] = true
		}
	}
	ctes := []string{}
	for factIndex := 1; factIndex < len(facts); factIndex++ {
		right := fmt.Sprintf("bundle_fact_%d", factIndex)
		selects := []string{"COALESCE(l.__bundle_group, r.__bundle_group) AS __bundle_group"}
		joins := []string{"l.__bundle_group = r.__bundle_group"}
		for dimensionIndex := range dimensions {
			column := fmt.Sprintf("__d%d", dimensionIndex)
			selects = append(selects, "COALESCE(l."+column+", r."+column+") AS "+column)
			joins = append(joins, "l."+column+" IS NOT DISTINCT FROM r."+column)
		}
		for _, name := range measureNames {
			column := measureColumns[name]
			if available[name] {
				selects = append(selects, "l."+column+" AS "+column)
			} else if measures[name].Fact == facts[factIndex] {
				selects = append(selects, "r."+column+" AS "+column)
			}
		}
		for _, name := range measureNames {
			if measures[name].Fact == facts[factIndex] {
				available[name] = true
			}
		}
		cteName := fmt.Sprintf("bundle_stitch_%d", factIndex)
		ctes = append(ctes, cteName+" AS (\n  SELECT "+strings.Join(selects, ", ")+"\n  FROM "+left+" l\n  FULL OUTER JOIN "+right+" r ON "+strings.Join(joins, " AND ")+"\n)")
		left = cteName
	}
	return left, ctes
}

func renderMultiFactBundleBranches(requests []BundleRequest, resolutions []aggregateResolution, branchDimensions [][]int, branchGroups []int, dimensions []bundleDimension, memberNames, memberColumns, memberExpr, physicalColumns []string, source string) ([]BundleBranch, []string, error) {
	branches := make([]BundleBranch, 0, len(requests))
	sqlBranches := make([]string, 0, len(requests))
	for branchIndex, item := range requests {
		mapping := map[string]string{}
		columns := []BundleColumn{}
		outputs := map[string]bool{}
		for local, dimension := range resolutions[branchIndex].Dimensions {
			if err := addOutputColumn(outputs, dimension.Alias); err != nil {
				return nil, nil, err
			}
			physical := fmt.Sprintf("__d%d", branchDimensions[branchIndex][local])
			mapping[dimension.Alias] = physical
			columns = append(columns, BundleColumn{Output: dimension.Alias, Physical: physical})
		}
		for _, member := range resolutions[branchIndex].Members {
			if err := addOutputColumn(outputs, member.Alias); err != nil {
				return nil, nil, err
			}
			physical := memberColumns[indexString(memberNames, member.Name)]
			mapping[member.Alias] = physical
			columns = append(columns, BundleColumn{Output: member.Alias, Physical: physical})
		}
		branches = append(branches, BundleBranch{ID: item.ID, Ordinal: branchIndex, Columns: columns})
		selects := []string{fmt.Sprintf("%d AS %s", branchIndex, BundleBranchColumn)}
		for dimensionIndex := range dimensions {
			selects = append(selects, fmt.Sprintf("__d%d", dimensionIndex))
		}
		for memberIndex, expression := range memberExpr {
			selects = append(selects, expression+" AS "+memberColumns[memberIndex])
		}
		core := "SELECT " + strings.Join(selects, ", ") + "\nFROM " + source + "\nWHERE __bundle_group = " + fmt.Sprint(branchGroups[branchIndex])
		sorts := make([]Sort, len(item.Request.Sort))
		for i, sortSpec := range item.Request.Sort {
			physical, ok := mapping[sortSpec.Field]
			if !ok {
				return nil, nil, fmt.Errorf("bundle branch %q sort references unknown output %q", item.ID, sortSpec.Field)
			}
			sorts[i] = Sort{Field: physical, Direction: sortSpec.Direction}
		}
		set := map[string]bool{}
		for _, column := range physicalColumns {
			set[column] = true
		}
		orderSorts := append([]Sort{}, sorts...)
		ordered := map[string]bool{}
		for _, sortSpec := range orderSorts {
			ordered[sortSpec.Field] = true
		}
		for _, column := range columns {
			if !ordered[column.Physical] {
				orderSorts = append(orderSorts, Sort{Field: column.Physical, Direction: "asc"})
				ordered[column.Physical] = true
			}
		}
		orderParts, err := sortSQL(orderSorts, set)
		if err != nil {
			return nil, nil, err
		}
		var sql strings.Builder
		sql.WriteString("SELECT branch_data.*, ROW_NUMBER() OVER (ORDER BY " + strings.Join(orderParts, ", ") + ") AS " + BundleRowColumn)
		sql.WriteString("\nFROM (\n" + indentSQL(core, "  ") + "\n) branch_data")
		if err := writeOrderLimitOffset(&sql, orderSorts, set, item.Request.Limit, item.Request.Offset); err != nil {
			return nil, nil, err
		}
		sqlBranches = append(sqlBranches, "("+sql.String()+")")
	}
	return branches, sqlBranches, nil
}
