package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

func newWatchCommand(flags *globalFlags) *cobra.Command {
	var (
		bind        string
		port        int
		allowRemote bool
		upstream    string
		filter      string
		host        string
		all         bool
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Start the local proxy and print API traffic as it arrives",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}

			ctx, cancel := signalContext(cmd)
			defer cancel()

			session, err := eng.Watch(ctx, apilens.WatchOptions{
				Bind:        bind,
				Port:        port,
				AllowRemote: allowRemote,
				Upstream:    upstream,
				PathPrefix:  filter,
				Host:        host,
				All:         all,
			})
			if err != nil {
				return err
			}

			printWatchBanner(cmd, session)

			for ex := range session.Events {
				printWatchLine(cmd, ex)
			}

			// Ctrl+C stops the proxy and drops in-memory history, but the
			// session file survives (docs/05-cli.md "apilens watch":
			// "Ctrl+C stops the proxy and drops in-memory history").
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Stopped. Session file kept at", session.SessionFile)
			setExitCode(ExitOK)
			return nil
		},
	}
	cmd.Flags().StringVar(&bind, "bind", "", "Bind host (default 127.0.0.1)")
	cmd.Flags().IntVar(&port, "port", 0, "Bind port (default 8888)")
	cmd.Flags().BoolVar(&allowRemote, "allow-remote", false, "Allow binding a non-loopback address (unsafe)")
	cmd.Flags().StringVar(&upstream, "upstream", "", "Reverse-proxy mode: forward everything to this base URL")
	cmd.Flags().StringVar(&filter, "filter", "", "Only capture requests whose path has this prefix")
	cmd.Flags().StringVar(&host, "host", "", "Only capture requests to this host")
	cmd.Flags().BoolVar(&all, "all", false, "Capture static assets too (disable extension filtering)")
	return cmd
}

func printWatchBanner(cmd *cobra.Command, session *apilens.WatchSession) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "API WATCHER")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Listening on %s\n", session.Addr)
	fmt.Fprintf(out, "HTTP_PROXY=http://%s\n", session.Addr)
	fmt.Fprintf(out, "History: %s\n", session.SessionFile)
	fmt.Fprintln(out, "Ctrl+C stops watch and keeps the session file until reboot.")
	fmt.Fprintln(out)
}

// printWatchLine implements docs/08-proxy.md section 7's default one-line
// format: "#1  GET   /api/profile           200    81ms".
func printWatchLine(cmd *cobra.Command, ex domain.Exchange) {
	status := ex.Response.StatusCode
	fmt.Fprintf(cmd.OutOrStdout(), "#%-3d %-6s %-24s %-6d %dms\n",
		ex.Display, ex.Request.Method, ex.Request.URL, status, ex.Timing.Duration.Milliseconds())
}

// signalContext returns a context canceled on SIGINT/SIGTERM, matching
// docs/05-cli.md's Ctrl+C handling for `apilens watch`.
func signalContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(cmd.Context())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()
	return ctx, cancel
}
