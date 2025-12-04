package agent

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/proto"
)

type mockBoundaryAuditReporter struct {
	logs []*proto.ReportBoundaryAuditLogsRequest
}

func (m *mockBoundaryAuditReporter) ReportBoundaryAuditLogs(_ context.Context, req *proto.ReportBoundaryAuditLogsRequest) (*emptypb.Empty, error) {
	m.logs = append(m.logs, req)
	return &emptypb.Empty{}, nil
}

func TestBoundaryAuditListener(t *testing.T) {
	t.Parallel()

	t.Run("ReceivesBatch", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sockDir := t.TempDir()
		reporter := &mockBoundaryAuditReporter{}
		logger := slogtest.Make(t, nil)

		listener := NewBoundaryAuditListener(logger, sockDir, reporter)
		err := listener.Start(ctx)
		require.NoError(t, err)
		defer listener.Close()

		// Connect to the socket.
		conn, err := net.Dial("unix", listener.SocketPath())
		require.NoError(t, err)
		defer conn.Close()

		// Send a batch of events.
		events := []BoundaryAuditEvent{
			{Timestamp: time.Now(), ResourceType: "network", Resource: "https://github.com", Operation: "GET", Decision: "allow"},
			{Timestamp: time.Now(), ResourceType: "network", Resource: "https://malicious.com", Operation: "POST", Decision: "deny"},
		}
		data, err := json.Marshal(events)
		require.NoError(t, err)
		_, err = conn.Write(append(data, '\n'))
		require.NoError(t, err)

		// Wait for the events to be processed.
		require.Eventually(t, func() bool {
			return len(reporter.logs) > 0
		}, 5*time.Second, 100*time.Millisecond)

		// Verify the events.
		require.Len(t, reporter.logs, 1)
		require.Len(t, reporter.logs[0].Logs, 2)
		assert.Equal(t, "network", reporter.logs[0].Logs[0].ResourceType)
		assert.Equal(t, "https://github.com", reporter.logs[0].Logs[0].Resource)
		assert.Equal(t, "GET", reporter.logs[0].Logs[0].Operation)
		assert.Equal(t, "allow", reporter.logs[0].Logs[0].Decision)
		assert.Equal(t, "network", reporter.logs[0].Logs[1].ResourceType)
		assert.Equal(t, "https://malicious.com", reporter.logs[0].Logs[1].Resource)
		assert.Equal(t, "POST", reporter.logs[0].Logs[1].Operation)
		assert.Equal(t, "deny", reporter.logs[0].Logs[1].Decision)
	})

	t.Run("SocketPath", func(t *testing.T) {
		t.Parallel()

		sockDir := "/tmp/test-dir"
		listener := NewBoundaryAuditListener(nil, sockDir, nil)
		assert.Equal(t, filepath.Join(sockDir, BoundaryAuditSocketName), listener.SocketPath())
	})

	t.Run("CleansUpSocket", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sockDir := t.TempDir()
		reporter := &mockBoundaryAuditReporter{}
		logger := slogtest.Make(t, nil)

		listener := NewBoundaryAuditListener(logger, sockDir, reporter)
		err := listener.Start(ctx)
		require.NoError(t, err)

		socketPath := listener.SocketPath()
		_, err = os.Stat(socketPath)
		require.NoError(t, err, "socket should exist")

		err = listener.Close()
		require.NoError(t, err)

		_, err = os.Stat(socketPath)
		require.True(t, os.IsNotExist(err), "socket should be removed after close")
	})
}
