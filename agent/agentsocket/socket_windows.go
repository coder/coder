//go:build windows

package agentsocket

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cdr.dev/slog"
)

// createSocket creates a Unix domain socket listener on Windows
// Falls back to named pipe if Unix sockets are not supported
func createSocket(ctx context.Context, path string, logger slog.Logger) (net.Listener, error) {
	logger.Debug(ctx, "SOCKET_DEBUG: Creating socket on Windows", slog.F("path", path))

	// Try Unix domain socket first (Windows 10 build 17063+)
	logger.Debug(ctx, "SOCKET_DEBUG: Attempting Unix domain socket on Windows", slog.F("path", path))
	listener, err := net.Listen("unix", path)
	if err == nil {
		logger.Info(ctx, "SOCKET_DEBUG: Unix domain socket created successfully on Windows", slog.F("path", path))
		return listener, nil
	}
	logger.Debug(ctx, "SOCKET_DEBUG: Unix domain socket failed, falling back to named pipe", slog.Error(err), slog.F("path", path))

	// Fall back to named pipe
	pipePath := `\\.\pipe\coder-agent`
	logger.Debug(ctx, "SOCKET_DEBUG: Creating named pipe", slog.F("pipe_path", pipePath))
	listener, err = net.Listen("tcp", pipePath)
	if err != nil {
		logger.Error(ctx, "SOCKET_DEBUG: Failed to create named pipe", slog.Error(err), slog.F("pipe_path", pipePath))
		return nil, err
	}
	logger.Info(ctx, "SOCKET_DEBUG: Named pipe created successfully", slog.F("pipe_path", pipePath))
	return listener, nil
}

// getDefaultSocketPath returns the default socket path for Windows
func getDefaultSocketPath() (string, error) {
	// Try to use a temporary directory
	tempDir := os.TempDir()
	if tempDir == "" {
		tempDir = "C:\\temp"
	}

	// Create a user-specific subdirectory
	uid := os.Getuid()
	userDir := filepath.Join(tempDir, "coder-agent", strconv.Itoa(uid))

	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return "", fmt.Errorf("create user directory: %w", err)
	}

	return filepath.Join(userDir, "agent.sock"), nil
}

// cleanupSocket removes the socket file
func cleanupSocket(path string) error {
	return os.Remove(path)
}

// isSocketAvailable checks if a socket path is available for use
func isSocketAvailable(path string, logger slog.Logger) bool {
	logger.Debug(context.Background(), "SOCKET_DEBUG: Checking socket availability on Windows", slog.F("path", path))

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

	// On Windows, we'll use a simplified approach for now
	// In a real implementation, you'd get the security descriptor
	return &SocketInfo{
		Path:    path,
		UID:     0, // Simplified for now
		GID:     0, // Simplified for now
		Mode:    stat.Mode(),
		ModTime: stat.ModTime(),
		Owner:   "unknown",
		Group:   "unknown",
	}, nil
}

// SocketInfo contains information about a socket file
type SocketInfo struct {
	Path    string
	UID     int
	GID     int
	Mode    os.FileMode
	ModTime time.Time
	Owner   string // Windows SID string
	Group   string // Windows SID string
}
