//go:build windows

package agentsocket

import (
	"net"
	"os"
	"time"

	"golang.org/x/xerrors"
)

// createSocket returns an error indicating that agentsocket is not supported on Windows.
// This feature is unix-only in its current experimental state.
func createSocket(path string) (net.Listener, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}

// getDefaultSocketPath returns an error indicating that agentsocket is not supported on Windows.
// This feature is unix-only in its current experimental state.
func getDefaultSocketPath() (string, error) {
	return "", xerrors.New("agentsocket is not supported on Windows")
}

// cleanupSocket is a no-op on Windows since agentsocket is not supported.
func cleanupSocket(path string) error {
	// No-op since agentsocket is not supported on Windows
	return nil
}

// isSocketAvailable always returns false on Windows since agentsocket is not supported.
func isSocketAvailable(path string) bool {
	// Always return false since agentsocket is not supported on Windows
	return false
}

// GetSocketInfo returns an error indicating that agentsocket is not supported on Windows.
// This function is kept for API compatibility but will always return an error.
func GetSocketInfo(path string) (*SocketInfo, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}

// SocketInfo contains information about a socket file.
// This type is kept for API compatibility but is not used on Windows.
type SocketInfo struct {
	Path    string
	UID     int
	GID     int
	Mode    os.FileMode
	ModTime time.Time
	Owner   string
	Group   string
}
