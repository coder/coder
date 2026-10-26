//go:build linux

package agentnative

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerNativeCancellationFlag(t *testing.T) {
	t.Parallel()
	backend := &managerTestBackend{}
	backend.prompt = func(_ context.Context, session Session, prompt Prompt) error {
		return backend.host.Publish(session.ID, []Event{{Type: "prompt.rejected", Data: map[string]any{
			"requestId": prompt.ID,
			"cancelled": true, //nolint:misspell // Native event protocol spelling.
		}}}, nil)
	}
	manager := newTestManager(t, filepath.Join(t.TempDir(), "state"), backend)
	job, err := manager.RunJob(t.Context(), JobSpec{Cwd: t.TempDir(), Prompt: "initial task"})
	require.NoError(t, err)
	require.Equal(t, "canceled", job.Status)
	manager.mu.Lock()
	request := manager.state.Sessions[job.SessionID].Requests[job.RequestID]
	manager.mu.Unlock()
	require.Equal(t, "canceled", request.Status)
	events, _, err := manager.Events(t.Context(), job.SessionID, 0, false)
	require.NoError(t, err)
	require.Equal(t, true, events[len(events)-1].Data["cancelled"]) //nolint:misspell // Preserve native event spelling for ACP.
	require.NotContains(t, events[len(events)-1].Data, "canceled")
}
