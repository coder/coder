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

	"cdr.dev/slog"
)

// createSocket creates a Unix domain socket listener
func createSocket(ctx context.Context, path string, logger slog.Logger) (net.Listener, error) {
	logger.Debug(ctx, "SOCKET_DEBUG: Creating Unix domain socket", slog.F("path", path))

	// Remove existing socket file if it exists
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Error(ctx, "SOCKET_DEBUG: Failed to remove existing socket", slog.Error(err), slog.F("path", path))
		return nil, xerrors.Errorf("remove existing socket: %w", err)
	}
	logger.Debug(ctx, "SOCKET_DEBUG: Removed existing socket file (if it existed)", slog.F("path", path))

	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(path)
	logger.Debug(ctx, "SOCKET_DEBUG: Creating parent directory", slog.F("parent_dir", parentDir))
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		logger.Error(ctx, "SOCKET_DEBUG: Failed to create socket directory", slog.Error(err), slog.F("parent_dir", parentDir))
		return nil, xerrors.Errorf("create socket directory: %w", err)
	}
	logger.Debug(ctx, "SOCKET_DEBUG: Created parent directory successfully", slog.F("parent_dir", parentDir))

	// Create Unix domain socket listener
	logger.Debug(ctx, "SOCKET_DEBUG: Creating Unix domain socket listener", slog.F("path", path))
	listener, err := net.Listen("unix", path)
	if err != nil {
		logger.Error(ctx, "SOCKET_DEBUG: Failed to create Unix domain socket listener", slog.Error(err), slog.F("path", path))
		return nil, xerrors.Errorf("listen on unix socket: %w", err)
	}
	logger.Debug(ctx, "SOCKET_DEBUG: Created Unix domain socket listener successfully", slog.F("path", path))

	// Set socket permissions to be accessible only by the current user
	logger.Debug(ctx, "SOCKET_DEBUG: Setting socket permissions", slog.F("path", path))
	if err := os.Chmod(path, 0o600); err != nil {
		logger.Error(ctx, "SOCKET_DEBUG: Failed to set socket permissions", slog.Error(err), slog.F("path", path))
		_ = listener.Close()
		return nil, xerrors.Errorf("set socket permissions: %w", err)
	}
	logger.Debug(ctx, "SOCKET_DEBUG: Set socket permissions successfully", slog.F("path", path))

	logger.Info(ctx, "SOCKET_DEBUG: Unix domain socket created successfully", slog.F("path", path))
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
func isSocketAvailable(path string, logger slog.Logger) bool {
	logger.Debug(context.Background(), "SOCKET_DEBUG: Checking socket availability", slog.F("path", path))

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Debug(context.Background(), "SOCKET_DEBUG: Socket file does not exist, path is available", slog.F("path", path))
		return true
	}
	logger.Debug(context.Background(), "SOCKET_DEBUG: Socket file exists, checking if it's listening", slog.F("path", path))

	// Try to connect to see if it's actually listening
	conn, err := net.Dial("unix", path)
	if err != nil {
		// If we can't connect, the socket is not in use
		logger.Debug(context.Background(), "SOCKET_DEBUG: Cannot connect to socket, path is available", slog.F("path", path), slog.Error(err))
		return true
	}
	_ = conn.Close()
	logger.Debug(context.Background(), "SOCKET_DEBUG: Socket is listening, path is not available", slog.F("path", path))
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
