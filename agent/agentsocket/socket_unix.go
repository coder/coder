//go:build !windows

package agentsocket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// createSocket creates a Unix domain socket listener
func createSocket(path string) (net.Listener, error) {
	if !isSocketAvailable(path) {
		return nil, xerrors.Errorf("socket path %s is not available", path)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, xerrors.Errorf("remove existing socket: %w", err)
	}

	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		return nil, xerrors.Errorf("create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, xerrors.Errorf("listen on unix socket: %w", err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, xerrors.Errorf("set socket permissions: %w", err)
	}
	return listener, nil
}

// getDefaultSocketPath returns the default socket path for Unix-like systems
func getDefaultSocketPath() (string, error) {
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", xerrors.Errorf("generate random socket name: %w", err)
	}
	randomSuffix := hex.EncodeToString(randomBytes)

	// Try XDG_RUNTIME_DIR first
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "coder-agent-"+randomSuffix+".sock"), nil
	}

	return filepath.Join("/tmp", "coder-agent-"+randomSuffix+".sock"), nil
}

// CleanupSocket removes the socket file
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
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("unix", path)
	if err != nil {
		// If we can't connect, the socket is not in use
		// Socket is available for use
		return true
	}
	_ = conn.Close()
	// Socket is in use
	return false
}

// NewClient creates a DRPC client for the agent socket at the given path.
func NewClient(path string, logger slog.Logger) (*Client, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, xerrors.Errorf("dial unix socket: %w", err)
	}

	config := yamux.DefaultConfig()
	config.LogOutput = nil
	config.Logger = slog.Stdlib(context.Background(), logger, slog.LevelInfo)
	session, err := yamux.Client(conn, config)
	if err != nil {
		_ = conn.Close()
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return &Client{
		DRPCAgentSocketClient: proto.NewDRPCAgentSocketClient(drpcsdk.MultiplexedConn(session)),
		conn:                  conn,
		session:               session,
	}, nil
}
