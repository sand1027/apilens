package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

// newRecordCommand implements plan.md v9's "Test recording sessions":
// turn the current history (from `apilens watch`) into a chained DSL v2
// suite on disk.
func newRecordCommand(flags *globalFlags) *cobra.Command {
	var (
		limit int
		out   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Turn the current history into a chained test suite",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			res, err := eng.Record(apilens.RecordOptions{Limit: limit, Out: out, Force: force})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Recorded %d step(s):\n", res.Steps)
			for _, f := range res.Files {
				fmt.Fprintln(w, "  "+f)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Use only the N most recently captured exchanges (default: everything in history)")
	cmd.Flags().StringVar(&out, "out", "", "Output directory (default .apilens/tests/recorded)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing recorded test files")
	return cmd
}
