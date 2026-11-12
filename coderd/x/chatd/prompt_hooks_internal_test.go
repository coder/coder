package chatd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

const (
	hookAllow    = `{}`
	hookRewrite  = `{"permission":{"decision":"allow","input_override":{"prompt":"rewritten"}},"model_context":"context","user_message":"notice"}`
	hookDeny     = `{"permission":{"decision":"deny"},"user_message":"blocked by policy"}`
	hookDenyBare = `{"permission":{"decision":"deny"}}`
)

// promptHooksHarness holds a running chat that is owned by one worker, with
// a history of one plain message and one message that waits for its hooks.
type promptHooksHarness struct {
	f        *workerTestFixture
	chat     database.Chat
	flagged  database.ChatMessage
	starter  *taskStarter
	recorder *taskSideEffectRecorder
	workerID uuid.UUID
	runnerID uuid.UUID

	mu       sync.Mutex
	requests []agenthooks.Request
	gate     chan struct{}
	entered  chan struct{}
}

// newPromptHooksHarness starts a hook consumer that answers request n (from 1)
// with status and body. A nil reply leaves hooks off.
func newPromptHooksHarness(t *testing.T, reply func(n int) (status int, body string)) *promptHooksHarness {
	t.Helper()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	flagged := f.insertFlaggedMessage(t, chat.ID, "flagged")
	h := &promptHooksHarness{
		f:        f,
		flagged:  flagged,
		recorder: newTaskSideEffectRecorder(),
		workerID: uuid.New(),
		runnerID: uuid.New(),
	}
	h.chat = acquireChat(t, f, chat.ID, h.workerID, h.runnerID)
	h.starter = newTestTaskStarter(t, f, h.recorder)
	if reply == nil {
		return h
	}
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		h.requests = append(h.requests, request)
		n := len(h.requests)
		gate, entered := h.gate, h.entered
		h.gate, h.entered = nil, nil
		h.mu.Unlock()
		if gate != nil {
			entered <- struct{}{}
			select {
			case <-gate:
			case <-r.Context().Done():
			}
		}
		status, body := reply(n)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(consumer.Close)
	h.starter.server.hooks = chathooks.NewTrigger(dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	))
	return h
}

func (f *workerTestFixture) insertFlaggedMessage(t *testing.T, chatID uuid.UUID, text string) database.ChatMessage {
	t.Helper()
	return dbgen.ChatMessage(t, f.db, database.ChatMessage{
		ChatID:             chatID,
		CreatedBy:          uuid.NullUUID{UUID: f.user.ID, Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: f.model.ID, Valid: true},
		Role:               database.ChatMessageRoleUser,
		Content:            userTextMessage(t, text, f.user.ID, f.model.ID).Content,
		PromptHooksPending: true,
	})
}

func (h *promptHooksHarness) requestCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.requests)
}

func (h *promptHooksHarness) promptOf(t *testing.T, n int) string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Less(t, n, len(h.requests))
	require.Equal(t, agenthooks.EventUserPromptSubmit, h.requests[n].Type)
	return decodeHookDataInternal[agenthooks.UserPromptSubmitData](t, h.requests[n]).Prompt
}

func decodeHookDataInternal[T any](t *testing.T, request agenthooks.Request) T {
	t.Helper()
	var data T
	require.NoError(t, json.Unmarshal(request.Data, &data))
	return data
}

// run starts a generation task as the given owner.
func (h *promptHooksHarness) run(ctx context.Context, t *testing.T, workerID, runnerID uuid.UUID) error {
	t.Helper()
	chat, err := h.f.db.GetChatByID(context.WithoutCancel(ctx), h.chat.ID)
	require.NoError(t, err)
	debugTurn := newRunnerDebugTurn(h.starter.opts.Logger)
	defer debugTurn.Finalize(context.WithoutCancel(ctx))
	return h.starter.StartGeneration(ctx, chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            database.ChatStatusRunning,
		DebugTurn:         debugTurn,
		SessionStart:      &sessionStartTracker{},
	})
}

// runDuring starts a generation task, holds the first hook request while
// during runs on the test goroutine, and returns the task's result.
func (h *promptHooksHarness) runDuring(ctx context.Context, t *testing.T, workerID, runnerID uuid.UUID, during func()) error {
	t.Helper()
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	h.mu.Lock()
	h.gate, h.entered = gate, entered
	h.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx, t, workerID, runnerID) }()
	testutil.TryReceive(testutil.Context(t, testutil.WaitLong), t, entered)
	during()
	close(gate)
	return testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, done)
}

