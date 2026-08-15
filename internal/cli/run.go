package cli

import (
	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newRunCommand(flags *globalFlags) *cobra.Command {
	var (
		filter     string
		method     string
		tag        string
		sequential bool
		parallel   bool
		failFast   bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full test suite under .apilens/tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			report, err := eng.Run(cmd.Context(), apilens.RunFilter{
				Ref:            filter,
				Method:         method,
				Tag:            tag,
				Sequential:     sequential,
				Parallel:       parallel,
				FailFast:       failFast,
				ReporterFormat: flags.format,
				Quiet:          flags.quiet,
			})
			return handleRunResult(cmd, report, err)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "Match test name, tag, or path substring")
	cmd.Flags().StringVar(&method, "method", "", "Restrict to tests with this HTTP method")
	cmd.Flags().StringVar(&tag, "tag", "", "Restrict to tests carrying this tag")
	cmd.Flags().BoolVar(&sequential, "sequential", false, "Force sequential execution")
	cmd.Flags().BoolVar(&parallel, "parallel", false, "Force parallel execution")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop after the first failure")
	return cmd
}

// handleRunResult sets the process exit code from the Report/err and
// returns nil so cobra doesn't also print a duplicate "Error:" line for
// ordinary test failures (which aren't usage errors) — those are already
// visible in the reporter's own output.
func handleRunResult(cmd *cobra.Command, report *apilens.Report, err error) error {
	if err != nil {
		return err // usage/config error; caller (main) maps + prints it
	}
	setExitCode(exitCodeForReport(*report))
	return nil
}
