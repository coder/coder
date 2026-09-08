//go:build !linux

package xvnc

import (
	"context"

	"golang.org/x/xerrors"
)

// Start is unsupported outside Linux because no desktop runtime is built for
// other platforms.
func Start(_ context.Context, _ Options) (*Server, error) {
	return nil, xerrors.New("embedded Xvnc is only supported on linux")
}