type promptHooksRow struct {
	deleted bool
	pending bool
	parts   []codersdk.ChatMessagePart
}

func plain(text string) []codersdk.ChatMessagePart {
	return []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
}

func (h *promptHooksHarness) row(t *testing.T, id int64) promptHooksRow {
	t.Helper()
	message, err := h.f.db.GetChatMessageByIDForStream(testutil.Context(t, testutil.WaitShort), id)
	require.NoError(t, err)
	// A message with unparsable content has no parts.
	parts, _ := chatprompt.ParseContent(message)
	return promptHooksRow{deleted: message.Deleted, pending: message.PromptHooksPending, parts: parts}
}

func (h *promptHooksHarness) chatNow(t *testing.T) database.Chat {
	t.Helper()
	chat, err := h.f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), h.chat.ID)
	require.NoError(t, err)
	return chat
}

func (h *promptHooksHarness) chatError(t *testing.T) codersdk.ChatError {
	t.Helper()
	chat := h.chatNow(t)
	require.True(t, chat.LastError.Valid)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(chat.LastError.RawMessage, &chatErr))
	return chatErr
}

func (h *promptHooksHarness) machine() *chatstate.ChatMachine {
	return chatstate.NewChatMachine(h.f.db, h.f.pubsub, h.chat.ID)
}

// unparsableContent leaves the flagged message with content that is not a
// list of message parts.
func unparsableContent(t *testing.T, h *promptHooksHarness) {
	t.Helper()
	_, err := h.f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort),
		`UPDATE chat_messages SET content = '"not parts"' WHERE id = $1`, h.flagged.ID)
	require.NoError(t, err)
}

func TestResolvePromptHooksOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		reply        func(n int) (int, string)
		prepare      func(t *testing.T, h *promptHooksHarness)
		wantRequests int
		wantRow      promptHooksRow
		wantStatus   database.ChatStatus
		wantKind     codersdk.ChatErrorKind
		wantErr      string
	}{
		{
			name:         "Allow",
			reply:        func(int) (int, string) { return http.StatusOK, hookAllow },
			wantRequests: 1,
			wantRow:      promptHooksRow{parts: plain("flagged")},
			wantStatus:   database.ChatStatusRunning,
		},
		{
			name:         "Rewrite",
			reply:        func(int) (int, string) { return http.StatusOK, hookRewrite },
			wantRequests: 1,
			wantRow:      promptHooksRow{parts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("rewritten"), {Type: codersdk.ChatMessagePartTypeHookContext, Text: "context"}, {Type: codersdk.ChatMessagePartTypeHookNotice, Text: "notice"}}},
			wantStatus:   database.ChatStatusRunning,
		},
		{
			name:         "Deny",
			reply:        func(int) (int, string) { return http.StatusOK, hookDeny },
			wantRequests: 1,
			wantRow:      promptHooksRow{deleted: true, parts: plain("flagged")},
			wantStatus:   database.ChatStatusError,
			wantKind:     codersdk.ChatErrorKindHookDenied,
			wantErr:      "blocked by policy",
		},
		{
			name:         "DenyWithoutUserMessage",
			reply:        func(int) (int, string) { return http.StatusOK, hookDenyBare },
			wantRequests: 1,
			wantRow:      promptHooksRow{deleted: true, parts: plain("flagged")},
			wantStatus:   database.ChatStatusError,
			wantKind:     codersdk.ChatErrorKindHookDenied,
			wantErr:      promptHooksDeniedFallback,
		},
		{
			// The consumer made no decision, so the message stays and waits.
			name:         "DispatchFailure",
			reply:        func(int) (int, string) { return http.StatusInternalServerError, "" },
			wantRequests: 1,
			wantRow:      promptHooksRow{pending: true, parts: plain("flagged")},
			wantStatus:   database.ChatStatusError,
			wantKind:     codersdk.ChatErrorKindHookDispatchFailed,
			wantErr:      "hook dispatch failed: user_prompt_submit: http_error",
		},
		{
			// An error that is not a dispatch error must fail the turn, not
			// retry forever with the chat still running.
			name:       "UnparsableContent",
			reply:      func(int) (int, string) { return http.StatusOK, hookAllow },
			prepare:    unparsableContent,
			wantRow:    promptHooksRow{pending: true},
			wantStatus: database.ChatStatusError,
			wantErr:    "failed unexpectedly",
		},
		{
			name:       "HooksOff",
			reply:      nil,
			wantRow:    promptHooksRow{parts: plain("flagged")},
			wantStatus: database.ChatStatusRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			h := newPromptHooksHarness(t, tt.reply)
			if tt.prepare != nil {
				tt.prepare(t, h)
			}

			require.NoError(t, h.run(ctx, t, h.workerID, h.runnerID))

			require.Equal(t, tt.wantRequests, h.requestCount())
			if tt.wantRequests > 0 {
				require.Equal(t, "flagged", h.promptOf(t, 0))
			}
			require.Equal(t, tt.wantRow, h.row(t, h.flagged.ID))
			require.Equal(t, tt.wantStatus, h.chatNow(t).Status)
			if tt.wantStatus == database.ChatStatusError {
				chatErr := h.chatError(t)
				require.Contains(t, chatErr.Message, tt.wantErr)
				if tt.wantKind != "" {
					require.Equal(t, tt.wantKind, chatErr.Kind)
				}
			} else {
				// A settled message starts a new task through a state hint.
				h.recorder.requireStateHintCount(t, 1)
			}
		})
	}
}

