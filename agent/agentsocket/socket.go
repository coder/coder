//go:build !windows

package agentsocket

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
)

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
