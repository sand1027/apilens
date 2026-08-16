package cli

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandeepv/apilens/internal/config"
	"github.com/sandeepv/apilens/internal/secrets"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newConfigureCommand implements `apilens configure`: a guided way to set
// up the two credentials most projects need before assert.db or
// auth-protected tests can run -- an auth token and a database
// connection -- without hand-editing YAML or exporting env vars in every
// shell.
//
// The auth token is NEVER read from an interactive prompt. A real-world
// token (e.g. a JWT) routinely runs well past 1024 bytes, and most
// terminals' canonical/cooked line discipline has a hard per-line input
// buffer around that size (a kernel tty limit, not something this
// program's stdin-reading code can work around) -- past that limit the
// terminal simply stops accepting keystrokes for the line, Enter does
// nothing, and the process looks "stuck" with no way to recover short of
// Ctrl+C. The token must come from --token or the AUTH_TOKEN environment
// variable instead, both of which bypass line-buffered terminal input
// entirely. Only the short fields (environment name, db driver, db DSN)
// use the interactive prompt.
//
// Values are never written as plaintext into a file that could be
// committed:
//   - The auth token goes into .apilens/environments/<env>.secrets.yaml
//     (already gitignored by `apilens init`, already the documented
//     place for a token -- docs/06-test-dsl.md section 8) as
//     variables.token, with AUTH_TOKEN also persisted to
//     .apilens/.secrets.env so any test file's own "${AUTH_TOKEN}"
//     reference resolves too.
//   - The database DSN is persisted only to .apilens/.secrets.env as
//     DATABASE_URL; config.yaml's db.connections.<name>.dsn is set to
//     the placeholder "${DATABASE_URL}", never the literal value.
func newConfigureCommand(flags *globalFlags) *cobra.Command {
	var (
		token        string
		env          string
		dbDriver     string
		dbDSN        string
		nonInter     bool
		connName     string
		envFile      string
		envFileKey   string
		tokenEnvFile string
		tokenEnvKey  string
	)

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Set the auth token and database connection for this project",
		Long: `Set the auth token and database connection for this project.

The auth token is NEVER prompted for interactively -- pass it as a flag
or export it first, since a real token (e.g. a JWT) is routinely too long
for a terminal's line-input buffer to accept, which makes an interactive
prompt appear to hang with no way to recover except Ctrl+C:

  export AUTH_TOKEN='...'
  apilens configure --token "$AUTH_TOKEN"

The database driver/DSN are short enough to prompt for interactively, so
running with no flags at all still asks about those:

  apilens configure

Or do everything non-interactively in one line for CI:

  apilens configure --token "$AUTH_TOKEN" --db-driver sqlite --db-dsn "file:./app.db"

If your app already has the DSN in its own .env file (e.g.
apps/api/.env's MONGO_CONNECTION_URL), read it directly from there
instead of exporting it into the shell yourself first:

  apilens configure --db-driver mongodb \
    --db-dsn-from-env-file apps/api/.env --db-dsn-key MONGO_CONNECTION_URL

--token-from-env-file/--token-env-key work the same way for the auth
token.

Secrets are never written to a file that could end up in git: the token
goes to .apilens/environments/<env>.secrets.yaml and .apilens/.secrets.env
(both gitignored); the database DSN goes only to .apilens/.secrets.env,
with config.yaml referencing it as "${DATABASE_URL}".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := bufio.NewScanner(cmd.InOrStdin())
			out := cmd.OutOrStdout()

			if tokenEnvFile != "" {
				v, err := readEnvFileKey(tokenEnvFile, defaultStr(tokenEnvKey, "AUTH_TOKEN"))
				if err != nil {
					return err
				}
				token = v
			}
			if dbDSN == "" && envFile != "" {
				v, err := readEnvFileKey(envFile, defaultStr(envFileKey, "DATABASE_URL"))
				if err != nil {
					return err
				}
				dbDSN = v
			}

			// The token is only ever taken from --token, --token-from-env-file,
			// or AUTH_TOKEN -- never prompted (see the Long help text above
			// for why).
			if token == "" {
				token = os.Getenv("AUTH_TOKEN")
			}

			interactiveDB := !nonInter && dbDriver == "" && dbDSN == ""
			if interactiveDB {
				fmt.Fprintf(out, "Environment [%s]: ", defaultStr(env, "local"))
				if v := readLine(in); v != "" {
					env = v
				}

				fmt.Fprint(out, "Database driver (sqlite/postgres/mongodb/skip) [skip]: ")
				dbDriver = readLine(in)

				if dbDriver != "" && !strings.EqualFold(dbDriver, "skip") {
					fmt.Fprint(out, "Database DSN: ")
					dbDSN = readLine(in)
				}
			}
			if env == "" {
				env = "local"
			}
			if connName == "" {
				connName = "main"
			}

			projectDir := flags.project
			var wrote []string

			if token != "" {
				envPath := filepath.Join(projectDir, ".apilens", "environments", env+".secrets.yaml")
				if err := writeTokenSecretsFile(envPath, token); err != nil {
					return err
				}
				wrote = append(wrote, envPath)

				secretsPath := secrets.Path(projectDir)
				if err := secrets.Set(secretsPath, map[string]string{"AUTH_TOKEN": token}); err != nil {
					return err
				}
				wrote = append(wrote, secretsPath)
			}

			if dbDriver != "" && !strings.EqualFold(dbDriver, "skip") {
				if dbDSN == "" {
					return fmt.Errorf("--db-dsn is required when --db-driver is set")
				}
				if err := config.ValidateDBDriver(dbDriver); err != nil {
					return err
				}
				if isMongoDriver(dbDriver) {
					if err := validateMongoDSN(dbDSN); err != nil {
						return err
					}
				}

				secretsPath := secrets.Path(projectDir)
				if err := secrets.Set(secretsPath, map[string]string{"DATABASE_URL": dbDSN}); err != nil {
					return err
				}
				wrote = append(wrote, secretsPath)

				configPath := flags.config
				if configPath == "" {
					configPath = filepath.Join(projectDir, ".apilens", "config.yaml")
				}
				if err := config.SetDBConnection(configPath, connName, dbDriver, "DATABASE_URL"); err != nil {
					return err
				}
				wrote = append(wrote, configPath)
			}

			if len(wrote) == 0 {
				fmt.Fprintln(out, "Nothing to configure -- pass --token (or export AUTH_TOKEN) and/or --db-driver.")
				return nil
			}

			fmt.Fprintln(out, "Updated:")
			for _, w := range dedupe(wrote) {
				fmt.Fprintln(out, " ", w)
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "These files hold credentials and are gitignored -- do not commit them.")
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "Auth token to store for the environment (never prompted; falls back to $AUTH_TOKEN if unset)")
	cmd.Flags().StringVar(&env, "env", "", "Environment name the token applies to (default: local)")
	cmd.Flags().StringVar(&dbDriver, "db-driver", "", "Database driver: sqlite, postgres, mongodb, or skip")
	cmd.Flags().StringVar(&dbDSN, "db-dsn", "", "Database connection string (required if --db-driver is set; for mongodb, must include a database name, e.g. mongodb://host:27017/mydb)")
	cmd.Flags().StringVar(&connName, "db-connection", "main", "Connection name to write under db.connections in config.yaml")
	cmd.Flags().StringVar(&envFile, "db-dsn-from-env-file", "", "Read the DSN from a KEY=VALUE .env file instead of --db-dsn (e.g. apps/api/.env)")
	cmd.Flags().StringVar(&envFileKey, "db-dsn-key", "DATABASE_URL", "Key to read from --db-dsn-from-env-file")
	cmd.Flags().StringVar(&tokenEnvFile, "token-from-env-file", "", "Read the auth token from a KEY=VALUE .env file instead of --token")
	cmd.Flags().StringVar(&tokenEnvKey, "token-env-key", "AUTH_TOKEN", "Key to read from --token-from-env-file")
	cmd.Flags().BoolVar(&nonInter, "yes", false, "Do not prompt; use flags as given (fails if a required value is missing)")
	return cmd
}

// readEnvFileKey reads a single key out of a dotenv-style file, failing
// with a clear error (naming the file and key) rather than silently
// falling through to an empty DSN/token if the key is missing or the
// file doesn't exist -- both of which would otherwise surface much later
// as a confusing "MongoDB DSN must include a database name" or similar.
func readEnvFileKey(path, key string) (string, error) {
	kv, err := secrets.ReadEnvFile(path)
	if err != nil {
		return "", err
	}
	v, ok := kv[key]
	if !ok || v == "" {
		return "", fmt.Errorf("%q not found in %s", key, path)
	}
	return v, nil
}

func readLine(s *bufio.Scanner) string {
	if !s.Scan() {
		return ""
	}
	return strings.TrimSpace(s.Text())
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// secretsFileDoc mirrors environment.fileDocument's on-disk shape (that
// type is unexported, so it is duplicated here rather than imported --
// this is purely a "write one field" concern, not part of the
// interpolation contract internal/environment owns).
type secretsFileDoc struct {
	BaseURL   string            `yaml:"base_url,omitempty"`
	Variables map[string]string `yaml:"variables"`
}

// writeTokenSecretsFile sets variables.token in the "<env>.secrets.yaml"
// overlay at path, merging with (rather than clobbering) any other
// variables already saved there.
func writeTokenSecretsFile(path, token string) error {
	doc := secretsFileDoc{Variables: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if doc.Variables == nil {
			doc.Variables = map[string]string{}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	doc.Variables["token"] = token

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// isMongoDriver mirrors dbassert.isMongoDriver (unexported, so duplicated
// here — this is a two-line classification, not worth exporting a new
// dbassert API just for the CLI's own DSN sanity check below).
func isMongoDriver(name string) bool {
	switch strings.ToLower(name) {
	case "mongodb", "mongo":
		return true
	default:
		return false
	}
}

// validateMongoDSN catches the one MongoDB-specific mistake
// dbassert.mongoDatabaseNameFromDSN would otherwise only surface at the
// first `apilens run` (a DSN with no database name in its path) --
// failing here, at configure time, gives a much clearer error than
// discovering it mid test-suite.
func validateMongoDSN(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("invalid MongoDB connection string: %w", err)
	}
	if strings.TrimPrefix(u.Path, "/") == "" {
		return fmt.Errorf("MongoDB DSN must include a database name, e.g. \"mongodb://host:27017/mydb\" (got %q)", dsn)
	}
	return nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