// TestResolvePromptHooksRunsAgainAfterACrash covers a worker that dies after
// the hook answered and before the transaction committed: the flag is still
// set, so the next task runs the hooks again and applies one result.
func TestResolvePromptHooksRunsAgainAfterACrash(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	h := newPromptHooksHarness(t, func(n int) (int, string) {
		return http.StatusOK, fmt.Sprintf(`{"permission":{"decision":"allow","input_override":{"prompt":"answer %d"}}}`, n)
	})

	crashCtx, crash := context.WithCancel(ctx)
	err := h.runDuring(crashCtx, t, h.workerID, h.runnerID, crash)
	require.ErrorIs(t, err, errTaskExpectedExit)
	require.Equal(t, promptHooksRow{pending: true, parts: plain("flagged")}, h.row(t, h.flagged.ID))

	require.NoError(t, h.run(ctx, t, h.workerID, h.runnerID))
	require.Equal(t, 2, h.requestCount())
	require.Equal(t, promptHooksRow{parts: plain("answer 2")}, h.row(t, h.flagged.ID))
}

// TestResolvePromptHooksSecondReplica covers a worker whose lease ended while
// its hook ran. The second worker owns the chat, so the first worker's late
// result is dropped and the second worker's result is the one that applies.
func TestResolvePromptHooksSecondReplica(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	h := newPromptHooksHarness(t, func(n int) (int, string) {
		prompt := "second"
		if n == 1 {
			prompt = "late"
		}
		return http.StatusOK, `{"permission":{"decision":"allow","input_override":{"prompt":"` + prompt + `"}}}`
	})

	secondWorker, secondRunner := uuid.New(), uuid.New()
	err := h.runDuring(ctx, t, h.workerID, h.runnerID, func() {
		require.NoError(t, h.machine().Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.Abandon(chatstate.AbandonInput{})
			return err
		}))
		acquireChat(t, h.f, h.chat.ID, secondWorker, secondRunner)
	})
	require.ErrorIs(t, err, errTaskExpectedExit)
	require.Equal(t, promptHooksRow{pending: true, parts: plain("flagged")}, h.row(t, h.flagged.ID))

	require.NoError(t, h.run(ctx, t, secondWorker, secondRunner))
	require.Equal(t, 2, h.requestCount())
	require.Equal(t, promptHooksRow{parts: plain("second")}, h.row(t, h.flagged.ID))
}

