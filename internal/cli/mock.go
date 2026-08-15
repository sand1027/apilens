package cli

import (
	"fmt"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newMockCommand(flags *globalFlags) *cobra.Command {
	var (
		bind        string
		port        int
		allowRemote bool
	)

	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Serve mock responses from the registry and captured examples (localhost only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}

			ctx, cancel := signalContext(cmd)
			defer cancel()

			session, err := eng.Mock(ctx, apilens.MockOptions{
				Bind: bind, Port: port, AllowRemote: allowRemote,
			})
			if err != nil {
				return err
			}

			printMockBanner(cmd, session)
			<-ctx.Done()

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Stopped.")
			setExitCode(ExitOK)
			return nil
		},
	}
	cmd.Flags().StringVar(&bind, "bind", "", "Bind host (default 127.0.0.1)")
	cmd.Flags().IntVar(&port, "port", 0, "Bind port (default 4489)")
	cmd.Flags().BoolVar(&allowRemote, "allow-remote", false, "Allow binding a non-loopback address (unsafe)")
	return cmd
}

func printMockBanner(cmd *cobra.Command, session *apilens.MockSession) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "API MOCK SERVER")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Listening on http://%s\n", session.Addr)
	fmt.Fprintln(out, "Serving canned responses from the registry and captured examples.")
	fmt.Fprintln(out, "Ctrl+C stops the mock server.")
	fmt.Fprintln(out)
}
