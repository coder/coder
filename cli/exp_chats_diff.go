package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsDiff() *serpent.Command {
	return &serpent.Command{
		Use:        "diff <chat-id>",
		Short:      "Show the diff for a chat.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
