//go:build slim

package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
	"golang.org/x/xerrors"
)

func (*RootCmd) dbcryptRotate() *clibase.Cmd {
	return &clibase.Cmd{
		Use:     "dbcrypt-rotate --postgres-url <postgres_url> --external-token-encryption-keys <new-key>,<old-key>",
		Short:   "Rotate database encryption keys",
		Options: clibase.OptionSet{},
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			return xerrors.Errorf("slim build does not support `coder dbcrypt-rotate`")
		},
	}
}
