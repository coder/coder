package cli

import (
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/enterprise/cli/boundary"
	"github.com/coder/serpent"
)

func (*RootCmd) boundary() *serpent.Command {
	cmd := boundary.BaseCommand(buildinfo.Version())
	cmd.Use += " [args...]" // The base command looks like `boundary -- command`. Serpent adds the flags piece, but we need to add the args.
	return cmd
}
