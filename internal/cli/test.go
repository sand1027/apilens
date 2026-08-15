package cli

import (
	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newTestCommand(flags *globalFlags) *cobra.Command {
	var method string

	cmd := &cobra.Command{
		Use:   "test <ref>",
		Short: "Run tests matching a path, file, or test name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			report, err := eng.Run(cmd.Context(), apilens.RunFilter{
				Ref:            args[0],
				Method:         method,
				ReporterFormat: flags.format,
			})
			return handleRunResult(cmd, report, err)
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "Required when multiple methods share the path")
	return cmd
}
