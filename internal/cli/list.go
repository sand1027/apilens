package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newListCommand(flags *globalFlags) *cobra.Command {
	var (
		method string
		tag    string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List endpoints from the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			endpoints := eng.List(apilens.EndpointFilter{Method: method, Tag: tag})
			return printEndpointList(cmd, flags, endpoints)
		},
	}
	cmd.Flags().StringVar(&method, "method", "", "Restrict to this HTTP method")
	cmd.Flags().StringVar(&tag, "tag", "", "Restrict to endpoints carrying this tag")
	return cmd
}

func printEndpointList(cmd *cobra.Command, flags *globalFlags, endpoints []apilens.Endpoint) error {
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
		}{Count: len(endpoints)}
		for _, ep := range endpoints {
			payload.Endpoints = append(payload.Endpoints, jsonEndpoint{
				Method: string(ep.Method), Path: ep.Path, Sources: ep.Sources,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if len(endpoints) == 0 {
		fmt.Fprintln(out, "No endpoints in the registry. Run 'apilens discover' first.")
		return nil
	}
	for _, ep := range endpoints {
		fmt.Fprintf(out, "%-8s%s\n", ep.Method, ep.Path)
	}
	return nil
}
