package confine

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type fakeSessionAgentClient struct {
	PolicyClient

	mu       sync.Mutex
	sessions []agentsdk.PostAISandboxSessionRequest
	events   []agentsdk.AISandboxNetworkEvent
}

func (f *fakeSessionAgentClient) PostAISandboxSession(_ context.Context, req agentsdk.PostAISandboxSessionRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append(f.sessions, req)
	return nil
}

func (f *fakeSessionAgentClient) PatchAISandboxNetworkEvents(_ context.Context, req agentsdk.PatchAISandboxNetworkEventsRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, req.Events...)
	return nil
}

func (*fakeSessionAgentClient) PatchLogs(context.Context, agentsdk.PatchLogs) error {
	return nil
}

func TestSessionReporter(t *testing.T) {
	t.Parallel()

	client := &fakeSessionAgentClient{}
	reporter := NewSessionReporter(client, slog.Make(), codersdk.AISandboxEgressEnforcementForced)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reporter.Start(ctx)

	client.mu.Lock()
	require.Len(t, client.sessions, 1)
	opened := client.sessions[0]
	client.mu.Unlock()
	require.NotEqual(t, opened.ID.String(), "00000000-0000-0000-0000-000000000000")
	require.Equal(t, codersdk.AISandboxEgressEnforcementForced, opened.EgressEnforcement)
	require.Zero(t, opened.ChildAgentID, "the reporter is the confined agent itself; attribution comes from its binding")
	require.Nil(t, opened.EndedAt)
	require.False(t, opened.StartedAt.IsZero())

	reporter.Record(NetworkEvent{
		Protocol:       "connect",
		Host:           "denied.example.com",
		Port:           443,
		Action:         codersdk.AISandboxNetworkEventActionDenied,
		PolicyRevision: 7,
	})
	reporter.Close()

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Len(t, client.events, 1)
	event := client.events[0]
	require.Equal(t, opened.ID, event.SessionID)
	require.Equal(t, "denied.example.com", event.Host)
	require.Equal(t, codersdk.AISandboxNetworkEventActionDenied, event.Action)
	require.EqualValues(t, 7, event.PolicyRevision)

	require.Len(t, client.sessions, 2)
	closed := client.sessions[1]
	require.Equal(t, opened.ID, closed.ID)
	require.NotNil(t, closed.EndedAt)
	require.False(t, closed.EndedAt.Before(opened.StartedAt))
}
