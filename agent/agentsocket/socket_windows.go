//go:build windows

package agentsocket

import (
	"context"
	"net"
	"os"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/hashicorp/yamux"
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
