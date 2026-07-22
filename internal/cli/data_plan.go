package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/manageddata/localplan"
	"github.com/spf13/cobra"
)

type dataPlanner interface {
	Plan(context.Context, localplan.Request) (localplan.Result, error)
}

func dataCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	return dataCommandWithOptions(ctx, localplan.NewService(), opts)
}

func dataCommandWithPlanner(ctx context.Context, planner dataPlanner) *cobra.Command {
	return dataCommandWithOptions(ctx, planner, &rootOptions{})
}

func dataCommandWithOptions(ctx context.Context, planner dataPlanner, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:          "data",
		Short:        "Manage project-global data revisions",
		SilenceUsage: true,
	}
	parent.AddCommand(dataPlanCommand(ctx, planner))
	parent.AddCommand(dataSyncCommand(ctx, planner, opts))
	parent.AddCommand(dataRevisionsCommand(ctx, opts))
	return parent
}

func dataPlanCommand(ctx context.Context, planner dataPlanner) *cobra.Command {
	var projectPath string
	var connection string
	var from string
	var previousManifestPath string
	command := &cobra.Command{
		Use:   "plan",
		Short: "Plan a local managed data revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(connection) == "" {
				return fmt.Errorf("connection is required")
			}
			if strings.TrimSpace(from) == "" {
				return fmt.Errorf("from is required")
			}
			var previous *manageddata.Manifest
			if previousManifestPath != "" {
				manifest, err := readManagedDataManifest(previousManifestPath)
				if err != nil {
					return fmt.Errorf("previous manifest: %w", err)
				}
				previous = &manifest
			}
			result, err := planner.Plan(ctx, localplan.Request{
				ProjectPath: projectPath,
				Connection:  connection,
				From:        from,
				Previous:    previous,
			})
			if err != nil {
				return err
			}
			return writeDataPlan(cmd.OutOrStdout(), result)
		},
	}
	command.Flags().StringVar(&projectPath, "project", filepath.Join("dashboards", "leapview.yaml"), "project path")
	command.Flags().StringVar(&connection, "connection", "", "project-global managed connection")
	command.Flags().StringVar(&from, "from", "", "local filesystem root to ingest")
	command.Flags().StringVar(&previousManifestPath, "previous-manifest", "", "prior managed data manifest path")
	return command
}

type dataPlanOutput struct {
	Connection string               `json:"connection"`
	Root       string               `json:"root"`
	Sources    []string             `json:"sources"`
	RevisionID string               `json:"revisionId"`
	Manifest   manageddata.Manifest `json:"manifest"`
	Diff       dataPlanDiff         `json:"diff"`
}

type dataPlanDiff struct {
	Added     []manageddata.File `json:"added"`
	Changed   []manageddata.File `json:"changed"`
	Removed   []manageddata.File `json:"removed"`
	Unchanged []manageddata.File `json:"unchanged"`
}

func writeDataPlan(out io.Writer, result localplan.Result) error {
	document := dataPlanOutput{
		Connection: result.Connection,
		Root:       result.Root,
		Sources:    append([]string{}, result.Sources...),
		RevisionID: result.Manifest.RevisionID(),
		Manifest:   result.Manifest,
		Diff: dataPlanDiff{
			Added:     append([]manageddata.File{}, result.Diff.Added...),
			Changed:   append([]manageddata.File{}, result.Diff.Changed...),
			Removed:   append([]manageddata.File{}, result.Diff.Removed...),
			Unchanged: append([]manageddata.File{}, result.Diff.Unchanged...),
		},
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func readManagedDataManifest(name string) (manageddata.Manifest, error) {
	file, err := os.Open(name)
	if err != nil {
		return manageddata.Manifest{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest manageddata.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return manageddata.Manifest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return manageddata.Manifest{}, fmt.Errorf("must contain exactly one JSON object")
		}
		return manageddata.Manifest{}, err
	}
	if err := manifest.Validate(manageddata.Limits{}); err != nil {
		return manageddata.Manifest{}, err
	}
	return manifest, nil
}
