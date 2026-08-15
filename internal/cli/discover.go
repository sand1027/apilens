package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newDiscoverCommand(flags *globalFlags) *cobra.Command {
	var (
		source  []string
		path    []string
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find APIs in the project and update the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			res, err := eng.Discover(cmd.Context(), apilens.DiscoverOptions{Enabled: source, Paths: path})
			if err != nil {
				return err
			}
			return printDiscoverResult(cmd, flags, res, verbose)
		},
	}
	cmd.Flags().StringSliceVar(&source, "source", nil, "Limit to these provider names (e.g. openapi, graphql, express)")
	cmd.Flags().StringSliceVar(&path, "path", nil, "Explicit spec/source path(s), bypassing auto-detection")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show the source column")
	return cmd
}

func printDiscoverResult(cmd *cobra.Command, flags *globalFlags, res *apilens.DiscoverResult, verbose bool) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		type jsonEndpoint struct {
			Method  string   `json:"method"`
			Path    string   `json:"path"`
			Sources []string `json:"sources"`
		}
		payload := struct {
			Count     int            `json:"count"`
			Endpoints []jsonEndpoint `json:"endpoints"`
		}{Count: len(res.Endpoints)}
		for _, ep := range res.Endpoints {
			payload.Endpoints = append(payload.Endpoints, jsonEndpoint{
				Method: string(ep.Method), Path: ep.Path, Sources: ep.Sources,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	fmt.Fprintln(out, "API DISCOVERY")
	fmt.Fprintln(out)
	for _, ep := range res.Endpoints {
		if verbose {
			fmt.Fprintf(out, "%-8s%-28s%s\n", ep.Method, ep.Path, joinSources(ep.Sources))
		} else {
			fmt.Fprintf(out, "%-8s%s\n", ep.Method, ep.Path)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Found: %d APIs\n", len(res.Endpoints))

	for _, e := range res.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", e.Provider, e.Message)
	}
	return nil
}

func joinSources(sources []string) string {
	out := ""
	for i, s := range sources {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
