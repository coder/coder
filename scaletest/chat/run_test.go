//nolint:testpackage // Runner loop tests need access to unexported helpers.
package chat

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestRunnerRunConversation(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	t.Run("OneTurnHappyPath", func(t *testing.T) {
		t.Parallel()

		runner := newTestRunner(t, newRunConfig(t))
		events := make(chan codersdk.ChatStreamEvent, 3)
		events <- statusEvent(chatID, codersdk.ChatStatusRunning)
		events <- messagePartEvent(chatID)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
		close(events)

		result, err := runner.runConversation(context.Background(), chatID, io.Discard, time.Now(), events, func(ctx context.Context, nextTurn int, phase string) error {
			t.Fatalf("unexpected next-turn send for turn %d in phase %s", nextTurn, phase)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
		require.Empty(t, result.failureStage)
		require.True(t, result.sawFirstOutput)
		require.Equal(t, 1, result.turnsCompleted)
		require.Equal(t, 3, result.eventCount)
	})

	t.Run("FirstTurnGatesFollowUpStorm", func(t *testing.T) {
		t.Parallel()

		// Reproduces the contract that the turn-start gate is checked
		// after the first turn finishes, not before it begins. The runner
		// must mark itself ready, wait for the release channel, and only
		// then call sendNextTurn for turn 2.
		cfg := newRunConfig(t)
		cfg.Turns = 2
		readyWG := &sync.WaitGroup{}
		readyWG.Add(1)
		releaseChan := make(chan struct{})
		cfg.TurnStartReadyWaitGroup = readyWG
		cfg.StartTurnsChan = releaseChan
		runner := newTestRunner(t, cfg)

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
		var sendCount int
		go func() {
			_, runErr := runner.runConversation(context.Background(), chatID, io.Discard, time.Now(), events, func(_ context.Context, nextTurn int, phase string) error {
				sendCount++
				require.Equal(t, 2, nextTurn)
				require.Equal(t, phaseFollowUp, phase)
				events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
				close(events)
				return nil
			})
			errCh <- runErr
		}()

		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not mark turn-start gate ready after first turn")
		}

		require.Equal(t, 0, sendCount, "sendNextTurn fired before turn-start release")

		close(releaseChan)

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not finish after turn-start release")
		}
		require.Equal(t, 1, sendCount)
	})

	t.Run("FirstOutputFromAssistantMessageEvent", func(t *testing.T) {
		t.Parallel()

		// Covers the snapshot race where a turn finishes before the
		// runner attaches its stream: StreamChat replays the persisted
		// rows as message events, never as message_part deltas. The
		// runner must still record first-output for the assistant row,
		// and must not count the persisted user row as output.
		runner := newTestRunner(t, newRunConfig(t))
		events := make(chan codersdk.ChatStreamEvent, 3)
		events <- messageEvent(chatID, codersdk.ChatMessageRoleUser)
		events <- messageEvent(chatID, codersdk.ChatMessageRoleAssistant)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
		close(events)

		result, err := runner.runConversation(context.Background(), chatID, io.Discard, time.Now(), events, func(_ context.Context, nextTurn int, phase string) error {
			t.Fatalf("unexpected next-turn send for turn %d in phase %s", nextTurn, phase)
			return nil
		})
		require.NoError(t, err)
		require.True(t, result.sawFirstOutput, "first output not recorded from assistant message event")
		require.Equal(t, 1, result.turnsCompleted)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})

	t.Run("ImmediateWaitingCountsNextTurn", func(t *testing.T) {
		t.Parallel()

		cfg := newRunConfig(t)
		cfg.Turns = 2
		runner := newTestRunner(t, cfg)

		events := make(chan codersdk.ChatStreamEvent, 2)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		sendCount := 0
		result, err := runner.runConversation(context.Background(), chatID, io.Discard, time.Now(), events, func(ctx context.Context, nextTurn int, phase string) error {
			sendCount++
			require.Equal(t, 2, nextTurn)
			require.Equal(t, phaseFollowUp, phase)
			events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
			close(events)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, sendCount)
		require.Equal(t, 2, result.turnsCompleted)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
	})
}

func TestRunnerCleanup(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	client := newTestClient(t)

	t.Run("ArchivesChat", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.chatID = chatID

		archived := false
		runner.archiveChat = func(ctx context.Context, gotChatID uuid.UUID) error {
			require.Equal(t, chatID, gotChatID)
			archived = true
			return nil
		}

		logs := bytes.NewBuffer(nil)
		err := runner.Cleanup(context.Background(), "runner-1", logs)
		require.NoError(t, err)
		require.True(t, archived)
		require.Contains(t, logs.String(), "archived chat")
	})

	t.Run("ArchiveErrorIsReturned", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.chatID = chatID
		archiveErr := xerrors.New("boom")
		runner.archiveChat = func(ctx context.Context, gotChatID uuid.UUID) error {
			require.Equal(t, chatID, gotChatID)
			return archiveErr
		}

		err := runner.Cleanup(context.Background(), "runner-1", bytes.NewBuffer(nil))
		require.Error(t, err)
		require.ErrorContains(t, err, "archive chat")
		require.ErrorIs(t, err, archiveErr)
	})
}

func newRunConfig(t *testing.T) Config {
	t.Helper()
	reg := prometheus.NewRegistry()
	return Config{
		RunID:             "run-123",
		OrganizationID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		WorkspaceID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Prompt:            "Reply with one short sentence.",
		Turns:             1,
		ReadyWaitGroup:    &sync.WaitGroup{},
		StartChan:         make(chan struct{}),
		Metrics:           NewMetrics(reg, MetricLabelNames()...),
		MetricLabelValues: MetricLabelValues("run-123"),
	}
}

func newTestRunner(t *testing.T, cfg Config) *Runner {
	t.Helper()
	return NewRunner(newTestClient(t), cfg)
}

func newTestClient(t *testing.T) *codersdk.Client {
	t.Helper()
	serverURL, err := url.Parse("http://example.com")
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
