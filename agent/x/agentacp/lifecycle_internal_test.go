package agentacp

import (
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type failingWriter struct{ io.WriteCloser }

func (failingWriter) Write([]byte) (int, error) { return 0, xerrors.New("broken harness input") }

func TestMessageWriteFailureCanRetry(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	s, err := m.get(ctx, info.ID)
	require.NoError(t, err)
	p := s.proc
	p.stdin = failingWriter{p.stdin}
	message := workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "retry after broken input"}
	_, err = m.Message(ctx, info.ID, message)
	require.ErrorContains(t, err, "broken harness input")
	_, err = m.Message(ctx, info.ID, message)
	require.NoError(t, err)
	result, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, "answer:"+message.Text+":other", result.AssistantResponse)
}

func TestDetachedSteeringCompletion(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	_, events, unsubscribe, err := m.Subscribe(ctx, info.ID, &info.Cursor)
	require.NoError(t, err)
	defer unsubscribe()
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hold"})
	require.NoError(t, err)
	for {
		event := testutil.RequireReceive(ctx, t, events)
		if strings.Contains(string(event.Update), `"text":"started"`) {
			break
		}
	}
	result, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "new-complete"})
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPMessageOutcomeSteering, result.Outcome)
	completed, err := awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPSessionStatusIdle, completed.Session.Status)
	require.Contains(t, completed.AssistantResponse, "answer:new-complete:other")
	// Messages following detached work must wait for its adapter-owned cleanup.
	_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "after detached"})
	require.NoError(t, err)
	completed, err = awaitTestSession(ctx, m, info.ID)
	require.NoError(t, err)
	require.Contains(t, completed.AssistantResponse, "answer:after detached:other")
}

func TestSuccessfulRecoveryAllowsAnotherImmediateRecovery(t *testing.T) {
	t.Parallel()
	m, dir, _ := newTestManager(t, fakeHarnessModeBoth)
	info := createTestSession(t, m, dir)
	ctx := testutil.Context(t, testutil.WaitLong)
	for _, text := range []string{"first", "second"} {
		_, err := m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "exit"})
		require.NoError(t, err)
		failed, err := awaitTestSession(ctx, m, info.ID)
		require.NoError(t, err)
		require.Equal(t, workspacesdk.ACPSessionStatusError, failed.Session.Status)
		_, err = m.Message(ctx, info.ID, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: text})
		require.NoError(t, err)
		completed, err := awaitTestSession(ctx, m, info.ID)
		require.NoError(t, err)
		require.Equal(t, "answer:"+text+":other", completed.AssistantResponse)
	}
}