// TestResolvePromptHooksConcurrentActors runs one action while the hook of the
// first task is in flight and checks the outcome of each order.
func TestResolvePromptHooksConcurrentActors(t *testing.T) {
	t.Parallel()

	human := func(behavior chatstate.BusyBehavior) func(t *testing.T, h *promptHooksHarness) {
		return func(t *testing.T, h *promptHooksHarness) {
			require.NoError(t, h.machine().Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage(t, "human", h.f.user.ID, h.f.model.ID),
					BusyBehavior: behavior,
					MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
				})
				return err
			}))
		}
	}
	tests := []struct {
		name string
		do   func(t *testing.T, h *promptHooksHarness)
		// wantErrExit is true when the first task's result is dropped.
		wantErrExit bool
		wantRow     promptHooksRow
		wantStatus  database.ChatStatus
		wantQueued  int
	}{
		{
			name:       "HumanMessage",
			do:         human(chatstate.BusyBehaviorQueue),
			wantRow:    promptHooksRow{parts: plain("flagged")},
			wantStatus: database.ChatStatusRunning,
			wantQueued: 1,
		},
		{
			// The edit deletes the flagged message with the rest of the
			// suffix, so no hook result applies to it.
			name: "EditOfEarlierMessage",
			do: func(t *testing.T, h *promptHooksHarness) {
				history, err := h.f.db.GetChatMessagesByChatID(testutil.Context(t, testutil.WaitShort), database.GetChatMessagesByChatIDParams{ChatID: h.chat.ID})
				require.NoError(t, err)
				first := history[0]
				require.NoError(t, h.machine().Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.EditMessage(chatstate.EditMessageInput{
						MessageID: first.ID,
						CreatedBy: h.f.user.ID,
						Content:   userTextMessage(t, "edited", h.f.user.ID, h.f.model.ID).Content,
					})
					return err
				}))
			},
			wantErrExit: true,
			wantRow:     promptHooksRow{deleted: true, parts: plain("flagged")},
			wantStatus:  database.ChatStatusRunning,
		},
		{
			name: "Interrupt",
			do: func(t *testing.T, h *promptHooksHarness) {
				require.NoError(t, h.machine().Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
					return err
				}))
			},
			wantErrExit: true,
			wantRow:     promptHooksRow{pending: true, parts: plain("flagged")},
			wantStatus:  database.ChatStatusInterrupting,
		},
		{
			name: "ChatRemoved",
			do: func(t *testing.T, h *promptHooksHarness) {
				_, err := h.f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), `DELETE FROM chats WHERE id = $1`, h.chat.ID)
				require.NoError(t, err)
			},
			wantErrExit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			h := newPromptHooksHarness(t, func(int) (int, string) { return http.StatusOK, hookAllow })

			err := h.runDuring(ctx, t, h.workerID, h.runnerID, func() { tt.do(t, h) })

			if tt.wantErrExit {
				require.ErrorIs(t, err, errTaskExpectedExit)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, h.requestCount())
			if tt.name == "ChatRemoved" {
				return
			}
			require.Equal(t, tt.wantRow, h.row(t, h.flagged.ID))
			require.Equal(t, tt.wantStatus, h.chatNow(t).Status)
			queued, err := h.f.db.GetChatQueuedMessages(ctx, h.chat.ID)
			require.NoError(t, err)
			require.Len(t, queued, tt.wantQueued)
		})
	}
}

func TestFirstPromptHooksPending(t *testing.T) {
	t.Parallel()

	row := func(id int64, pending bool) database.ChatMessage {
		return database.ChatMessage{ID: id, PromptHooksPending: pending}
	}
	boundary, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolName:   "chat_cleared",
		ToolCallID: "call_clear",
	}})
	require.NoError(t, err)
	clear := database.ChatMessage{ID: 50, Compressed: true, Content: boundary, ContentVersion: chatprompt.CurrentContentVersion}

	tests := []struct {
		name     string
		messages []database.ChatMessage
		want     int64
	}{
		{name: "Oldest", messages: []database.ChatMessage{row(1, false), row(2, true), row(3, true)}, want: 2},
		{name: "HiddenByAClear", messages: []database.ChatMessage{row(1, true), clear, row(60, false)}},
		{name: "AfterAClear", messages: []database.ChatMessage{row(1, true), clear, row(60, true)}, want: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := firstPromptHooksPending(tt.messages)
			require.Equal(t, tt.want != 0, ok)
			require.Equal(t, tt.want, got.ID)
		})
	}
}

func TestMergeManualTitleMessagesSkipsPendingPromptHooks(t *testing.T) {
	t.Parallel()
	head := []database.ChatMessage{{ID: 1}, {ID: 2, PromptHooksPending: true}}
	tail := []database.ChatMessage{{ID: 3, PromptHooksPending: true}, {ID: 2, PromptHooksPending: true}}
	merged := mergeManualTitleMessages(head, tail)
	require.Len(t, merged, 1)
	require.EqualValues(t, 1, merged[0].ID)
}
