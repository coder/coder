package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsInterrupt() *serpent.Command {
	return &serpent.Command{
		Use:        "interrupt <chat-id>",
		Short:      "Interrupt a running chat.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
