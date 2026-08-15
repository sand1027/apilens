package cli

import (
	"fmt"
	"os"

	"github.com/sandeepv/apilens/pkg/apilens"
	"github.com/spf13/cobra"
)

// globalFlags holds the persistent flags from docs/05-cli.md section 2.
type globalFlags struct {
	project string
	env     string
	config  string
	format  string
	quiet   bool
	verbose bool
}

// BuildVersion is set via -ldflags at build time (docs/05-cli.md
// `apilens version`). Defaults keep `go run`/`go build` without ldflags
// useful during development.
var (
	BuildVersion = "dev"
	BuildCommit  = "none"
	BuildDate    = "unknown"
)

// NewRootCommand builds the full `apilens` command tree.
func NewRootCommand() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:           "apilens",
		Short:         "Developer-focused API discovery, inspection, replay, and automated testing.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flags.project, "project", envOr("APILENS_PROJECT", "."), "Project root containing .apilens/")
	root.PersistentFlags().StringVar(&flags.env, "env", envOr("APILENS_ENV", ""), "Environment name")
	root.PersistentFlags().StringVar(&flags.config, "config", envOr("APILENS_CONFIG", ""), "Config path (defaults to <project>/.apilens/config.yaml)")
	root.PersistentFlags().StringVar(&flags.format, "format", envOr("APILENS_FORMAT", "terminal"), "Output format: terminal, json, or junit")
	root.PersistentFlags().BoolVar(&flags.quiet, "quiet", false, "Errors only")
	root.PersistentFlags().BoolVar(&flags.verbose, "verbose", false, "Debug logs (still redacted)")

	root.AddCommand(
		newInitCommand(flags),
		newVersionCommand(),
		newEnvCommand(flags),
		newRunCommand(flags),
		newTestCommand(flags),
		newDiscoverCommand(flags),
		newListCommand(flags),
		newInspectCommand(flags),
		newWatchCommand(flags),
		newHistoryCommand(flags),
		newReplayCommand(flags),
		newGenerateCommand(flags),
		newUICommand(flags),
	)

	return root
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// newEngine builds the Engine for the given global flags, applying --env
// as a one-invocation override on top of the persisted current environment
// (docs/05-cli.md section 2: "--env on any command overrides it for that
// invocation").
func newEngine(flags *globalFlags) (apilens.Engine, error) {
	eng, err := apilens.New(apilens.Options{
		ProjectDir: flags.project,
		ConfigPath: flags.config,
	})
	if err != nil {
		return nil, err
	}
	if flags.env != "" {
		if err := eng.UseEnv(flags.env); err != nil {
			return nil, err
		}
	}
	return eng, nil
}

func printErr(cmd *cobra.Command, err error) {
	fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err)
}
