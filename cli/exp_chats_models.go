package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsModels() *serpent.Command {
	return &serpent.Command{
		Use:        "models",
		Short:      "List available chat models.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
