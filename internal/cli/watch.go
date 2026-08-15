package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/graphqlop"
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
		browser     bool
		openURL     string
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

			printWatchBanner(cmd, session, upstream, browser, openURL)
			if browser {
				if err := openProxiedBrowser(session.Addr, openURL); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: could not open browser:", err)
				}
			}

			var st watchLineState
			out := cmd.OutOrStdout()
			for ex := range session.Events {
				for _, line := range st.push(ex) {
					fmt.Fprintln(out, line)
				}
			}
			for _, line := range st.flush() {
				fmt.Fprintln(out, line)
			}

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Stopped. Session file kept at", session.SessionFile)
			setExitCode(ExitOK)
			return nil
		},
	}
	cmd.Flags().StringVar(&bind, "bind", "", "Bind host (default 127.0.0.1)")
	cmd.Flags().IntVar(&port, "port", 0, "Bind port (default 8888)")
	cmd.Flags().BoolVar(&allowRemote, "allow-remote", false, "Allow binding a non-loopback address (unsafe)")
	cmd.Flags().StringVar(&upstream, "upstream", "", "Reverse-proxy mode (changes the URL the client must call). Prefer --browser instead.")
	cmd.Flags().StringVar(&filter, "filter", "", "Only capture requests whose path has this prefix")
	cmd.Flags().StringVar(&host, "host", "", "Only capture requests to this host")
	cmd.Flags().BoolVar(&all, "all", false, "Capture static assets too (disable extension filtering)")
	cmd.Flags().BoolVar(&browser, "browser", false, "Open a dedicated Chrome that proxies localhost (existing Safari/Chrome tabs will not appear here)")
	cmd.Flags().StringVar(&openURL, "open", "", "URL to load in the --browser Chrome window (e.g. http://localhost:3001)")
	return cmd
}

func printWatchBanner(cmd *cobra.Command, session *apilens.WatchSession, upstream string, browser bool, openURL string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "API WATCHER")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Listening on %s\n", session.Addr)
	fmt.Fprintf(out, "History: %s\n", session.SessionFile)
	fmt.Fprintln(out)
	if upstream != "" {
		fmt.Fprintf(out, "Reverse-proxy → %s\n", upstream)
		fmt.Fprintf(out, "Clients must call http://%s (this changes the URL they hit).\n", session.Addr)
	} else {
		fmt.Fprintln(out, "Forward proxy — do not change frontend or backend URLs.")
		fmt.Fprintln(out, "Your existing browser will never show up here: Chrome/Safari skip")
		fmt.Fprintln(out, "proxies for localhost. Capture dashboard traffic with:")
		fmt.Fprintln(out)
		fmt.Fprintf(out, "  apilens watch --browser --open http://localhost:3001\n")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Use only the Chrome window ApiLens opens. You should immediately see")
		fmt.Fprintln(out, "GET http://localhost:3001/... then POST .../graphql on login.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "curl still works with:")
		fmt.Fprintf(out, "  curl -x http://%s --noproxy '' http://localhost:3000/graphql \\\n", session.Addr)
		fmt.Fprintln(out, "    -H 'Content-Type: application/json' -d '{\"query\":\"{ __typename }\"}'")
	}
	if browser {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Opened a dedicated Chrome (--proxy-server + localhost). App URLs stay the same.")
		if openURL != "" {
			fmt.Fprintf(out, "Loading %s in that window.\n", openURL)
		} else {
			fmt.Fprintln(out, "In that window open the dashboard (http://localhost:3001), not your old tabs.")
		}
		fmt.Fprintln(out, "If this CLI stays empty, fully quit Chrome (Cmd+Q) and run --browser again.")
	}
	fmt.Fprintln(out, "Ctrl+C stops watch and keeps the session file until reboot.")
	fmt.Fprintln(out)
}

func chromeLaunchArgs(proxyAddr, profileDir, openURL string) []string {
	args := []string{
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-component-update",
		// PAC cannot proxy localhost in Chrome (implicit bypass is not
		// subtractable for PAC). Manual --proxy-server + <-loopback> can.
		"--proxy-server=http://" + proxyAddr,
		// <-loopback> forces localhost:3000 / :3001 through the proxy.
		// Carve out apilens ui so the in-app overlay can read history
		// without capturing its own polls (and without the proxy
		// truncating a large /api/history JSON body).
		"--proxy-bypass-list=<-loopback>;127.0.0.1:4488;localhost:4488;[::1]:4488",
	}
	if openURL != "" {
		args = append(args, openURL)
	}
	return args
}

func openProxiedBrowser(proxyAddr, openURL string) error {
	dir, err := os.MkdirTemp("", "apilens-chrome-")
	if err != nil {
		return err
	}
	args := chromeLaunchArgs(proxyAddr, dir, openURL)
	switch runtime.GOOS {
	case "darwin":
		// -n starts a new Chrome instance so flags are not ignored by an
		// already-running Chrome that owns the default profile.
		openArgs := append([]string{"-n", "-a", "Google Chrome", "--args"}, args...)
		return exec.Command("open", openArgs...).Start()
	case "linux":
		chrome := "google-chrome"
		if _, err := exec.LookPath(chrome); err != nil {
			chrome = "chromium"
		}
		return exec.Command(chrome, args...).Start()
	default:
		return fmt.Errorf("apilens watch --browser is not supported on %s", runtime.GOOS)
	}
}

type watchLineState struct {
	key   string
	line  string
	count int
}

func (s *watchLineState) push(ex domain.Exchange) []string {
	key := watchCollapseKey(ex)
	line := formatExchangeLine(ex)
	if s.count == 0 {
		s.key, s.line, s.count = key, line, 1
		return []string{line}
	}
	if key == s.key {
		s.count++
		s.line = line
		return nil
	}
	var out []string
	if s.count > 1 {
		out = append(out, fmt.Sprintf("  (×%d same)", s.count))
	}
	s.key, s.line, s.count = key, line, 1
	return append(out, line)
}

func (s *watchLineState) flush() []string {
	if s.count > 1 {
		return []string{fmt.Sprintf("  (×%d same)", s.count)}
	}
	return nil
}

func watchCollapseKey(ex domain.Exchange) string {
	method, name := graphqlop.DisplayColumns(ex.Request.Method, ex.Request.URL, ex.Request.Body)
	status := fmt.Sprintf("%d", ex.Response.StatusCode)
	if graphqlop.LooksLike(ex.Request.Body) && graphqlop.ResponseHasErrors(ex.Response.Body) {
		status += "*"
	}
	return method + " " + name + " " + status
}

func formatExchangeLine(ex domain.Exchange) string {
	method, name := graphqlop.DisplayColumns(ex.Request.Method, ex.Request.URL, ex.Request.Body)
	status := fmt.Sprintf("%d", ex.Response.StatusCode)
	if graphqlop.LooksLike(ex.Request.Body) && graphqlop.ResponseHasErrors(ex.Response.Body) {
		status += "*"
	}
	return fmt.Sprintf("#%-3d %-10s %-32s %-5s %dms",
		ex.Display, method, name, status, ex.Timing.Duration.Milliseconds())
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
