package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunner_IgnoresDuplicateStateNotifications(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	starter.waitCall(t, taskKindGeneration, chat.ID)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)

	publishChatUpdate(t, f, latest)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

func TestRunner_CancelsActiveTaskWhenHistoryChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "first step")
	require.Greater(t, updated.HistoryVersion, first.input.HistoryVersion)
	requireTaskCanceled(t, first)
	require.NotErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
	require.Same(t, first.input.SessionStart, second.input.SessionStart)
}

func TestRunner_CancelsActiveTaskWhenStatusChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := interruptChat(t, f, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)
	requireTaskCanceled(t, first)
	second := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_CleansUpOnOwnershipTakeover(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	starter.assertNoCall(t)
}

func TestRunner_SerializesReplacementTasksForSameHistoryAndStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusInterrupting, false)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusRunning, false)
	starter.assertNoCall(t)

	starter.release(t, 0)
	replacement := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, replacement.input.HistoryVersion)
}

func TestRunner_AllowsReplacementForDifferentHistoryOrStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "different history")
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Greater(t, second.input.HistoryVersion, first.input.HistoryVersion)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_TaskTimeoutRetries(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	timeoutTrap := clock.Trap().AfterFunc("chatworker", "task-timeout-generation")
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.TaskRetryInitialBackoff = time.Minute
	opts.TaskRetryMaxBackoff = time.Minute
	startWorker(t, opts)

	timeoutTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	timeoutTrap.Close()
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	retryTrap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer retryTrap.Close()

	ctx := testutil.Context(t, testutil.WaitLong)
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	retryTrap.MustWait(ctx).MustRelease(ctx)
	require.ErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	clock.Advance(time.Minute).MustWait(ctx)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, second.input.HistoryVersion)
}

func TestWorker_RoutesDatabaseSyncStateToActiveRunner(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.RunnerSyncInterval = time.Minute
	startWorker(t, opts)
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	forceExecutionState(t, f, chat.ID, database.ChatStatusInterrupting, false)
	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	requireTaskCanceled(t, first)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
}

func TestWorker_CleanupStopsRoutingAndCancelsTasks(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	latest := acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

// A task can exit without a newer snapshot following it, and a direct message
// edit leaves the snapshot version unchanged. Neither publishes a
// notification, so the manager sync must restore the work.
func TestRunner_SyncRestoresRequiredWork(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		releaseTask bool
		mutate      func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat
	}{
		{
			name:        "UnchangedRow",
			releaseTask: true,
			mutate: func(_ *testing.T, _ *workerTestFixture, chat database.Chat) database.Chat {
				return chat
			},
		},
		{
			name:        "NewerSnapshotSameWork",
			releaseTask: true,
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return forceExecutionState(t, f, chat.ID, database.ChatStatusRunning, false)
			},
		},
		{
			name:        "EqualSnapshotHistoryChange",
			releaseTask: true,
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return editUserMessage(t, f, chat.ID, "edited")
			},
		},
		{
			name: "EqualSnapshotHistoryChangeWhileActive",
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return editUserMessage(t, f, chat.ID, "edited")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			starter := newBlockingTaskStarter(false)
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			opts.RunnerSyncInterval = time.Minute
			startWorker(t, opts)
			first := starter.waitCall(t, taskKindGeneration, chat.ID)
			if tc.releaseTask {
				starter.release(t, 0)
			}
			expected := tc.mutate(t, f, chat)
			starter.assertNoCall(t)

			// Each advance delivers one sync. A released task may still be
			// exiting when the first sync arrives; the next one restores it.
			var second taskCall
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				clock.Advance(time.Minute).MustWait(ctx)
				select {
				case second = <-starter.callCh:
					return true
				default:
					return false
				}
			}, testutil.IntervalFast)
			if !tc.releaseTask {
				requireTaskCanceled(t, first)
			}
			require.Equal(t, taskKindGeneration, second.kind)
			require.Equal(t, expected.HistoryVersion, second.input.HistoryVersion)
			require.Equal(t, first.input.RunnerID, second.input.RunnerID)
			require.Same(t, first.input.SessionStart, second.input.SessionStart)
		})
	}
}

