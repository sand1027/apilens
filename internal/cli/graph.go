package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newGraphCommand(flags *globalFlags) *cobra.Command {
	var windowMS int

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Show an inferred API dependency graph built from captured traffic",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			g := eng.Graph(apilens.GraphOptions{WindowMS: windowMS})
			return printGraph(cmd, flags, g)
		},
	}
	cmd.Flags().IntVar(&windowMS, "window-ms", 0, "Time window (ms) for grouping calls into one burst (default 2000)")
	return cmd
}

func printGraph(cmd *cobra.Command, flags *globalFlags, g apilens.Graph) error {
	out := cmd.OutOrStdout()

	if flags.format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	}

	fmt.Fprintln(out, "API DEPENDENCY GRAPH")
	fmt.Fprintln(out, "(inferred from time-proximity in captured traffic — a hint, not ground truth)")
	fmt.Fprintln(out)
	if len(g.Edges) == 0 {
		fmt.Fprintln(out, "No inferred dependencies yet — run apilens watch and generate some traffic first.")
		return nil
	}
	for _, e := range g.Edges {
		fmt.Fprintf(out, "%s %-24s --(%dx)--> %s %s\n",
			e.From.Method, e.From.Path, e.Weight, e.To.Method, e.To.Path)
	}
	return nil
}
