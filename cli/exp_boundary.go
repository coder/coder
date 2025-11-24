package cli

import (
	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/serpent"
)

func (*RootCmd) boundary() *serpent.Command {
	cmd := boundarycli.BaseCommand() // Package coder/boundary/cli exports a "base command" designed to be integrated as a subcommand.
	cmd.Use += " [args...]"          // The base command looks like `boundary -- command`. Serpent adds the flags piece, but we need to add the args.
	return cmd
}
