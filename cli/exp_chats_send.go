package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsSend() *serpent.Command {
	return &serpent.Command{
		Use:        "send <chat-id> [prompt]",
		Short:      "Send a message to an existing chat.",
		Middleware: serpent.RequireRangeArgs(1, -1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
