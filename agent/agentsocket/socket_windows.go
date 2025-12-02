//go:build windows

package agentsocket

import (
	"context"
	"net"

	"golang.org/x/xerrors"
)

func createSocket(_ string) (net.Listener, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}

func cleanupSocket(_ string) error {
	return nil
}

func dialSocket(_ context.Context, _ string) (net.Conn, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}
