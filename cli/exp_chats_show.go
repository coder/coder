package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsShow() *serpent.Command {
	return &serpent.Command{
		Use:        "show <chat-id>",
		Short:      "Show details for a chat.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
