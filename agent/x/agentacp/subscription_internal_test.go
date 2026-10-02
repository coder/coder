package agentacp

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSubscribeCompletion(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"idle", "error", "running", "interrupting", "canceled"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			status := workspacesdk.ACPSessionStatus(ending)
			if ending == "canceled" {
				status = workspacesdk.ACPSessionStatusRunning
			}
			m, s := newTransportSession(t, status)
			ctx := testutil.Context(t, testutil.WaitLong)
			// Historical idle statuses must not complete an active subscription.
			s.mu.Lock()
			s.info.Status = workspacesdk.ACPSessionStatusIdle
			s.appendEventLocked(workspacesdk.ACPEventKindStatus, "", nil)
			s.info.Status = status
			s.mu.Unlock()
			baseline, events, cancel, err := m.Subscribe(ctx, s.id, nil)
			require.NoError(t, err)
			defer cancel()
			require.Len(t, baseline, 2)
			require.Equal(t, workspacesdk.ACPEventKindSnapshot, baseline[1].Kind)
			require.Equal(t, status, baseline[1].Session.Status)
			switch ending {
			case "canceled":
				cancel()
				requireClosed(ctx, t, events)
				read, err := m.Read(ctx, s.id, nil)
				require.NoError(t, err)
				require.Equal(t, status, read.Session.Status)
			case "running", "interrupting":
				notification := sessionNotification(s.id, "final output")
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Notification: &notification}))
				require.NoError(t, s.ProcessACPUpdate(ctx, ACPUpdate{Prompt: &acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, Turn: 1}))
				completed, err := s.Read(ctx, nil)
				require.NoError(t, err)
				_, err = m.Message(ctx, s.id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "next turn"})
				require.NoError(t, err)
				// Drain after the next turn has discarded the completed transcript.
				for _, expected := range completed.Events[1:] {
					require.Equal(t, expected, testutil.RequireReceive(ctx, t, events))
				}
			}
			requireClosed(ctx, t, events)
		})
	}
}