// Real generation rejects a direct message edit on its history fence; the
// runner must then continue from the edited history without a notification.
func TestRunner_RealGenerationRecoversHistoryFence(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"BeforeGeneration", "InFlightResponse"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			firstRequest := make(chan struct{})
			releaseResponse := make(chan struct{})
			recoveredRequest := make(chan string, 2)
			var requests atomic.Int32
			providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("history fence")
				}
				if requests.Add(1) == 1 && phase == "InFlightResponse" {
					close(firstRequest)
					select {
					case <-releaseResponse:
					case <-req.Context().Done():
					case <-ctx.Done():
					}
					return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("stale response must not commit")...)
				}
				select {
				case recoveredRequest <- string(req.RawBody):
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered response")...)
			})
			provider := dbgen.ChatProvider(t, f.db, database.ChatProvider{
				Provider: "openai-compat", DisplayName: "history fence", BaseUrl: providerURL,
			})
			model := dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{
				AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, OrganizationID: f.org.ID,
			})
			var transport atomic.Pointer[aibridge.TransportFactory]
			var factory aibridge.TransportFactory = chattest.NewMockAIBridgeTransport(t, providerURL)
			transport.Store(&factory)
			server := New(f.pubsub, Config{
				Logger: testutil.Logger(t), Database: f.db, ReplicaID: uuid.New(),
				Experiments: codersdk.ExperimentsKnown, AIBridgeTransportFactory: &transport,
			})
			t.Cleanup(func() { require.NoError(t, server.Close()) })
			created, err := server.CreateChat(ctx, CreateOptions{
				OrganizationID: f.org.ID, OwnerID: f.user.ID, ModelConfigID: model.ID,
				Title: "history fence", InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
			})
			require.NoError(t, err)
			// Close clears chatWorker.manager under its mutex while tasks may
			// still be draining, so read it the way Wake and WaitIdle do.
			manager := func() *runnerManager {
				server.chatWorker.mu.Lock()
				defer server.chatWorker.mu.Unlock()
				return server.chatWorker.manager
			}
			starter, err := newTaskStarter(server, server.chatWorker.opts,
				func(ctx context.Context, state runnerStateUpdate) {
					if m := manager(); m != nil {
						m.RouteStateHint(ctx, state)
					}
				},
				func(ctx context.Context, key runnerKey) {
					if m := manager(); m != nil {
						m.requestCleanup(ctx, key)
					}
				},
			)
			require.NoError(t, err)
			results := make(chan error, 16)
			generationStarted := make(chan chatWorkerTaskStartInput, 1)
			releaseGeneration := make(chan struct{})
			var invocations atomic.Int32
			server.chatWorker.opts.TaskStarter = &generationResultTaskStarter{
				chatWorkerTaskStarter: starter,
				results:               results,
				beforeGeneration: func(ctx context.Context, input chatWorkerTaskStartInput) {
					if phase != "BeforeGeneration" || invocations.Add(1) != 1 {
						return
					}
					generationStarted <- input
					select {
					case <-releaseGeneration:
					case <-ctx.Done():
					}
				},
			}
			// The message edit publishes nothing, so recovery comes from the
			// manager sync. It runs on the real clock here; shorten its interval.
			server.chatWorker.opts.RunnerSyncInterval = testutil.IntervalMedium
			server.Start()
			if phase == "BeforeGeneration" {
				input := testutil.RequireReceive(ctx, t, generationStarted)
				require.Equal(t, created.ID, input.ChatID)
			} else {
				testutil.TryReceive(ctx, t, firstRequest)
			}
			before, err := f.db.GetChatByID(ctx, created.ID)
			require.NoError(t, err)
			require.Greater(t, before.SnapshotVersion, before.HistoryVersion)
			mutated := editUserMessage(t, f, created.ID, "hello after out-of-band edit")
			require.Zero(t, mutated.GenerationAttempt)
			if phase == "BeforeGeneration" {
				close(releaseGeneration)
			} else {
				close(releaseResponse)
			}
			fenceExit := testutil.RequireReceive(ctx, t, results)
			require.ErrorIs(t, fenceExit, errTaskExpectedExit)
			require.NotErrorIs(t, fenceExit, errTaskRetryable)
			require.ErrorContains(t, fenceExit, "chat history version mismatch")
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				chat, err := f.db.GetChatByID(ctx, created.ID)
				return err == nil && chat.Status == database.ChatStatusWaiting && !chat.WorkerID.Valid && !chat.RunnerID.Valid
			}, testutil.IntervalFast)
			messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.ID})
			require.NoError(t, err)
			var users, assistants int
			for _, message := range messages {
				switch message.Role {
				case database.ChatMessageRoleUser:
					users++
					require.Contains(t, string(message.Content.RawMessage), "hello after out-of-band edit")
				case database.ChatMessageRoleAssistant:
					assistants++
					require.Contains(t, string(message.Content.RawMessage), "recovered response")
				}
				require.NotContains(t, string(message.Content.RawMessage), "stale response must not commit")
			}
			require.Equal(t, 1, users)
			require.Equal(t, 1, assistants)
			wantRequests := int32(1)
			if phase == "InFlightResponse" {
				wantRequests = 2
			}
			require.Equal(t, wantRequests, requests.Load())
			require.Contains(t, testutil.RequireReceive(ctx, t, recoveredRequest), "hello after out-of-band edit")
			final, err := f.db.GetChatByID(ctx, created.ID)
			require.NoError(t, err)
			require.False(t, final.LastError.Valid)
		})
	}
}
