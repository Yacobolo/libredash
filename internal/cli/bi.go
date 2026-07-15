package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

func workspacesCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "workspaces", Short: "Inspect workspaces"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listWorkspaces", nil, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(list, opts)
	addPaginationFlags(list, opts)
	parent.AddCommand(list)
	return parent
}

func dashboardsCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "dashboards", Short: "Inspect dashboards"}
	parent.PersistentFlags().StringVar(&opts.workspaceID, "workspace", opts.workspaceID, "workspace id")
	list := &cobra.Command{
		Use:   "list",
		Short: "List dashboards",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listDashboards", map[string]string{"workspace": opts.workspaceID}, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(list, opts)
	addPaginationFlags(list, opts)

	describe := &cobra.Command{
		Use:   "describe <dashboard>",
		Short: "Describe a dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "getDashboard", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0]}, nil, nil)
		},
	}
	addTargetTokenFlags(describe, opts)

	components := &cobra.Command{
		Use:   "components <dashboard> <page>",
		Short: "List dashboard page components",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listDashboardComponents", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1]}, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(components, opts)
	addPaginationFlags(components, opts)

	visual := &cobra.Command{
		Use:   "visual <dashboard> <page> <visual>",
		Short: "Describe a dashboard visual",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "getDashboardVisual", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1], "visual": args[2]}, nil, nil)
		},
	}
	addTargetTokenFlags(visual, opts)

	visualData := &cobra.Command{
		Use:   "visual-data <dashboard> <page> <visual>",
		Short: "Query dashboard visual data",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := filtersBody(opts.filtersJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, "queryDashboardVisualData", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1], "visual": args[2]}, nil, postBody(body))
		},
	}
	addTargetTokenFlags(visualData, opts)
	visualData.Flags().StringVar(&opts.filtersJSON, "filters-json", "", "dashboard filters JSON")

	queryPage := &cobra.Command{
		Use:   "query-page <dashboard> <page>",
		Short: "Query a dashboard page",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := filtersBody(opts.filtersJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, "queryDashboardPage", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1]}, nil, postBody(body))
		},
	}
	addTargetTokenFlags(queryPage, opts)
	queryPage.Flags().StringVar(&opts.filtersJSON, "filters-json", "", "dashboard filters JSON")

	queryTable := &cobra.Command{
		Use:   "query-table <dashboard> <table>",
		Short: "Query a dashboard table",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := tableQueryBody(opts.pageID, opts.count, opts.filtersJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, "queryDashboardTable", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "table": args[1]}, nil, postBody(body))
		},
	}
	addTargetTokenFlags(queryTable, opts)
	queryTable.Flags().StringVar(&opts.pageID, "page", "", "dashboard page id")
	queryTable.Flags().IntVar(&opts.count, "count", 0, "row count")
	queryTable.Flags().StringVar(&opts.filtersJSON, "filters-json", "", "dashboard filters JSON")

	tableData := &cobra.Command{
		Use:   "table-data <dashboard> <page> <table>",
		Short: "Query dashboard table data",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := tableQueryBody("", opts.count, opts.filtersJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, "queryDashboardTableData", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1], "table": args[2]}, nil, postBody(body))
		},
	}
	addTargetTokenFlags(tableData, opts)
	tableData.Flags().IntVar(&opts.count, "count", 0, "row count")
	tableData.Flags().StringVar(&opts.filtersJSON, "filters-json", "", "dashboard filters JSON")

	filterOptions := &cobra.Command{
		Use:   "filter-options <dashboard> <page> <filter>",
		Short: "List dashboard filter options",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := filtersBody(opts.filtersJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, "listDashboardFilterOptions", map[string]string{"workspace": opts.workspaceID, "dashboard": args[0], "page": args[1], "filter": args[2]}, paginationQuery(opts), postBody(body))
		},
	}
	addTargetTokenFlags(filterOptions, opts)
	addPaginationFlags(filterOptions, opts)
	filterOptions.Flags().StringVar(&opts.filtersJSON, "filters-json", "", "dashboard filters JSON")

	parent.AddCommand(list, describe, components, visual, visualData, queryPage, queryTable, tableData, filterOptions)
	return parent
}

func semanticModelsCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "semantic-models", Short: "Inspect semantic models"}
	parent.PersistentFlags().StringVar(&opts.workspaceID, "workspace", opts.workspaceID, "workspace id")
	list := &cobra.Command{
		Use:   "list",
		Short: "List semantic models",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listSemanticModels", map[string]string{"workspace": opts.workspaceID}, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(list, opts)
	addPaginationFlags(list, opts)

	describe := &cobra.Command{
		Use:   "describe <model>",
		Short: "Describe a semantic model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "getSemanticModel", map[string]string{"workspace": opts.workspaceID, "model": args[0]}, nil, nil)
		},
	}
	addTargetTokenFlags(describe, opts)

	datasets := &cobra.Command{
		Use:   "datasets <model>",
		Short: "List semantic model datasets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listSemanticDatasets", map[string]string{"workspace": opts.workspaceID, "model": args[0]}, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(datasets, opts)
	addPaginationFlags(datasets, opts)

	dataset := &cobra.Command{
		Use:   "dataset <model> <dataset>",
		Short: "Describe a semantic model dataset",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "getSemanticDataset", map[string]string{"workspace": opts.workspaceID, "model": args[0], "dataset": args[1]}, nil, nil)
		},
	}
	addTargetTokenFlags(dataset, opts)

	fields := &cobra.Command{
		Use:   "fields <model> <dataset>",
		Short: "List semantic model dataset fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRawAPI(ctx, opts, "listSemanticFields", map[string]string{"workspace": opts.workspaceID, "model": args[0], "dataset": args[1]}, paginationQuery(opts), nil)
		},
	}
	addTargetTokenFlags(fields, opts)
	addPaginationFlags(fields, opts)

	query := semanticBodyCommand(ctx, opts, "query <model> <dataset>", "Query a semantic model dataset", "querySemanticDataset")
	preview := semanticBodyCommand(ctx, opts, "preview <model> <dataset>", "Preview semantic model dataset rows", "previewSemanticDataset")
	explainQuery := semanticBodyCommand(ctx, opts, "explain-query <model> <dataset>", "Explain a semantic model dataset query", "explainSemanticQuery")
	explainPreview := semanticBodyCommand(ctx, opts, "explain-preview <model> <dataset>", "Explain a semantic model dataset row preview", "explainSemanticPreview")

	parent.AddCommand(list, describe, datasets, dataset, fields, query, preview, explainQuery, explainPreview)
	return parent
}

func semanticBodyCommand(ctx context.Context, opts *rootOptions, use, short, operationID string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := bodyJSONMap(opts.bodyJSON)
			if err != nil {
				return err
			}
			return runRawAPI(ctx, opts, operationID, map[string]string{"workspace": opts.workspaceID, "model": args[0], "dataset": args[1]}, nil, postBody(body))
		},
	}
	addTargetTokenFlags(cmd, opts)
	cmd.Flags().StringVar(&opts.bodyJSON, "body-json", "", "request JSON body")
	return cmd
}

func addTargetTokenFlags(cmd *cobra.Command, opts *rootOptions) {
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
}

func addPaginationFlags(cmd *cobra.Command, opts *rootOptions) {
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum items to return")
	cmd.Flags().StringVar(&opts.pageToken, "page-token", "", "opaque page token")
}

func paginationQuery(opts *rootOptions) url.Values {
	query := url.Values{}
	if opts.limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.limit))
	}
	if opts.pageToken != "" {
		query.Set("pageToken", opts.pageToken)
	}
	return query
}

func postBody(body map[string]any) map[string]any {
	if body == nil {
		return map[string]any{}
	}
	return body
}

func runRawAPI(ctx context.Context, opts *rootOptions, operationID string, pathParams map[string]string, query url.Values, body map[string]any) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	endpoint, err := apiOperationURL(target, operationID, pathParams, query)
	if err != nil {
		return err
	}
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	var out any
	var requestBody io.Reader
	if body != nil {
		requestBody = reader
	}
	if err := doJSON(ctx, method, endpoint, token, requestBody, &out); err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	return encoder.Encode(out)
}

func filtersBody(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	filters, err := decodeObjectJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("filters-json: %w", err)
	}
	return map[string]any{"filters": filters}, nil
}

func tableQueryBody(pageID string, count int, rawFilters string) (map[string]any, error) {
	body := map[string]any{}
	if pageID != "" {
		body["pageId"] = pageID
	}
	if count > 0 {
		body["count"] = count
	}
	if rawFilters != "" {
		filters, err := decodeObjectJSON(rawFilters)
		if err != nil {
			return nil, fmt.Errorf("filters-json: %w", err)
		}
		body["filters"] = filters
	}
	if len(body) == 0 {
		return nil, nil
	}
	return body, nil
}

func decodeObjectJSON(raw string) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	return out, nil
}

func bodyJSONMap(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	body, err := decodeObjectJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("body-json: %w", err)
	}
	return body, nil
}
