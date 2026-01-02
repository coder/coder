//go:build linux || darwin

package agent_test

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/boundarylogproxy"
	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/testutil"
)

// logSink captures structured log entries for testing.
type logSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *logSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*logSink) Sync() {}

func (s *logSink) getEntries() []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]slog.SinkEntry{}, s.entries...)
}

// getField returns the value of a field by name from a slog.Map.
func getField(fields slog.Map, name string) interface{} {
	for _, f := range fields {
		if f.Name == name {
			return f.Value
		}
	}
	return nil
}

func sendBoundaryLogsRequest(t *testing.T, conn net.Conn, req *agentproto.ReportBoundaryLogsRequest) {
	t.Helper()

	data, err := proto.Marshal(req)
	require.NoError(t, err)

	err = codec.WriteFrame(conn, codec.TagV1, data)
	require.NoError(t, err)
}

// TestBoundaryLogs_EndToEnd is an end-to-end test that sends a protobuf
// message over the agent's unix socket (as boundary would) and verifies
// it is ultimately logged by coderd with the correct structured fields.
func TestBoundaryLogs_EndToEnd(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath)

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	sink := &logSink{}
	logger := slog.Make(sink)
	workspaceID := uuid.New()
	reporter := &agentapi.BoundaryLogsAPI{
		Log:         logger,
		WorkspaceID: workspaceID,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Allowed HTTP request.
	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method:      "GET",
						Url:         "https://example.com/allowed",
						MatchedRule: "*.example.com",
					},
				},
			},
		},
	}
	sendBoundaryLogsRequest(t, conn, req)

	require.Eventually(t, func() bool {
		return len(sink.getEntries()) >= 1
	}, testutil.WaitShort, testutil.IntervalFast)

	entries := sink.getEntries()
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, slog.LevelInfo, entry.Level)
	require.Equal(t, "boundary_request", entry.Message)
	require.Equal(t, "allow", getField(entry.Fields, "decision"))
	require.Equal(t, workspaceID.String(), getField(entry.Fields, "workspace_id"))
	require.Equal(t, "GET", getField(entry.Fields, "http_method"))
	require.Equal(t, "https://example.com/allowed", getField(entry.Fields, "http_url"))
	require.Equal(t, "*.example.com", getField(entry.Fields, "matched_rule"))

	// Denied HTTP request.
	req2 := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: false,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "POST",
						Url:    "https://blocked.com/denied",
					},
				},
			},
		},
	}
	sendBoundaryLogsRequest(t, conn, req2)

	require.Eventually(t, func() bool {
		return len(sink.getEntries()) >= 2
	}, testutil.WaitShort, testutil.IntervalFast)

	entries = sink.getEntries()
	entry = entries[1]
	require.Len(t, entries, 2)
	require.Equal(t, slog.LevelInfo, entry.Level)
	require.Equal(t, "boundary_request", entry.Message)
	require.Equal(t, "deny", getField(entry.Fields, "decision"))
	require.Equal(t, workspaceID.String(), getField(entry.Fields, "workspace_id"))
	require.Equal(t, "POST", getField(entry.Fields, "http_method"))
	require.Equal(t, "https://blocked.com/denied", getField(entry.Fields, "http_url"))
	require.Equal(t, nil, getField(entry.Fields, "matched_rule"))

	cancel()
	<-forwarderDone
}
