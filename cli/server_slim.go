//go:build slim

package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd"
)

func (r *RootCmd) Server(_ func(context.Context, *coderd.Options) (*coderd.API, io.Closer, error)) *clibase.Cmd {
	root := &clibase.Cmd{
		Use:   "server",
		Short: "Start a Coder server",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			serverUnsupported(inv.Stderr)
			return nil
		},
	}

	return root
}

func serverUnsupported(w io.Writer) {
	_, _ = fmt.Fprintf(w, "You are using a 'slim' build of Coder, which does not support the %s subcommand.\n", cliui.DefaultStyles.Code.Render("server"))
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Please use a build of Coder from GitHub releases:")
	_, _ = fmt.Fprintln(w, "  https://github.com/coder/coder/releases")
	os.Exit(1)
}
