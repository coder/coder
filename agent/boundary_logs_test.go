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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/boundarylogproxy"
	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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
// it is ultimately logged by coderd with the correct structured fields
// and that usage statistics are tracked properly.
func TestBoundaryLogs_EndToEnd(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath)

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })
	const maxStalenessMs = 60000

	sink := &logSink{}
	logger := slog.Make(sink)
	workspaceID := uuid.New()
	ownerID := uuid.New()
	templateID := uuid.New()
	templateVersionID := uuid.New()
	replicaID := uuid.New()
	tracker := boundaryusage.NewTracker()

	reporter := &agentapi.BoundaryLogsAPI{
		Log:                  logger,
		WorkspaceID:          workspaceID,
		OwnerID:              ownerID,
		TemplateID:           templateID,
		TemplateVersionID:    templateVersionID,
		BoundaryUsageTracker: tracker,
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
	require.Equal(t, templateID.String(), getField(entry.Fields, "template_id"))
	require.Equal(t, templateVersionID.String(), getField(entry.Fields, "template_version_id"))
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
	require.Equal(t, templateID.String(), getField(entry.Fields, "template_id"))
	require.Equal(t, templateVersionID.String(), getField(entry.Fields, "template_version_id"))
	require.Equal(t, "POST", getField(entry.Fields, "http_method"))
	require.Equal(t, "https://blocked.com/denied", getField(entry.Fields, "http_url"))
	require.Equal(t, nil, getField(entry.Fields, "matched_rule"))

	cancel()
	<-forwarderDone

	// Verify usage tracking: flush tracker to database and check counts.
	// Use a fresh context since the forwarder context has been canceled.
	dbCtx := testutil.Context(t, testutil.WaitShort)
	err = tracker.FlushToDB(dbCtx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(dbCtx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, maxStalenessMs)
	require.NoError(t, err)

	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.UniqueUsers)
	require.Equal(t, int64(1), summary.AllowedRequests)
	require.Equal(t, int64(1), summary.DeniedRequests)
}
