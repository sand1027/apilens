package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newInspectCommand(flags *globalFlags) *cobra.Command {
	var (
		method string
		live   bool
	)

	cmd := &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Show a registry endpoint's spec, or probe it live with --live",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			insp, err := eng.Inspect(cmd.Context(), apilens.InspectRef{
				Ref: args[0], Method: method, Live: live,
			})
			if err != nil {
				return err
			}
			return printInspection(cmd, flags, insp)
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "Required when multiple methods share the path")
	cmd.Flags().BoolVar(&live, "live", false, "Execute a probe request through the runner")
	return cmd
}

func printInspection(cmd *cobra.Command, flags *globalFlags, insp *apilens.Inspection) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		type jsonLive struct {
			StatusCode int   `json:"status_code"`
			DurationMS int64 `json:"duration_ms"`
		}
		payload := struct {
			Method  string    `json:"method"`
			Path    string    `json:"path"`
			Sources []string  `json:"sources"`
			Live    *jsonLive `json:"live,omitempty"`
		}{
			Method:  string(insp.Endpoint.Method),
			Path:    insp.Endpoint.Path,
			Sources: insp.Endpoint.Sources,
		}
		if insp.Live != nil {
			payload.Live = &jsonLive{
				StatusCode: insp.Live.Response.StatusCode,
				DurationMS: insp.Live.Timing.Duration.Milliseconds(),
			}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	fmt.Fprintf(out, "Method:  %s\n", insp.Endpoint.Method)
	fmt.Fprintf(out, "Path:    %s\n", insp.Endpoint.Path)
	fmt.Fprintf(out, "Sources: %s\n", joinSources(insp.Endpoint.Sources))
	if insp.Endpoint.Spec != nil && insp.Endpoint.Spec.Name != "" {
		fmt.Fprintf(out, "Name:    %s\n", insp.Endpoint.Spec.Name)
	}
	if insp.Live != nil {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Live probe:")
		if insp.Live.Err != nil {
			fmt.Fprintf(out, "  error: %s\n", insp.Live.Err)
		} else {
			fmt.Fprintf(out, "  status:   %d\n", insp.Live.Response.StatusCode)
			fmt.Fprintf(out, "  duration: %dms\n", insp.Live.Timing.Duration.Milliseconds())
		}
	}
	return nil
}
