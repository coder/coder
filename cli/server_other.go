//go:build !linux

package cli

import (
	"context"

	"golang.org/x/xerrors"
)

func startBuiltinPostgresAs(ctx context.Context, uid, gid uint32, stdout, stderr io.Writer, cfg config.Root, connectionURL string) (closer func() error, err error) {
	return nil, xerrors.New("not implemented")
}
