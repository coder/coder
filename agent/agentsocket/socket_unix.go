//go:build !windows

package agentsocket

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/xerrors"
)

// createSocket creates a Unix domain socket listener
func createSocket(ctx context.Context, path string) (net.Listener, error) {
	// Remove existing socket file if it exists
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, xerrors.Errorf("remove existing socket: %w", err)
	}

	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		return nil, xerrors.Errorf("create socket directory: %w", err)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, xerrors.Errorf("listen on unix socket: %w", err)
	}

	// Set socket permissions to be accessible only by the current user
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, xerrors.Errorf("set socket permissions: %w", err)
	}

	return listener, nil
}

// getDefaultSocketPath returns the default socket path for Unix-like systems
func getDefaultSocketPath() (string, error) {
	// Try XDG_RUNTIME_DIR first
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "coder-agent.sock"), nil
	}

	// Fall back to /tmp with user-specific path
	uid := os.Getuid()
	return filepath.Join("/tmp", fmt.Sprintf("coder-agent-%d.sock", uid)), nil
}

// cleanupSocket removes the socket file
func cleanupSocket(path string) error {
	return os.Remove(path)
}

// isSocketAvailable checks if a socket path is available for use
func isSocketAvailable(path string) bool {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	// Try to connect to see if it's actually listening
	conn, err := net.Dial("unix", path)
	if err != nil {
		// If we can't connect, the socket is not in use
		return true
	}
	conn.Close()
	return false
}

// getSocketInfo returns information about the socket file
func getSocketInfo(path string) (*SocketInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	sys, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, xerrors.New("unable to get stat_t from file info")
	}
	return &SocketInfo{
		Path:    path,
		UID:     int(sys.Uid),
		GID:     int(sys.Gid),
		Mode:    stat.Mode(),
		ModTime: stat.ModTime(),
	}, nil
}

// SocketInfo contains information about a socket file
type SocketInfo struct {
	Path    string
	UID     int
	GID     int
	Mode    os.FileMode
	ModTime time.Time
}
