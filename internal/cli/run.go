package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

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

		// --load and friends implement plan.md v9's "Load / soak mode
		// (apilens run --load) on the same runner" — a flag on `run`
		// rather than a separate command, per plan.md's exact wording
		// ("apilens run --load or apilens load"); a flag keeps the
		// filter flags (--filter/--method/--tag) shared between normal
		// and load runs instead of duplicating them on a new command.
		load           bool
		loadDuration   string
		loadIterations int
		loadWorkers    int
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full test suite under .apilens/tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}

			runFilter := apilens.RunFilter{
				Ref:            filter,
				Method:         method,
				Tag:            tag,
				Sequential:     sequential,
				Parallel:       parallel,
				FailFast:       failFast,
				ReporterFormat: flags.format,
				Quiet:          flags.quiet,
			}

			if load {
				var duration time.Duration
				if loadDuration != "" {
					duration, err = time.ParseDuration(loadDuration)
					if err != nil {
						return fmt.Errorf("invalid --duration %q: %w", loadDuration, err)
					}
				}
				report, err := eng.RunLoad(cmd.Context(), runFilter, apilens.LoadOptions{
					Duration:   duration,
					Iterations: loadIterations,
					Workers:    loadWorkers,
				})
				if err != nil {
					return err
				}
				return printLoadReport(cmd, flags, report)
			}

			report, err := eng.Run(cmd.Context(), runFilter)
			return handleRunResult(cmd, report, err)
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "Match test name, tag, or path substring")
	cmd.Flags().StringVar(&method, "method", "", "Restrict to tests with this HTTP method")
	cmd.Flags().StringVar(&tag, "tag", "", "Restrict to tests carrying this tag")
	cmd.Flags().BoolVar(&sequential, "sequential", false, "Force sequential execution")
	cmd.Flags().BoolVar(&parallel, "parallel", false, "Force parallel execution")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop after the first failure")
	cmd.Flags().BoolVar(&load, "load", false, "Load/soak mode: repeat the matched tests and report latency percentiles instead of pass/fail")
	cmd.Flags().StringVar(&loadDuration, "duration", "", "Load mode: wall-clock bound, e.g. \"30s\" (requires --load; --duration and/or --iterations)")
	cmd.Flags().IntVar(&loadIterations, "iterations", 0, "Load mode: total request bound across all workers (requires --load)")
	cmd.Flags().IntVar(&loadWorkers, "workers", 0, "Load mode: concurrent workers (default 10, requires --load)")
	return cmd
}

// printLoadReport formats a load-mode Report. Terminal output focuses on
// what a load/soak run is actually for — latency distribution and error
// rate — not a pass/fail list (docs/05-cli.md's reporter output
// conventions extended to a shape domain.Report was never meant to hold).
func printLoadReport(cmd *cobra.Command, flags *globalFlags, report *apilens.LoadReport) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
		setExitCode(exitCodeForLoadReport(*report))
		return nil
	}

	fmt.Fprintln(out, "LOAD TEST RESULTS")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Workers:        %d\n", report.Workers)
	fmt.Fprintf(out, "Duration:       %dms\n", report.DurationMS)
	fmt.Fprintf(out, "Total requests: %d\n", report.TotalRequests)
	fmt.Fprintf(out, "Errors:         %d (%.1f%%)\n", report.TotalErrors, report.ErrorRate()*100)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Overall latency:")
	printPercentiles(out, report.Overall)
	if len(report.PerTest) > 1 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Per test:")
		for _, ts := range report.PerTest {
			fmt.Fprintf(out, "  %s — %d requests, %d errors\n", ts.Name, ts.Requests, ts.Errors)
			printPercentiles(out, ts.Percentiles)
		}
	}

	setExitCode(exitCodeForLoadReport(*report))
	return nil
}

func printPercentiles(out io.Writer, p apilens.Percentiles) {
	fmt.Fprintf(out, "  min=%s  mean=%s  p50=%s  p90=%s  p95=%s  p99=%s  max=%s\n",
		p.Min, p.Mean, p.P50, p.P90, p.P95, p.P99, p.Max)
}

// exitCodeForLoadReport uses a simple threshold: any error at all is a
// failing exit code. Load mode has no built-in SLA/threshold flag yet
// (plan.md v9 doesn't specify one), so "zero errors" is the only
// unambiguous default; a future version could add --max-error-rate.
func exitCodeForLoadReport(report apilens.LoadReport) int {
	if report.TotalErrors > 0 {
		return 1
	}
	return 0
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
