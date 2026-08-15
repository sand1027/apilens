package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newInitCommand(flags *globalFlags) *cobra.Command {
	var (
		force  bool
		openUI bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the .apilens/ project layout",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Init does not require config/environments to already exist,
			// so build a bare Engine directly rather than via newEngine.
			eng, err := apilens.New(apilens.Options{ProjectDir: flags.project})
			if err != nil {
				return err
			}
			res, err := eng.Init(cmd.Context(), flags.project, force)
			if err != nil {
				return err
			}
			for _, c := range res.Created {
				fmt.Fprintln(cmd.OutOrStdout(), "created", c)
			}
			for _, s := range res.Skipped {
				fmt.Fprintln(cmd.OutOrStdout(), "skipped (already exists)", s)
			}
			if res.Detected != "" {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "detected", res.Detected)
			}
			for _, n := range res.Notes {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			if !openUI {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Starting dashboard (live hits widget)...")
			return runDashboard(cmd, flags, dashboardOpts{openBrowser: true})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite config.yaml if it already exists")
	cmd.Flags().BoolVar(&openUI, "ui", false, "Open the local dashboard after init (corner widget for live hits)")
	return cmd
}
