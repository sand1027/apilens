package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newGenerateCommand(flags *globalFlags) *cobra.Command {
	var (
		out   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "generate <id>",
		Short: "Write a YAML test from a captured exchange",
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
			result, err := eng.Generate(id, apilens.GenerateOptions{Out: out, Force: force})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Wrote", result.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default .apilens/tests/generated/<method>-<slug>.yaml)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing file")
	return cmd
}
