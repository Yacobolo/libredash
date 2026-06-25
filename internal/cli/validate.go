package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"github.com/spf13/cobra"
)

func validateCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [catalog]",
		Short: "Validate a dashboard-as-code catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("validate accepts at most one positional catalog")
			}
			if len(args) == 1 {
				if cmd.Flags().Changed("catalog") {
					return fmt.Errorf("choose either --catalog or positional catalog, not both")
				}
				opts.catalog = args[0]
			}
			return runValidate(ctx, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.catalog, "catalog", filepath.Join("dashboards", "catalog.yaml"), "catalog path")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit JSON diagnostics")
	return cmd
}

func schemaCommand(opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "schema",
		Short: "Inspect LibreDash YAML schemas",
	}
	export := &cobra.Command{
		Use:   "export",
		Short: "Export generated schema artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaExport(opts)
		},
	}
	export.Flags().StringVar(&opts.schemaFormat, "format", "json-schema", "schema output format")
	export.Flags().StringVar(&opts.schemaOut, "out", filepath.Join("schemas", "json"), "output directory")
	parent.AddCommand(export)
	return parent
}

type validateResponse struct {
	OK          bool                      `json:"ok"`
	Diagnostics []configschema.Diagnostic `json:"diagnostics"`
}

func runValidate(ctx context.Context, opts *rootOptions, out io.Writer) error {
	diagnostics := validateCatalog(ctx, opts.catalog)
	response := validateResponse{OK: len(diagnostics) == 0, Diagnostics: diagnostics}
	if opts.jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			return err
		}
		if response.OK {
			return nil
		}
		return fmt.Errorf("validation failed")
	}
	if response.OK {
		fmt.Fprintf(out, "ok %s\n", opts.catalog)
		return nil
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(out, diagnostic.String())
	}
	return fmt.Errorf("validation failed")
}

func validateCatalog(ctx context.Context, catalogPath string) []configschema.Diagnostic {
	if err := configschema.ValidateFile(configschema.KindCatalog, catalogPath); err != nil {
		return configschema.Diagnostics(err)
	}
	catalog, baseDir, err := workspace.LoadCatalog(catalogPath)
	if err != nil {
		return configschema.Diagnostics(err)
	}
	var diagnostics []configschema.Diagnostic
	for _, entry := range catalog.SemanticModels {
		path := filepath.Join(baseDir, entry.Path)
		if err := configschema.ValidateFile(configschema.KindSemanticModel, path); err != nil {
			diagnostics = append(diagnostics, configschema.Diagnostics(err)...)
		}
	}
	for _, entry := range catalog.Dashboards {
		path := filepath.Join(baseDir, entry.Path)
		if err := configschema.ValidateFile(configschema.KindDashboard, path); err != nil {
			diagnostics = append(diagnostics, configschema.Diagnostics(err)...)
		}
	}
	if len(diagnostics) > 0 {
		return diagnostics
	}
	if _, err := workspacecompiler.CompileDefinition(catalogPath); err != nil {
		return configschema.Diagnostics(err)
	}
	if err := ctx.Err(); err != nil {
		return configschema.Diagnostics(err)
	}
	return nil
}

func runSchemaExport(opts *rootOptions) error {
	if opts.schemaFormat != "json-schema" {
		return fmt.Errorf("unsupported schema format %q", opts.schemaFormat)
	}
	files, err := configschema.JSONSchemaFiles()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.schemaOut, 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(opts.schemaOut, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
