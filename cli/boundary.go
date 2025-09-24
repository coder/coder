package cli

import (
	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/serpent"
)

func (*RootCmd) boundary() *serpent.Command {
	cmd := boundarycli.BaseCommand() // Package coder/jail/cli exports a "base command" designed to be integrated as a subcommand.
	cmd.Hidden = true                // We want jail to be a hidden command in coder for now.
	cmd.Use += " [args...]"          // The base command looks like `jail -- command`. Serpent adds the flags piece, but we need to add the args.
	return cmd
}
