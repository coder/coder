package chat

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

		err := runTestConversation(t, runner, chatID, events, noopMarkTurnStartReady)
		require.NoError(t, err)
		result := runner.result
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

		err := runTestConversation(t, runner, chatID, events, noopMarkTurnStartReady)
		require.NoError(t, err)
		result := runner.result
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

		err := runTestConversation(t, runner, chatID, events, noopMarkTurnStartReady)
		require.NoError(t, err)
		result := runner.result
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

		runner.resetConversation(time.Now(), sync.OnceFunc(readyWG.Done))

		go func() {
			runErr := runner.runConversation(context.Background(), chatID, testLogger(), events)
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

		err := runTestConversation(t, runner, chatID, events, noopMarkTurnStartReady)
		require.NoError(t, err)
		result := runner.result
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

		err := runTestConversation(t, runner, chatID, events, noopMarkTurnStartReady)
		require.NoError(t, err)
		result := runner.result
		require.Equal(t, int64(1), sendCount.Load())
		require.Equal(t, 2, result.turnsCompleted)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})
}

func runTestConversation(t *testing.T, runner *Runner, chatID uuid.UUID, events <-chan codersdk.ChatStreamEvent, markTurnStartReady func()) error {
	t.Helper()
	runner.resetConversation(time.Now(), markTurnStartReady)
	return runner.runConversation(context.Background(), chatID, testLogger(), events)
}

func TestRunnerCleanup(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	t.Run("ArchivesChat", func(t *testing.T) {
		t.Parallel()

		runner, archived := newTestRunnerWithChatArchive(t, chatID, nil)

		logs := bytes.NewBuffer(nil)
		err := runner.Cleanup(context.Background(), "runner-1", logs)
		require.NoError(t, err)
		require.True(t, archived())
		require.Contains(t, logs.String(), "archived chat")
	})

	t.Run("ArchiveErrorIsReturned", func(t *testing.T) {
		t.Parallel()

		runner, archived := newTestRunnerWithChatArchive(t, chatID, xerrors.New("boom"))

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

type fakeChatClient struct {
	createChatFunc        func(context.Context, codersdk.CreateChatRequest) (codersdk.Chat, error)
	streamChatFunc        func(context.Context, uuid.UUID, *codersdk.StreamChatOptions) (<-chan codersdk.ChatStreamEvent, io.Closer, error)
	createChatMessageFunc func(context.Context, uuid.UUID, codersdk.CreateChatMessageRequest) (codersdk.CreateChatMessageResponse, error)
	updateChatFunc        func(context.Context, uuid.UUID, codersdk.UpdateChatRequest) error
}

func newFakeChatClient(t *testing.T) *fakeChatClient {
	t.Helper()
	return &fakeChatClient{}
}

func (*fakeChatClient) SetLogger(logger slog.Logger) {}

func (*fakeChatClient) SetLogBodies(logBodies bool) {}

func (f *fakeChatClient) CreateChat(ctx context.Context, req codersdk.CreateChatRequest) (codersdk.Chat, error) {
	if f.createChatFunc == nil {
		return codersdk.Chat{}, xerrors.New("unexpected CreateChat call")
	}
	return f.createChatFunc(ctx, req)
}

func (f *fakeChatClient) StreamChat(ctx context.Context, chatID uuid.UUID, opts *codersdk.StreamChatOptions) (<-chan codersdk.ChatStreamEvent, io.Closer, error) {
	if f.streamChatFunc == nil {
		return nil, nil, xerrors.New("unexpected StreamChat call")
	}
	return f.streamChatFunc(ctx, chatID, opts)
}

func (f *fakeChatClient) CreateChatMessage(ctx context.Context, chatID uuid.UUID, req codersdk.CreateChatMessageRequest) (codersdk.CreateChatMessageResponse, error) {
	if f.createChatMessageFunc == nil {
		return codersdk.CreateChatMessageResponse{}, xerrors.New("unexpected CreateChatMessage call")
	}
	return f.createChatMessageFunc(ctx, chatID, req)
}

func (f *fakeChatClient) UpdateChat(ctx context.Context, chatID uuid.UUID, req codersdk.UpdateChatRequest) error {
	if f.updateChatFunc == nil {
		return xerrors.New("unexpected UpdateChat call")
	}
	return f.updateChatFunc(ctx, chatID, req)
}

var _ chatClient = (*fakeChatClient)(nil)

func newTestRunner(t *testing.T, cfg Config) *Runner {
	t.Helper()
	return &Runner{client: newFakeChatClient(t), cfg: cfg}
}

func newTestRunnerWithChatArchive(t *testing.T, chatID uuid.UUID, updateErr error) (*Runner, func() bool) {
	t.Helper()

	var archived atomic.Bool
	client := newFakeChatClient(t)
	client.updateChatFunc = func(ctx context.Context, gotChatID uuid.UUID, req codersdk.UpdateChatRequest) error {
		if gotChatID != chatID {
			return xerrors.Errorf("unexpected chat archive ID: %s", gotChatID)
		}
		if req.Archived == nil || !*req.Archived {
			return xerrors.Errorf("unexpected archived value: %v", req.Archived)
		}
		archived.Store(true)
		return updateErr
	}
	runner := &Runner{client: client, cfg: Config{}, chatID: chatID}
	return runner, archived.Load
}

func newTestRunnerWithChatMessage(t *testing.T, cfg Config, chatID uuid.UUID, onMessage func()) *Runner {
	t.Helper()

	client := newFakeChatClient(t)
	client.createChatMessageFunc = func(ctx context.Context, gotChatID uuid.UUID, req codersdk.CreateChatMessageRequest) (codersdk.CreateChatMessageResponse, error) {
		if gotChatID != chatID {
			return codersdk.CreateChatMessageResponse{}, xerrors.Errorf("unexpected chat message ID: %s", gotChatID)
		}
		if err := validatePromptParts(req.Content, cfg.Prompt); err != nil {
			return codersdk.CreateChatMessageResponse{}, err
		}
		if req.ModelConfigID == nil || *req.ModelConfigID != cfg.ModelConfigID {
			return codersdk.CreateChatMessageResponse{}, xerrors.Errorf("unexpected chat message model config ID: %v", req.ModelConfigID)
		}

		if onMessage != nil {
			onMessage()
		}
		return codersdk.CreateChatMessageResponse{Queued: true}, nil
	}
	return &Runner{client: client, cfg: cfg}
}

func validatePromptParts(parts []codersdk.ChatInputPart, prompt string) error {
	if len(parts) != 1 || parts[0].Type != codersdk.ChatInputPartTypeText || parts[0].Text != prompt {
		return xerrors.Errorf("unexpected chat message content: %#v", parts)
	}
	return nil
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
