package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

// newSpecCommand groups OpenAPI-document-producing subcommands
// (plan.md v7: "apilens spec export writes an OpenAPI file the team can
// review").
func newSpecCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Work with OpenAPI documents generated from the registry",
	}
	cmd.AddCommand(newSpecExportCommand(flags), newSpecGenerateCommand(flags))
	return cmd
}

// newSpecGenerateCommand implements plan.md v9's "Automatic test
// generation from OpenAPI examples (now that v7 contracts exist)":
// write one YAML v1 test per operation in a stored spec that declares a
// usable example.
func newSpecGenerateCommand(flags *globalFlags) *cobra.Command {
	var (
		specPath string
		out      string
		force    bool
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate YAML tests from a stored OpenAPI spec's examples",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			res, err := eng.GenerateFromSpec(apilens.GenerateFromSpecOptions{
				SpecPath: specPath, Out: out, Force: force,
			})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Generated %d of %d operation(s) (%d skipped — no usable example):\n",
				len(res.Files), res.Compiled, res.Skipped)
			for _, f := range res.Files {
				fmt.Fprintln(w, "  "+f)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&specPath, "from", "", "Path to the OpenAPI document to generate from (required)")
	cmd.Flags().StringVar(&out, "out", "", "Output directory (default .apilens/tests/generated)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing generated test files")
	return cmd
}

func newSpecExportCommand(flags *globalFlags) *cobra.Command {
	var (
		out     string
		force   bool
		title   string
		version string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write an OpenAPI 3 document built from the registry and captured exchanges",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			path, err := eng.SpecExport(apilens.SpecExportOptions{
				Out: out, Force: force, Title: title, Version: version,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Wrote", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default .apilens/api/export.openapi.yaml)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing file")
	cmd.Flags().StringVar(&title, "title", "", "OpenAPI info.title (default \"ApiLens Export\")")
	cmd.Flags().StringVar(&version, "spec-version", "", "OpenAPI info.version (default \"1.0.0\")")
	return cmd
}
