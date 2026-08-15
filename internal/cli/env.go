package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// envShowVarPattern mirrors environment.envPattern; duplicated here (rather
// than exported from internal/environment) since this is purely a display
// concern for `env show`, not part of the interpolation contract.
var envShowVarPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func newEnvCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage environments",
	}
	cmd.AddCommand(newEnvListCommand(flags), newEnvUseCommand(flags), newEnvShowCommand(flags))
	return cmd
}

func newEnvListCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			envs := eng.Environments()
			if len(envs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No environments found. Run 'apilens init' or add files under .apilens/environments/.")
				return nil
			}
			current := eng.CurrentEnv()
			for _, e := range envs {
				marker := "  "
				if e.Name == current.Name {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\t%s\n", marker, e.Name, e.BaseURL)
			}
			return nil
		},
	}
}

func newEnvUseCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Select the default environment for this project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			if err := eng.UseEnv(args[0]); err != nil {
				return err
			}
			if err := eng.PersistCurrentEnv(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Using environment", args[0])
			return nil
		},
	}
}

func newEnvShowCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show an environment's base URL and variables (secrets redacted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := newEngine(flags)
			if err != nil {
				return err
			}
			var env = eng.CurrentEnv()
			if len(args) == 1 {
				found := false
				for _, e := range eng.Environments() {
					if string(e.Name) == args[0] {
						env, found = e, true
						break
					}
				}
				if !found {
					return fmt.Errorf("unknown environment %q", args[0])
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Name:     %s\n", env.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Base URL: %s\n", env.BaseURL)
			fmt.Fprintln(cmd.OutOrStdout(), "Variables:")
			for k, v := range env.Variables {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", k, displayVariableValue(k, v))
			}
			return nil
		},
	}
}

// displayVariableValue prepares a variable's value for `env show`. It
// resolves any "${ENV}" placeholder against the process environment for
// display purposes only — a missing OS variable here is not a hard error
// (unlike interpolation at request time via ADR-015); it just displays as
// "(unset: ENV_NAME)" so `env show` stays usable before secrets are
// exported. Values for keys that look like secrets are always masked
// (docs/09-security.md: security is core, not an optional reporter
// feature).
func displayVariableValue(key, value string) string {
	lowerKey := strings.ToLower(key)
	for _, sensitive := range []string{"token", "secret", "password", "key", "auth"} {
		if strings.Contains(lowerKey, sensitive) {
			return "********"
		}
	}
	return envShowVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		name := envShowVarPattern.FindStringSubmatch(match)[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return "(unset: " + name + ")"
	})
}
