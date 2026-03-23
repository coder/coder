package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsWatch() *serpent.Command {
	return &serpent.Command{
		Use:        "watch <chat-id>",
		Short:      "Watch a chat for live updates.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
