package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/internal/history"
	"github.com/spf13/cobra"
)

func newHistoryCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect captured exchanges from the current or last watch session",
	}
	cmd.AddCommand(newHistoryListCommand(flags), newHistoryShowCommand(flags), newHistoryPageMapCommand(flags))
	return cmd
}

// newHistoryPageMapCommand implements plan.md v8's "Page-to-API mapping".
// Grouped under `history` since it's derived from captured traffic, same
// data source as `history list`.
func newHistoryPageMapCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "pagemap",
		Short: "Group captured API calls by the page (Referer) that triggered them",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			mappings := eng.PageMap()
			out := cmd.OutOrStdout()

			if flags.format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(mappings)
			}

			fmt.Fprintln(out, "PAGE-TO-API MAPPING")
			fmt.Fprintln(out, "(inferred from the Referer header — a best-effort signal, not ground truth)")
			fmt.Fprintln(out)
			if len(mappings) == 0 {
				fmt.Fprintln(out, "No history yet. Run 'apilens watch' first.")
				return nil
			}
			for _, m := range mappings {
				page := m.Page
				if page == "" {
					page = "(no Referer captured)"
				}
				fmt.Fprintln(out, page)
				for _, c := range m.Calls {
					fmt.Fprintf(out, "  %-6s %-24s %dx\n", c.Method, c.Path, c.Count)
				}
			}
			return nil
		},
	}
}

func newHistoryListCommand(flags *globalFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List captured exchanges",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			exchanges, err := eng.History(limit)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if flags.format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(exchanges)
			}
			if len(exchanges) == 0 {
				fmt.Fprintln(out, "No history yet. Run 'apilens watch' first.")
				fmt.Fprintf(out, "This command reads: %s\n", history.DefaultPath(flags.project))
				if p := history.ActivePath(); p != "" && p != history.DefaultPath(flags.project) {
					fmt.Fprintf(out, "Active watch session: %s\n", p)
				}
				return nil
			}
			for _, ex := range exchanges {
				fmt.Fprintln(out, formatExchangeLine(ex))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Show only the most recent N exchanges")
	return cmd
}

func newHistoryShowCommand(flags *globalFlags) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one captured exchange in detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseDisplayID(args[0])
			if err != nil {
				return err
			}
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			ex, ok, err := eng.HistoryGet(id)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no history entry #%d", id)
			}
			out := cmd.OutOrStdout()
			if flags.format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(ex)
			}
			fmt.Fprintf(out, "#%d %s %s\n", ex.Display, ex.Request.Method, ex.Request.URL)
			fmt.Fprintf(out, "Status:   %d\n", ex.Response.StatusCode)
			fmt.Fprintf(out, "Duration: %dms\n", ex.Timing.Duration.Milliseconds())
			// Headers/body are already redacted by the proxy before they
			// ever reach history (docs/09-security.md section 3), so
			// printing them here is safe even without --verbose gating
			// on a second redaction pass — but keep the default view
			// terse per docs/08-proxy.md section 7.
			if verbose {
				fmt.Fprintln(out, "Request headers:")
				for k, v := range ex.Request.Headers {
					fmt.Fprintf(out, "  %s: %v\n", k, v)
				}
				fmt.Fprintln(out, "Response headers:")
				for k, v := range ex.Response.Headers {
					fmt.Fprintf(out, "  %s: %v\n", k, v)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show redacted headers")
	return cmd
}

// parseDisplayID accepts "42" or "#42" per docs/04-interfaces.md section 1.
func parseDisplayID(s string) (int, error) {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid history id %q", s)
		}
		n = n*10 + int(c-'0')
	}
	if s == "" {
		return 0, fmt.Errorf("invalid history id %q", s)
	}
	return n, nil
}
