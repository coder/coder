package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
)

func TestRunnerRunConversation(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	noopMarkTurnStartReady := func() {}

	t.Run("OneTurnHappyPath", func(t *testing.T) {
		t.Parallel()

		runner := newTestRunner(t, newRunConfig(t))
		events := make(chan codersdk.ChatStreamEvent, 3)
		events <- statusEvent(chatID, codersdk.ChatStatusRunning)
		events <- messagePartEvent(chatID)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
		close(events)

		result, err := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, noopMarkTurnStartReady)
		require.NoError(t, err)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
		require.Empty(t, result.failureStage)
		require.True(t, result.sawFirstOutput)
		require.Equal(t, 1, result.turnsCompleted)
		require.Equal(t, 3, result.eventCount)
	})

	t.Run("DuplicateWaitingDoesNotAdvanceTurn", func(t *testing.T) {
		t.Parallel()

		cfg := newRunConfig(t)
		cfg.Turns = 2

		events := make(chan codersdk.ChatStreamEvent, 7)
		events <- statusEvent(chatID, codersdk.ChatStatusRunning)
		events <- messagePartEvent(chatID)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		var sendCount atomic.Int64
		runner := newTestRunnerWithChatMessage(t, cfg, chatID, func() {
			sendCount.Add(1)
			events <- statusEvent(chatID, codersdk.ChatStatusRunning)
			events <- messagePartEvent(chatID)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			close(events)
		})

		result, err := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, noopMarkTurnStartReady)
		require.NoError(t, err)
		require.Equal(t, int64(1), sendCount.Load())
		require.Equal(t, 2, result.turnsCompleted)
		require.Equal(t, 7, result.eventCount)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})

	t.Run("StaleWaitingAfterNextTurnRunningDoesNotAdvanceTurn", func(t *testing.T) {
		t.Parallel()

		cfg := newRunConfig(t)
		cfg.Turns = 2

		events := make(chan codersdk.ChatStreamEvent, 7)
		events <- statusEvent(chatID, codersdk.ChatStatusRunning)
		events <- messagePartEvent(chatID)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		var sendCount atomic.Int64
		runner := newTestRunnerWithChatMessage(t, cfg, chatID, func() {
			sendCount.Add(1)
			events <- statusEvent(chatID, codersdk.ChatStatusRunning)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			events <- messagePartEvent(chatID)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			close(events)
		})

		result, err := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, noopMarkTurnStartReady)
		require.NoError(t, err)
		require.Equal(t, int64(1), sendCount.Load())
		require.Equal(t, 2, result.turnsCompleted)
		require.Equal(t, 7, result.eventCount)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})

	t.Run("FirstTurnGatesFollowUpStorm", func(t *testing.T) {
		t.Parallel()

		// Reproduces the contract that the turn-start gate is checked
		// after the first turn finishes, not before it begins. The runner
		// must mark itself ready, wait for the release channel, and only
		// then send turn 2.
		cfg := newRunConfig(t)
		cfg.Turns = 2
		readyWG := &sync.WaitGroup{}
		readyWG.Add(1)
		releaseChan := make(chan struct{})
		cfg.TurnStartReadyWaitGroup = readyWG
		cfg.StartTurnsChan = releaseChan

		events := make(chan codersdk.ChatStreamEvent, 4)
		events <- statusEvent(chatID, codersdk.ChatStatusRunning)
		events <- messagePartEvent(chatID)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		ready := make(chan struct{})
		go func() {
			readyWG.Wait()
			close(ready)
		}()

		errCh := make(chan error, 1)
		var sendCount atomic.Int64
		runner := newTestRunnerWithChatMessage(t, cfg, chatID, func() {
			sendCount.Add(1)
			events <- statusEvent(chatID, codersdk.ChatStatusRunning)
			events <- messagePartEvent(chatID)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			close(events)
		})

		go func() {
			_, runErr := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, sync.OnceFunc(readyWG.Done))
			errCh <- runErr
		}()

		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not mark turn-start gate ready after first turn")
		}

		require.Equal(t, int64(0), sendCount.Load(), "next turn was sent before turn-start release")

		close(releaseChan)

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not finish after turn-start release")
		}
		require.Equal(t, int64(1), sendCount.Load())
	})

	t.Run("FirstOutputFromAssistantMessageEvent", func(t *testing.T) {
		t.Parallel()

		// Snapshot race: when a turn finishes before stream attach,
		// StreamChat replays rows as message events, never as
		// message_part deltas; the assistant row must record first output.
		runner := newTestRunner(t, newRunConfig(t))
		events := make(chan codersdk.ChatStreamEvent, 3)
		events <- messageEvent(chatID, codersdk.ChatMessageRoleUser)
		events <- messageEvent(chatID, codersdk.ChatMessageRoleAssistant)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
		close(events)

		result, err := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, noopMarkTurnStartReady)
		require.NoError(t, err)
		require.True(t, result.sawFirstOutput, "first output not recorded from assistant message event")
		require.Equal(t, 1, result.turnsCompleted)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})

	t.Run("ImmediateWaitingCountsNextTurn", func(t *testing.T) {
		t.Parallel()

		cfg := newRunConfig(t)
		cfg.Turns = 2

		events := make(chan codersdk.ChatStreamEvent, 3)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		var sendCount atomic.Int64
		runner := newTestRunnerWithChatMessage(t, cfg, chatID, func() {
			sendCount.Add(1)
			events <- statusEvent(chatID, codersdk.ChatStatusRunning)
			events <- messagePartEvent(chatID)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			close(events)
		})

		result, err := runner.runConversation(context.Background(), chatID, testLogger(), time.Now(), events, noopMarkTurnStartReady)
		require.NoError(t, err)
		require.Equal(t, int64(1), sendCount.Load())
		require.Equal(t, 2, result.turnsCompleted)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})
}

