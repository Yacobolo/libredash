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
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func searchCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search workspace assets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(ctx, opts, args[0])
		},
	}
	addTargetTokenFlags(cmd, opts)
	cmd.Flags().StringVar(&opts.workspaceID, "workspace", opts.workspaceID, "workspace id")
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
	if types := searchTypesValue(opts.searchTypes); types != "" {
		query.Set("types", types)
	}
	endpoint, err := apiOperationURL(target, "searchWorkspace", map[string]string{"workspace": opts.workspaceID}, query)
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

func searchTypesValue(values []string) string {
	types := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				types = append(types, part)
			}
		}
	}
	return strings.Join(types, ",")
}

func renderSearchResults(response searchResponse) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tNAME\tDESCRIPTION\tID")
	for _, item := range response.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.Type, item.Name, item.Description, item.ID)
	}
	if response.Page.NextCursor != "" {
		fmt.Fprintf(tw, "PAGE\tNEXT\t%s\t\n", response.Page.NextCursor)
	}
	return tw.Flush()
}
