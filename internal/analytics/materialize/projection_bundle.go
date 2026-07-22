package materialize

import (
	"context"
	"fmt"
	"math"

	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
	"github.com/Yacobolo/leapview/internal/analytics/resultcache"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

// executeProjectionBundle replaces scalar branches with exact reductions of
// complete grouped branches. Its inputs have already been governed branch by
// branch; exact governed filters and masks are rechecked by the semantic
// projection proof before execution. Multiple grouped branches execute through
// the ordinary bundle path so compatible single- and multi-fact shapes still
// share one physical statement.
func (r *Runtime) executeProjectionBundle(ctx context.Context, branches []dataquery.BundleRequest, _ map[string]dataquery.ResultTransformer) (dataquery.BundleResult, bool, error) {
	if len(branches) < 2 {
		return dataquery.BundleResult{}, false, nil
	}
	groupedIndexes := make([]int, 0, len(branches))
	scalarIndexes := make([]int, 0, len(branches))
	for index, branch := range branches {
		request := branch.Query
		if len(request.Fields) > 0 || request.Time.Field != "" {
			groupedIndexes = append(groupedIndexes, index)
			continue
		}
		if len(request.Measures) != 1 {
			return dataquery.BundleResult{}, false, nil
		}
		scalarIndexes = append(scalarIndexes, index)
	}
	// A grouped-only bundle belongs to the normal bundle planner. Requiring at
	// least one branch of each kind also prevents recursion when that planner is
	// invoked below for multiple grouped sources.
	if len(groupedIndexes) == 0 || len(scalarIndexes) == 0 {
		return dataquery.BundleResult{}, false, nil
	}

	compatibleSources := make(map[int][]int, len(scalarIndexes))
	for _, scalarIndex := range scalarIndexes {
		for _, groupedIndex := range groupedIndexes {
			grouped := branches[groupedIndex]
			probe := semanticquery.Row{}
			for _, measure := range grouped.Query.Measures {
				if measure.Alias == "" {
					return dataquery.BundleResult{}, false, nil
				}
				probe[measure.Alias] = int64(0)
			}
			_, compatible, err := semanticquery.ProjectScalarFromGrouped(
				r.model,
				semanticProjectionRequest(grouped.Query),
				semanticProjectionRequest(branches[scalarIndex].Query),
				semanticquery.Rows{probe},
				true,
			)
			if err == nil && compatible {
				compatibleSources[scalarIndex] = append(compatibleSources[scalarIndex], groupedIndex)
			}
		}
		if len(compatibleSources[scalarIndex]) == 0 {
			return dataquery.BundleResult{}, false, nil
		}
	}

	physicalGrouped := make([]dataquery.BundleRequest, len(groupedIndexes))
	for index, groupedIndex := range groupedIndexes {
		physicalGrouped[index] = branches[groupedIndex]
		if limit := physicalGrouped[index].Query.Limit; limit > 0 && limit < math.MaxInt {
			physicalGrouped[index].Query.Limit++
		}
	}

	var groupedBundle dataquery.BundleResult
	if len(physicalGrouped) == 1 {
		grouped := physicalGrouped[0]
		groupedResult, err := r.ExecuteDataQuery(dataquery.WithGovernanceApplied(ctx), grouped.Query)
		if err != nil {
			return dataquery.BundleResult{}, true, &dataquery.BundleBranchError{ID: grouped.ID, Err: err}
		}
		groupedBundle = dataquery.BundleResult{
			Results: map[string]dataquery.Result{grouped.ID: groupedResult},
			SQL:     groupedResult.SQL,
		}
	} else {
		var err error
		groupedBundle, err = r.ExecuteDataQueryBundle(dataquery.WithGovernanceApplied(ctx), physicalGrouped)
		if err != nil {
			return dataquery.BundleResult{}, true, err
		}
	}
	if err := ctx.Err(); err != nil {
		return dataquery.BundleResult{}, true, err
	}

	completeSources := make(map[int]bool, len(groupedIndexes))
	result := dataquery.BundleResult{Results: make(map[string]dataquery.Result, len(branches)), SQL: groupedBundle.SQL}
	for _, groupedIndex := range groupedIndexes {
		grouped := branches[groupedIndex]
		groupedResult, ok := groupedBundle.Results[grouped.ID]
		if !ok {
			return dataquery.BundleResult{}, true, &dataquery.BundleBranchError{ID: grouped.ID, Err: fmt.Errorf("grouped bundle result is missing")}
		}
		// MaxInt cannot be safely overfetched, so a limited source with that
		// sentinel is never considered proof of completeness.
		complete := grouped.Query.Limit == 0 || (grouped.Query.Limit < math.MaxInt && len(groupedResult.Rows) <= grouped.Query.Limit)
		completeSources[groupedIndex] = complete
		if grouped.Query.Limit > 0 && len(groupedResult.Rows) > grouped.Query.Limit {
			groupedResult.Rows = groupedResult.Rows[:grouped.Query.Limit]
		}
		groupedResult.RowsReturned = len(groupedResult.Rows)
		groupedResult.BytesEstimate = resultcache.EstimateResultBytes(groupedResult)
		result.Results[grouped.ID] = groupedResult
	}

	for _, scalarIndex := range scalarIndexes {
		branch := branches[scalarIndex]
		groupedIndex := -1
		for _, candidate := range compatibleSources[scalarIndex] {
			if completeSources[candidate] {
				groupedIndex = candidate
				break
			}
		}
		if groupedIndex < 0 {
			return dataquery.BundleResult{}, true, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("scalar branch %q has no complete compatible grouped projection source", branch.ID)}
		}
		grouped := branches[groupedIndex]
		groupedResult := groupedBundle.Results[grouped.ID]
		rows, projected, projectionErr := semanticquery.ProjectScalarFromGrouped(
			r.model,
			semanticProjectionRequest(grouped.Query),
			semanticProjectionRequest(branch.Query),
			semanticProjectionRows(groupedResult.Rows),
			true,
		)
		if projectionErr != nil {
			return dataquery.BundleResult{}, true, &dataquery.BundleBranchError{ID: branch.ID, Err: projectionErr}
		}
		if !projected {
			return dataquery.BundleResult{}, true, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("scalar branch %q is not safely projectable", branch.ID)}
		}
		projectedRows := dataQueryRows(rows)
		if budget, ok := dataquery.ResultBudgetFromContext(ctx); ok {
			if err := budget.ConsumeRows(projectedRows); err != nil {
				for _, grouped := range physicalGrouped {
					r.queryCache.remove(grouped.Query)
				}
				return dataquery.BundleResult{}, true, err
			}
		}
		outputAlias := branch.Query.Measures[0].Alias
		if outputAlias == "" {
			outputAlias = branch.Query.Measures[0].Field
		}
		branchResult := dataquery.Result{
			Columns:        dataquery.ColumnsFromNames([]string{outputAlias}),
			Rows:           projectedRows,
			SQL:            groupedResult.SQL,
			ExecutionState: dataquery.ExecutionSucceeded,
			Status:         dataquery.StatusSuccess,
			RowsReturned:   len(projectedRows),
			CacheOutcome:   groupedResult.CacheOutcome,
		}
		branchResult.BytesEstimate = resultcache.EstimateResultBytes(branchResult)
		result.Results[branch.ID] = branchResult
	}
	for _, branch := range branches {
		branchResult := result.Results[branch.ID]
		if !dashboardQueryResultCacheable(branch.Query) {
			continue
		}
		_, key, generation, hit, lookupErr := r.queryCache.lookup(branch.Query)
		if lookupErr != nil {
			return dataquery.BundleResult{}, true, &dataquery.BundleBranchError{ID: branch.ID, Err: lookupErr}
		}
		if !hit {
			if err := ctx.Err(); err != nil {
				return dataquery.BundleResult{}, true, err
			}
			r.queryCache.store(key, generation, branchResult)
			dataquery.ObserveCacheOutcome(ctx, dataquery.CacheMiss)
		}
	}
	return result, true, nil
}

func semanticProjectionRequest(request dataquery.Query) semanticquery.Request {
	return semanticquery.Request{
		Table:       request.Target,
		Dimensions:  dataQueryFields(request.Fields),
		Measures:    dataQueryFields(request.Measures),
		Time:        semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:     dataQueryFilters(request.Filters),
		Sort:        dataQuerySorts(request.Sort),
		ColumnMasks: dataQueryColumnMasks(request.ColumnMasks),
		Limit:       request.Limit,
		Offset:      request.Offset,
	}
}

func semanticProjectionRows(rows []dataquery.Row) semanticquery.Rows {
	result := make(semanticquery.Rows, len(rows))
	for index, row := range rows {
		result[index] = semanticquery.Row(row)
	}
	return result
}
