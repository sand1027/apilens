package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newGenerateCommand(flags *globalFlags) *cobra.Command {
	var (
		out          string
		force        bool
		dbHints      bool
		dbConnection string
	)

	cmd := &cobra.Command{
		Use:   "generate <id>",
		Short: "Write a YAML test from a captured exchange",
		Long: `Write a YAML test from a captured exchange.

--db-hints additionally derives assert.db checks by looking at the
captured response's "__typename"/"_id" fields (e.g. a GraphQL mutation
response containing {"__typename":"Advance","_id":"..."}) and verifying a
guessed collection name against a real, already-configured db connection
before writing it. A guess that doesn't match any real collection is
reported but never written to the file — requires db.connections to
already be set up (see "apilens configure").`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseDisplayID(args[0])
			if err != nil {
				return err
			}
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			result, err := eng.Generate(id, apilens.GenerateOptions{
				Out: out, Force: force, DBHints: dbHints, DBConnection: dbConnection,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Wrote", result.Path)
			for _, msg := range result.SkippedHints {
				fmt.Fprintln(cmd.OutOrStdout(), "  skipped db hint:", msg)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default .apilens/tests/generated/<method>-<slug>.yaml)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing file")
	cmd.Flags().BoolVar(&dbHints, "db-hints", false, "Derive assert.db checks from the response's __typename/_id fields, verified against a real db connection")
	cmd.Flags().StringVar(&dbConnection, "db-connection", "", "Which configured db connection to verify guessed collections against (default: \"main\", or the only configured connection)")
	return cmd
}
