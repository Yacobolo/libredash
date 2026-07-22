package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type searchResponse struct {
	Items []searchResult `json:"items"`
	Page  struct {
		NextCursor string `json:"nextCursor"`
	} `json:"page"`
}

type searchResult struct {
	Reference struct {
		WorkspaceID string `json:"workspaceId"`
		Type        string `json:"type"`
		ID          string `json:"id"`
	} `json:"reference"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Href        string `json:"href"`
}

func searchCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search accessible product objects",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(ctx, opts, args[0])
		},
	}
	addTargetTokenFlags(cmd, opts)
	cmd.Flags().StringArrayVar(&opts.searchWorkspaces, "workspace", nil, "workspace filter; repeatable")
	addPaginationFlags(cmd, opts)
	cmd.Flags().StringArrayVar(&opts.searchTypes, "type", nil, "result type filter; repeatable or comma-separated")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print JSON response")
	return cmd
}

func runSearch(ctx context.Context, opts *rootOptions, queryText string) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	query := paginationQuery(opts)
	query.Set("q", queryText)
	for _, workspaceID := range searchValues(opts.searchWorkspaces) {
		query.Add("workspace", workspaceID)
	}
	for _, typ := range searchValues(opts.searchTypes) {
		query.Add("type", typ)
	}
	endpoint, err := apiOperationURL(target, "search", nil, query)
	if err != nil {
		return err
	}
	var response searchResponse
	if err := doJSON(ctx, http.MethodGet, endpoint, token, nil, &response); err != nil {
		return err
	}
	if opts.jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		return encoder.Encode(response)
	}
	return renderSearchResults(response)
}

func searchValues(values []string) []string {
	types := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				types = append(types, part)
			}
		}
	}
	return types
}

func renderSearchResults(response searchResponse) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKSPACE\tTYPE\tNAME\tDESCRIPTION\tID")
	for _, item := range response.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Reference.WorkspaceID, item.Reference.Type, item.Name, item.Description, item.Reference.ID)
	}
	if response.Page.NextCursor != "" {
		fmt.Fprintf(tw, "PAGE\tNEXT\t%s\t\t\n", response.Page.NextCursor)
	}
	return tw.Flush()
}
