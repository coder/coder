package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsTranscript() *serpent.Command {
	return &serpent.Command{
		Use:        "transcript <chat-id>",
		Short:      "Show the transcript of a chat.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
