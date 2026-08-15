package cli

import (
	"fmt"
	"net"
	"net/http"

	"github.com/sandeepv/apilens/internal/security"
	"github.com/sandeepv/apilens/internal/webapi"
	"github.com/sandeepv/apilens/web/dashboard"
	"github.com/spf13/cobra"
)

func newUICommand(flags *globalFlags) *cobra.Command {
	var (
		bind        string
		port        int
		allowRemote bool
	)

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start the local web dashboard (localhost only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}

			addr := fmt.Sprintf("%s:%d", firstNonEmptyUI(bind, "127.0.0.1"), firstNonZeroUI(port, 4488))

			// Enforce the same loopback-only bind policy the proxy uses
			// (docs/09-security.md section 5, plan.md v6: "apilens ui —
			// localhost only") before printing a banner that claims the
			// dashboard is reachable.
			if err := security.ValidateListenAddr(addr, allowRemote); err != nil {
				return err
			}

			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}

			handler := webapi.New(eng, http.FileServer(http.FS(dashboard.FS())))
			server := &http.Server{Handler: handler}

			ctx, cancel := signalContext(cmd)
			defer cancel()

			errCh := make(chan error, 1)
			go func() { errCh <- server.Serve(ln) }()

			printUIBanner(cmd, ln.Addr().String())

			select {
			case <-ctx.Done():
				_ = server.Close()
				fmt.Fprintln(cmd.OutOrStdout(), "Stopped.")
				setExitCode(ExitOK)
				return nil
			case serveErr := <-errCh:
				if serveErr != nil && serveErr != http.ErrServerClosed {
					return serveErr
				}
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&bind, "bind", "", "Bind host (default 127.0.0.1)")
	cmd.Flags().IntVar(&port, "port", 0, "Bind port (default 4488)")
	cmd.Flags().BoolVar(&allowRemote, "allow-remote", false, "Allow binding a non-loopback address (unsafe)")
	return cmd
}

func printUIBanner(cmd *cobra.Command, addr string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "API DASHBOARD")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Listening on http://%s\n", addr)
	fmt.Fprintln(out, "Ctrl+C stops the dashboard.")
	fmt.Fprintln(out)
}

func firstNonEmptyUI(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNonZeroUI(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
