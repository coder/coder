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
func createSocket(path string) (net.Listener, error) {
	// Try Unix domain socket first (Windows 10 build 17063+)
	listener, err := net.Listen("unix", path)
	if err == nil {
		return listener, nil
	}

	// Fall back to named pipe
	pipePath := `\\.\pipe\coder-agent`
	listener, err = net.Listen("tcp", pipePath)
	if err != nil {
		return nil, err
	}
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
func IsSocketAvailable(path string, logger slog.Logger) bool {
	logger.Debug(context.Background(), "Checking socket availability on Windows", slog.F("path", path))

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Debug(context.Background(), "Socket file does not exist, path is available", slog.F("path", path))
		return true
	}
	logger.Debug(context.Background(), "Socket file exists, checking if it's listening", slog.F("path", path))

	// Try to connect to see if it's actually listening
	conn, err := net.Dial("unix", path)
	if err != nil {
		// If we can't connect, the socket is not in use
		logger.Debug(context.Background(), "Cannot connect to socket, path is available", slog.F("path", path), slog.Error(err))
		return true
	}
	_ = conn.Close()
	logger.Debug(context.Background(), "Socket is listening, path is not available", slog.F("path", path))
	return false
}

// getSocketInfo returns information about the socket file
func GetSocketInfo(path string) (*SocketInfo, error) {
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
