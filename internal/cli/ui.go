package cli

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/sandeepv/apilens/internal/history"
	"github.com/sandeepv/apilens/internal/security"
	"github.com/sandeepv/apilens/internal/webapi"
	"github.com/sandeepv/apilens/web/dashboard"
	"github.com/spf13/cobra"
)

type dashboardOpts struct {
	bind        string
	port        int
	allowRemote bool
	openBrowser bool
}

func newUICommand(flags *globalFlags) *cobra.Command {
	var opts dashboardOpts

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start the local web dashboard (localhost only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard(cmd, flags, opts)
		},
	}
	cmd.Flags().StringVar(&opts.bind, "bind", "", "Bind host (default 127.0.0.1)")
	cmd.Flags().IntVar(&opts.port, "port", 0, "Bind port (default 4488)")
	cmd.Flags().BoolVar(&opts.allowRemote, "allow-remote", false, "Allow binding a non-loopback address (unsafe)")
	return cmd
}

func runDashboard(cmd *cobra.Command, flags *globalFlags, opts dashboardOpts) error {
	eng, err := newEngine(flags)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", firstNonEmptyUI(opts.bind, "127.0.0.1"), firstNonZeroUI(opts.port, 4488))

	if err := security.ValidateListenAddr(addr, opts.allowRemote); err != nil {
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

	printUIBanner(cmd, ln.Addr().String(), flags.project)
	if opts.openBrowser {
		openDashboard(ln.Addr().String())
	}

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
}

func openDashboard(addr string) {
	url := "http://" + addr
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

func printUIBanner(cmd *cobra.Command, addr, projectDir string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "API DASHBOARD")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Listening on http://%s\n", addr)
	fmt.Fprintf(out, "History: %s\n", history.DefaultPath(projectDir))
	if p := history.ActivePath(); p != "" && p != history.DefaultPath(projectDir) {
		fmt.Fprintf(out, "Watch session: %s\n", p)
	}
	fmt.Fprintln(out, "Corner widget: live hits, health, and timings.")
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
