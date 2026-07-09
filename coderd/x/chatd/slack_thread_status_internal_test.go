package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// statusRecordingSlackAPI records assistant thread status calls on a
// channel so tests can synchronize with the maintenance goroutine.
type statusRecordingSlackAPI struct {
	stubSlackAPI
	calls chan slack.AssistantThreadsSetStatusParameters
	err   error
}

func newStatusRecordingSlackAPI() *statusRecordingSlackAPI {
	return &statusRecordingSlackAPI{
		calls: make(chan slack.AssistantThreadsSetStatusParameters, 16),
	}
}

func (f *statusRecordingSlackAPI) SetAssistantThreadsStatusContext(_ context.Context, params slack.AssistantThreadsSetStatusParameters) error {
	f.calls <- params
	return f.err
}

func slackStatusTestLogger(t *testing.T) slog.Logger {
	// The goroutine logs warnings on Slack call failures; tests
	// exercising the failure path must not fail on them.
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

func slackLabeledChat(labels map[string]string) database.Chat {
	return database.Chat{Labels: labels}
}

func TestSlackThreadStatusGating(t *testing.T) {
	t.Parallel()

	logger := slackStatusTestLogger(t)
	clock := quartz.NewMock(t)
	api := newStatusRecordingSlackAPI()

	boundLabels := map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "C123:1700000000.000100",
	}

	t.Run("NilAPI", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, newSlackThreadStatus(nil, slackLabeledChat(boundLabels), logger, clock))
	})

	t.Run("NotSlackd", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, newSlackThreadStatus(api, slackLabeledChat(map[string]string{
			LabelSlackThread: "C123:1700000000.000100",
		}), logger, clock))
	})

	t.Run("MissingThreadLabel", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, newSlackThreadStatus(api, slackLabeledChat(map[string]string{
			LabelSlackd: "true",
		}), logger, clock))
	})

	t.Run("MalformedThreadLabel", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, newSlackThreadStatus(api, slackLabeledChat(map[string]string{
			LabelSlackd:      "true",
			LabelSlackThread: "no-separator",
		}), logger, clock))
	})

	t.Run("Bound", func(t *testing.T) {
		t.Parallel()
		s := newSlackThreadStatus(api, slackLabeledChat(boundLabels), logger, clock)
		require.NotNil(t, s)
		require.Equal(t, "C123", s.channel)
		require.Equal(t, "1700000000.000100", s.threadTS)
	})
}

func TestSlackThreadStatusLifecycle(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slackStatusTestLogger(t)
	clock := quartz.NewMock(t)
	api := newStatusRecordingSlackAPI()

	s := newSlackThreadStatus(api, slackLabeledChat(map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "C123:1700000000.000100",
	}), logger, clock)
	require.NotNil(t, s)

	// Trap ticker creation before starting so clock advances are
	// deterministic.
	tickTrap := clock.Trap().NewTicker("chatd", "slack-thread-status")
	defer tickTrap.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.start(runCtx)

	// The status is set immediately on start.
	call := testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, "C123", call.ChannelID)
	require.Equal(t, "1700000000.000100", call.ThreadTS)
	require.Equal(t, slackThreadStatusThinking, call.Status)

	tickTrap.MustWait(ctx).MustRelease(ctx)

	// Each tick re-sets the status.
	clock.Advance(slackThreadStatusInterval).MustWait(ctx)
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, slackThreadStatusThinking, call.Status)

	// A failed set does not stop the loop; the next tick retries.
	api.err = xerrors.New("slack is down")
	clock.Advance(slackThreadStatusInterval).MustWait(ctx)
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, slackThreadStatusThinking, call.Status)
	api.err = nil

	clock.Advance(slackThreadStatusInterval).MustWait(ctx)
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, slackThreadStatusThinking, call.Status)

	// Canceling the context clears the status and exits; wait returns
	// only after the clear has been issued.
	cancel()
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, "", call.Status)
	s.wait()
	select {
	case extra := <-api.calls:
		t.Fatalf("unexpected extra status call after exit: %+v", extra)
	default:
	}
}

// createRunningSlackChat creates a running chat bound to a Slack thread
// via the slackd labels.
func createRunningSlackChat(t *testing.T, f *workerTestFixture) database.Chat {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	labels, err := json.Marshal(map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: "C123:1700000000.000100",
	})
	require.NoError(t, err)
	res, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "slack test",
		ClientType:        database.ChatClientTypeApi,
		Labels:            pqtype.NullRawMessage{RawMessage: labels, Valid: true},
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	return res.Chat
}

func TestRunner_SlackThreadStatusSetWhileLiveAndClearedOnClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	f := newWorkerTestFixture(t)
	chat := createRunningSlackChat(t, f)
	api := newStatusRecordingSlackAPI()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.SlackAPI = api
	worker := startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)

	// The runner sets the status while alive.
	call := testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, "C123", call.ChannelID)
	require.Equal(t, "1700000000.000100", call.ThreadTS)
	require.Equal(t, slackThreadStatusThinking, call.Status)

	// Closing the worker tears the runner down and clears the status
	// before Close returns.
	require.NoError(t, worker.Close())
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, "", call.Status)
}

func TestRunner_SlackThreadStatusClearedOnOwnershipTakeover(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	f := newWorkerTestFixture(t)
	chat := createRunningSlackChat(t, f)
	api := newStatusRecordingSlackAPI()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.SlackAPI = api
	startWorker(t, opts)
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	call := testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, slackThreadStatusThinking, call.Status)

	// Another worker takes over the chat; the losing runner cleans up
	// and clears the status.
	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	call = testutil.RequireReceive(ctx, t, api.calls)
	require.Equal(t, "", call.Status)
}

func TestRunner_NoSlackThreadStatusForUnlabeledChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	api := newStatusRecordingSlackAPI()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.SlackAPI = api
	worker := startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)

	require.NoError(t, worker.Close())
	select {
	case call := <-api.calls:
		t.Fatalf("unexpected slack status call for unlabeled chat: %+v", call)
	default:
	}
}
