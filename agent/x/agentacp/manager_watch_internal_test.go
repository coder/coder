package agentacp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func newTransportSession(t *testing.T, status workspacesdk.ACPSessionStatus) (*Manager, *SessionFSM) {
	t.Helper()
	s, _, _ := newRuntimeSession(t, status)
	m := NewManager(context.Background(), Options{Clock: s.proc.clock, Logger: slogtest.Make(t, nil)})
	m.sessions[s.id] = s
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	return m, s
}

func TestManagerWatchAcrossTurns(t *testing.T) {
	t.Parallel()
	for _, status := range []workspacesdk.ACPSessionStatus{
		workspacesdk.ACPSessionStatusIdle,
		workspacesdk.ACPSessionStatusError,
		workspacesdk.ACPSessionStatusRunning,
		workspacesdk.ACPSessionStatusInterrupting,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			m, s := newTransportSession(t, status)
			ctx := testutil.Context(t, testutil.WaitLong)
			s.mu.Lock()
			s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
			s.mu.Unlock()
			watchCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			baseline, events, err := m.Watch(watchCtx, s.id, nil)
			require.NoError(t, err)
			read, err := m.Read(ctx, s.id, nil)
			require.NoError(t, err)
			require.Equal(t, read, baseline)
			if status == workspacesdk.ACPSessionStatusRunning || status == workspacesdk.ACPSessionStatusInterrupting {
				notification := sessionNotification(s.id, "final output")
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
				completed, err := s.Read(ctx, nil)
				require.NoError(t, err)
				for _, expected := range completed.Events[len(baseline.Events):] {
					require.Equal(t, expected, testutil.RequireReceive(ctx, t, events))
				}
			}
			if status != workspacesdk.ACPSessionStatusError {
				_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{Text: "next turn"})
				require.NoError(t, err)
				read, err = m.Read(ctx, s.id, nil)
				require.NoError(t, err)
				for _, expected := range read.Events {
					require.Equal(t, expected, testutil.RequireReceive(ctx, t, events))
				}
			}
			cancel()
			requireClosed(ctx, t, events)
			current, err := m.Read(ctx, s.id, nil)
			require.NoError(t, err)
			require.Equal(t, read, current)
		})
	}
}
