package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

// newContractCommand groups contract-testing subcommands (plan.md v7:
// "Contract testing against a stored spec").
func newContractCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contract",
		Short: "Validate responses against a stored OpenAPI spec",
	}
	cmd.AddCommand(newContractTestCommand(flags))
	return cmd
}

func newContractTestCommand(flags *globalFlags) *cobra.Command {
	var (
		specPath string
		live     bool
	)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Check captured (or live) responses against a spec's response schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			report, err := eng.ContractTest(cmd.Context(), apilens.ContractTestOptions{
				SpecPath: specPath, Live: live,
			})
			if err != nil {
				return err
			}
			return printContractReport(cmd, flags, report)
		},
	}
	cmd.Flags().StringVar(&specPath, "spec", "", "Path to the OpenAPI document to validate against (required)")
	cmd.Flags().BoolVar(&live, "live", false, "Probe endpoints with no captured example through the current environment")
	return cmd
}

// contractJSONResult/contractJSONDocument mirror reporter's json.go shape
// closely enough that CI tooling parsing `apilens run --format json` can
// reuse the same field names for `apilens contract test --format json`.
type contractJSONResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	HTTPStatus int    `json:"http_status"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason,omitempty"`
	Error      string `json:"error,omitempty"`
}

type contractJSONDocument struct {
	Counts  interface{}          `json:"counts"`
	Results []contractJSONResult `json:"results"`
}

func printContractReport(cmd *cobra.Command, flags *globalFlags, report *apilens.Report) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		doc := contractJSONDocument{Counts: report.Counts}
		for _, r := range report.Results {
			reason := ""
			for _, a := range r.Assertions {
				if !a.Passed {
					reason = a.Reason
				}
			}
			doc.Results = append(doc.Results, contractJSONResult{
				Name: r.Name, Status: string(r.Status), Method: string(r.Method),
				URL: r.URL, HTTPStatus: r.HTTPStatus, DurationMS: r.DurationMS,
				Reason: reason, Error: r.Error,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return err
		}
		setExitCode(exitCodeForReport(*report))
		return nil
	}

	fmt.Fprintln(out, "CONTRACT TEST RESULTS")
	fmt.Fprintln(out)
	for _, r := range report.Results {
		mark := contractStatusMark(string(r.Status))
		fmt.Fprintf(out, "%s %-30s %d\n", mark, r.Name, r.HTTPStatus)
		for _, a := range r.Assertions {
			if !a.Passed {
				fmt.Fprintf(out, "    %s\n", a.Reason)
			}
		}
		if r.Error != "" {
			fmt.Fprintf(out, "    error: %s\n", r.Error)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Tests:    %d\n", report.Counts.Tests)
	fmt.Fprintf(out, "Passed:   %d\n", report.Counts.Passed)
	fmt.Fprintf(out, "Failed:   %d\n", report.Counts.Failed)
	if report.Counts.Errored > 0 {
		fmt.Fprintf(out, "Errored:  %d\n", report.Counts.Errored)
	}
	if report.Counts.Skipped > 0 {
		fmt.Fprintf(out, "Skipped:  %d\n", report.Counts.Skipped)
	}

	setExitCode(exitCodeForReport(*report))
	return nil
}

func contractStatusMark(s string) string {
	switch s {
	case "passed":
		return "\u2713"
	case "skipped":
		return "\u25cb"
	default:
		return "\u2717"
	}
}
