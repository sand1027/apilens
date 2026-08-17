package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newTestCommand(flags *globalFlags) *cobra.Command {
	var (
		method          string
		out             string
		outFormat       string
		captureResponse bool
	)

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
				Ref:             args[0],
				Method:          method,
				ReporterFormat:  flags.format,
				Quiet:           flags.quiet,
				Out:             out,
				OutFormat:       outFormat,
				CaptureResponse: captureResponse,
			})
			if err == nil && out != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Wrote report to", out)
			}
			return handleRunResult(cmd, report, err)
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "Required when multiple methods share the path")
	cmd.Flags().StringVar(&out, "out", "", "Also write the report to this file (e.g. .apilens/reports/latest.json), independent of --format")
	cmd.Flags().StringVar(&outFormat, "out-format", "", "Format for --out: json or junit (default: inferred from --out's extension, .xml -> junit)")
	cmd.Flags().BoolVar(&captureResponse, "capture-response", false, "Include the actual (redacted) response body/headers in the report, like Postman's collection runner")
	return cmd
}