func TestRunnerCleanup(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	t.Run("ArchivesChat", func(t *testing.T) {
		t.Parallel()

		client, archived := newTestClientWithChatArchive(t, chatID, http.StatusNoContent)
		runner := NewRunner(client, Config{})
		runner.chatID = chatID

		logs := bytes.NewBuffer(nil)
		err := runner.Cleanup(context.Background(), "runner-1", logs)
		require.NoError(t, err)
		require.True(t, archived())
		require.Contains(t, logs.String(), "archived chat")
	})

	t.Run("ArchiveErrorIsReturned", func(t *testing.T) {
		t.Parallel()

		client, archived := newTestClientWithChatArchive(t, chatID, http.StatusInternalServerError)
		runner := NewRunner(client, Config{})
		runner.chatID = chatID

		err := runner.Cleanup(context.Background(), "runner-1", bytes.NewBuffer(nil))
		require.Error(t, err)
		require.ErrorContains(t, err, "archive chat")
		require.True(t, archived())
	})
}

func testLogger() slog.Logger {
	return slog.Make(sloghuman.Sink(io.Discard)).Leveled(slog.LevelDebug)
}

func newRunConfig(t *testing.T) Config {
	t.Helper()
	reg := prometheus.NewRegistry()
	return Config{
		OrganizationID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		WorkspaceID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ModelConfigID:  uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Prompt:         "Reply with one short sentence.",
		Turns:          1,
		Metrics:        NewMetrics(reg),
	}
}

func newTestRunner(t *testing.T, cfg Config) *Runner {
	t.Helper()
	return NewRunner(newTestClient(t), cfg)
}

func newTestClientWithChatArchive(t *testing.T, chatID uuid.UUID, responseStatus int) (*codersdk.Client, func() bool) {
	t.Helper()

	var archived atomic.Bool
	expectedPath := "/api/experimental/chats/" + chatID.String()
	client := newTestClientWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("unexpected method for chat archive: %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected chat archive path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		var req codersdk.UpdateChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode chat archive request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Archived == nil || !*req.Archived {
			t.Errorf("unexpected archived value: %v", req.Archived)
			http.Error(w, "unexpected archived value", http.StatusBadRequest)
			return
		}
		archived.Store(true)
		if responseStatus == http.StatusNoContent {
			w.WriteHeader(responseStatus)
			return
		}
		http.Error(w, "boom", responseStatus)
	})
	return client, archived.Load
}

func newTestRunnerWithChatMessage(t *testing.T, cfg Config, chatID uuid.UUID, onMessage func()) *Runner {
	t.Helper()

	expectedPath := "/api/experimental/chats/" + chatID.String() + "/messages"
	return NewRunner(newTestClientWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method for chat message: %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected chat message path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		var req codersdk.CreateChatMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode chat message request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(req.Content) != 1 || req.Content[0].Type != codersdk.ChatInputPartTypeText || req.Content[0].Text != cfg.Prompt {
			t.Errorf("unexpected chat message content: %#v", req.Content)
			http.Error(w, "unexpected content", http.StatusBadRequest)
			return
		}
		if req.ModelConfigID == nil || *req.ModelConfigID != cfg.ModelConfigID {
			t.Errorf("unexpected chat message model config ID: %v", req.ModelConfigID)
			http.Error(w, "unexpected model config ID", http.StatusBadRequest)
			return
		}

		if onMessage != nil {
			onMessage()
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(codersdk.CreateChatMessageResponse{Queued: true}); err != nil {
			t.Errorf("encode chat message response: %v", err)
		}
	}), cfg)
}

func newTestClient(t *testing.T) *codersdk.Client {
	t.Helper()
	return newTestClientWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	})
}

func newTestClientWithHandler(t *testing.T, handler http.HandlerFunc) *codersdk.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return codersdk.New(serverURL)
}

func statusEvent(chatID uuid.UUID, status codersdk.ChatStatus) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		ChatID: chatID,
		Status: &codersdk.ChatStreamStatus{Status: status},
	}
}

func messagePartEvent(chatID uuid.UUID) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeMessagePart,
		ChatID: chatID,
	}
}

func messageEvent(chatID uuid.UUID, role codersdk.ChatMessageRole) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		ChatID:  chatID,
		Message: &codersdk.ChatMessage{Role: role},
	}
}
