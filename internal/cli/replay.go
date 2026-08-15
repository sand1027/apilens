package cli

import (
	"fmt"
	"strings"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newReplayCommand(flags *globalFlags) *cobra.Command {
	var (
		method  string
		urlOv   string
		headers []string
		unset   []string
		query   []string
	)

	cmd := &cobra.Command{
		Use:   "replay <id>",
		Short: "Replay a captured exchange through the runner, with overrides",
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

			ov := apilens.ReplayOverrides{}
			if method != "" {
				ov.Method = &method
			}
			if urlOv != "" {
				ov.URL = &urlOv
			}
			if len(headers) > 0 {
				ov.Headers, err = parseKeyValuePairs(headers, ":")
				if err != nil {
					return err
				}
			}
			ov.Unset = unset
			if len(query) > 0 {
				ov.Query, err = parseKeyValuePairs(query, "=")
				if err != nil {
					return err
				}
			}

			ex, err := eng.Replay(cmd.Context(), id, ov)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "#%d %s %s\n", ex.Display, ex.Request.Method, ex.Request.URL)
			fmt.Fprintf(out, "Status:   %d\n", ex.Response.StatusCode)
			fmt.Fprintf(out, "Duration: %dms\n", ex.Timing.Duration.Milliseconds())
			return nil
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "Override the HTTP method")
	cmd.Flags().StringVar(&urlOv, "url", "", "Override the request URL")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "Set/replace a header, e.g. --header \"X-Debug: 1\"")
	cmd.Flags().StringArrayVar(&unset, "unset", nil, "Remove a header by name")
	cmd.Flags().StringArrayVar(&query, "query", nil, "Set/replace a query parameter, e.g. --query \"page=2\"")
	return cmd
}

// parseKeyValuePairs parses ["X-Debug: 1", "Accept: text"] (sep=": ") or
// ["page=2"] (sep="=") into a map, trimming surrounding whitespace.
func parseKeyValuePairs(pairs []string, sep string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		idx := strings.Index(p, sep)
		if idx < 0 {
			return nil, fmt.Errorf("invalid key%svalue pair %q", sep, p)
		}
		key := strings.TrimSpace(p[:idx])
		val := strings.TrimSpace(p[idx+len(sep):])
		out[key] = val
	}
	return out, nil
}
