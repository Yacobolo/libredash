package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type dataRevisionOptions struct {
	project     string
	connection  string
	environment string
	limit       int
	pageToken   string
}

func dataRevisionsCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "revisions", Short: "Inspect managed data revisions"}
	listOptions := dataRevisionOptions{}
	list := &cobra.Command{
		Use:   "list",
		Short: "List staged managed data revisions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateDataRevisionOptions(listOptions, false); err != nil {
				return err
			}
			target, token, err := clientTargetAndToken(opts)
			if err != nil {
				return err
			}
			if _, err := targetEnvironment(ctx, nil, target, token, listOptions.environment); err != nil {
				return err
			}
			return runDataRevisionsList(ctx, opts, listOptions, newManagedDataCLIClient(nil, target, token), cmd.OutOrStdout())
		},
	}
	addDataRevisionFlags(list, opts, &listOptions, false)

	currentOptions := dataRevisionOptions{}
	current := &cobra.Command{
		Use:   "current",
		Short: "Print the active managed data revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateDataRevisionOptions(currentOptions, true); err != nil {
				return err
			}
			target, token, err := clientTargetAndToken(opts)
			if err != nil {
				return err
			}
			if _, err := targetEnvironment(ctx, nil, target, token, currentOptions.environment); err != nil {
				return err
			}
			return runDataRevisionCurrent(ctx, opts, currentOptions, newManagedDataCLIClient(nil, target, token), cmd.OutOrStdout())
		},
	}
	addDataRevisionFlags(current, opts, &currentOptions, true)
	parent.AddCommand(list, current)
	return parent
}

func addDataRevisionFlags(command *cobra.Command, opts *rootOptions, values *dataRevisionOptions, current bool) {
	command.Flags().StringVar(&values.project, "project", "", "server project id")
	command.Flags().StringVar(&values.connection, "connection", "", "project-global managed connection")
	command.Flags().StringVar(&values.environment, "environment", "", "assert the target instance environment")
	if !current {
		command.Flags().IntVar(&values.limit, "limit", 0, "maximum revisions to return")
		command.Flags().StringVar(&values.pageToken, "page-token", "", "opaque page token")
	}
	addTargetTokenFlags(command, opts)
}

func validateDataRevisionOptions(values dataRevisionOptions, current bool) error {
	if strings.TrimSpace(values.project) == "" {
		return fmt.Errorf("project is required")
	}
	if strings.TrimSpace(values.connection) == "" {
		return fmt.Errorf("connection is required")
	}
	if values.limit < 0 {
		return fmt.Errorf("limit must not be negative")
	}
	return nil
}

func runDataRevisionsList(ctx context.Context, _ *rootOptions, values dataRevisionOptions, client *managedDataCLIClient, out io.Writer) error {
	query := url.Values{}
	if values.limit > 0 {
		query.Set("limit", strconv.Itoa(values.limit))
	}
	if values.pageToken != "" {
		query.Set("pageToken", values.pageToken)
	}
	response, err := client.listRevisions(ctx, values.project, values.connection, query)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "REVISION\tSTATUS\tFILES\tBYTES\tCREATED"); err != nil {
		return err
	}
	for _, revision := range response.Items {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", revision.Id, strings.ToUpper(string(revision.Status)), revision.FileCount, revision.Size, revision.CreatedAt); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func runDataRevisionCurrent(ctx context.Context, _ *rootOptions, values dataRevisionOptions, client *managedDataCLIClient, out io.Writer) error {
	response, err := client.currentRevision(ctx, values.project, values.connection, "")
	if err != nil {
		return err
	}
	if response.Revision == nil {
		_, err = fmt.Fprintln(out, "none")
		return err
	}
	_, err = fmt.Fprintln(out, response.Revision.Id)
	return err
}
