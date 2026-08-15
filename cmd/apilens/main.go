// Command apilens is the CLI entry point. It wires the Cobra command tree
// and maps the result to a process exit code (docs/05-cli.md section 4).
package main

import (
	"fmt"
	"os"

	"github.com/sandeepv/apilens/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(cli.ExitCodeForError(err))
	}
	os.Exit(cli.LastExitCode())
}
