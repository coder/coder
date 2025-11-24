//go:build windows

package agentsocket

import (
	"net"

	"golang.org/x/xerrors"
)

// createSocket returns an error indicating that agentsocket is not supported on Windows.
// This feature is unix-only in its current experimental state.
func createSocket(_ string) (net.Listener, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}

// getDefaultSocketPath returns an error indicating that agentsocket is not supported on Windows.
// This feature is unix-only in its current experimental state.
func getDefaultSocketPath() (string, error) {
	return "", xerrors.New("agentsocket is not supported on Windows")
}

// cleanupSocket is a no-op on Windows since agentsocket is not supported.
func cleanupSocket(_ string) error {
	// No-op since agentsocket is not supported on Windows
	return nil
}
