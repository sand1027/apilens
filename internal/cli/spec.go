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
	cmd.AddCommand(newSpecExportCommand(flags))
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
