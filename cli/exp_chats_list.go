package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsList() *serpent.Command {
	return &serpent.Command{
		Use:        "list",
		Short:      "List chats.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
