package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newCoverageCommand(flags *globalFlags) *cobra.Command {
	var missingOnly bool

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Show which discovered endpoints have at least one test",
		Long: `Show which discovered endpoints have at least one test.

Cross-references the registry (from the last "apilens discover") against
every compiled test under .apilens/tests. Does not run any test or make
any network call — this only reads what's already on disk, so it's safe
to run at any time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			report, err := eng.Coverage()
			if err != nil {
				return err
			}
			return printCoverageReport(cmd, flags, report, missingOnly)
		},
	}
	cmd.Flags().BoolVar(&missingOnly, "missing-only", false, "Only list uncovered endpoints (terminal format only)")
	return cmd
}

func printCoverageReport(cmd *cobra.Command, flags *globalFlags, report *apilens.CoverageReport, missingOnly bool) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		type jsonEndpoint struct {
			Method string   `json:"method"`
			Path   string   `json:"path"`
			Tests  []string `json:"tests,omitempty"`
		}
		payload := struct {
			Total   int            `json:"total"`
			Covered []jsonEndpoint `json:"covered"`
			Missing []jsonEndpoint `json:"missing"`
			Percent float64        `json:"percent"`
		}{Total: report.Total, Percent: report.Percent()}
		for _, ep := range report.Covered {
			payload.Covered = append(payload.Covered, jsonEndpoint{Method: ep.Method, Path: ep.Path, Tests: ep.Tests})
		}
		for _, ep := range report.Missing {
			payload.Missing = append(payload.Missing, jsonEndpoint{Method: ep.Method, Path: ep.Path})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if report.Total == 0 {
		fmt.Fprintln(out, "No endpoints in the registry. Run 'apilens discover' first.")
		return nil
	}

	fmt.Fprintln(out, "API COVERAGE")
	fmt.Fprintln(out)
	if !missingOnly {
		for _, ep := range report.Covered {
			fmt.Fprintf(out, "✓ %-12s%-40s %s\n", ep.Method, ep.Path, coverageTestList(ep.Tests))
		}
	}
	for _, ep := range report.Missing {
		fmt.Fprintf(out, "✗ %-12s%s\n", ep.Method, ep.Path)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Covered: %d/%d (%.0f%%)\n", len(report.Covered), report.Total, report.Percent())
	if len(report.Missing) > 0 && !missingOnly {
		fmt.Fprintf(out, "Uncovered: %d — see the ✗ lines above, or run with --missing-only\n", len(report.Missing))
	}
	return nil
}

func coverageTestList(tests []string) string {
	if len(tests) == 0 {
		return ""
	}
	out := "(" + tests[0]
	for _, t := range tests[1:] {
		out += ", " + t
	}
	return out + ")"
}
