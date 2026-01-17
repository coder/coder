package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (*RootCmd) boundary() *serpent.Command {
	return &serpent.Command{
		Use:   "boundary",
		Short: "Network isolation tool for monitoring and restricting HTTP/HTTPS requests (enterprise)",
		Long:  `boundary creates an isolated network environment for target processes. This is an enterprise feature.`,
		Handler: func(_ *serpent.Invocation) error {
			return xerrors.New("boundary is an enterprise feature; upgrade to use this command")
		},
	}
}
