package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) chatsStart() *serpent.Command {
	return &serpent.Command{
		Use:   "start [prompt]",
		Short: "Start a new chat.",
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("not yet implemented")
		},
	}
}
