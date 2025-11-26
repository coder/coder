//go:build !windows

package agentsocket

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
)

const defaultSocketPath = "/tmp/coder-agent.sock"

func createSocket(path string) (net.Listener, error) {
	if !isSocketAvailable(path) {
		return nil, xerrors.Errorf("socket path %s is not available", path)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, xerrors.Errorf("remove existing socket: %w", err)
	}

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

func cleanupSocket(path string) error {
	return os.Remove(path)
}

func isSocketAvailable(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	// Try to connect to see if it's actually listening.
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("unix", path)
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}
