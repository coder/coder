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
			t.Fatalf("unexpected follow-up send for turn %d in phase %s", nextTurn, phase)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
		require.Empty(t, result.failureStage)
		require.True(t, result.sawFirstOutput)
		require.Equal(t, 1, result.turnsCompleted)
		require.Equal(t, 3, result.eventCount)
	})

	t.Run("ImmediateWaitingAfterFollowUpCountsNextTurn", func(t *testing.T) {
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

	t.Run("DelayedFollowUpWaitsForRelease", func(t *testing.T) {
		t.Parallel()

		cfg := newRunConfig(t)
		cfg.Turns = 2
		cfg.FollowUpStartDelay = time.Second
		cfg.FollowUpReadyWaitGroup = &sync.WaitGroup{}
		cfg.FollowUpReadyWaitGroup.Add(1)
		cfg.StartFollowUpChan = make(chan struct{})
		runner := newTestRunner(t, cfg)

		events := make(chan codersdk.ChatStreamEvent, 2)
		events <- statusEvent(chatID, codersdk.ChatStatusWaiting)

		sendCalled := make(chan struct{}, 1)
		resultCh := make(chan runnerResult, 1)
		errCh := make(chan error, 1)
		go func() {
			result, err := runner.runConversation(context.Background(), chatID, io.Discard, time.Now(), events, func(ctx context.Context, nextTurn int, phase string) error {
				if nextTurn != 2 || phase != phaseFollowUp {
					return xerrors.Errorf("unexpected follow-up turn=%d phase=%s", nextTurn, phase)
				}
				sendCalled <- struct{}{}
				events <- statusEvent(chatID, codersdk.ChatStatusWaiting)
				close(events)
				return nil
			})
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}()

		readyCh := make(chan struct{})
		go func() {
			cfg.FollowUpReadyWaitGroup.Wait()
			close(readyCh)
		}()

		select {
		case <-readyCh:
		case err := <-errCh:
			t.Fatalf("runner exited before follow-up gate released: %v", err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for follow-up gate readiness")
		}

		select {
		case <-sendCalled:
			t.Fatal("follow-up sent before gate release")
		default:
		}

		close(cfg.StartFollowUpChan)

		select {
		case <-sendCalled:
		case err := <-errCh:
			t.Fatalf("runner exited after gate release: %v", err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for follow-up send")
		}

		select {
		case result := <-resultCh:
			require.Equal(t, string(codersdk.ChatStatusWaiting), result.finalStatus)
			require.Equal(t, 2, result.turnsCompleted)
		case err := <-errCh:
			t.Fatalf("runner failed after gate release: %v", err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for runner completion")
		}
	})
}

func TestRunnerCleanup(t *testing.T) {
	t.Parallel()

	chatID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	client := newTestClient(t)

	t.Run("ArchivesChat", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.setChatID(chatID)

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
		runner.setChatID(chatID)
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
		WorkspaceID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Prompt:            "Reply with one short sentence.",
		Turns:             1,
		FollowUpPrompt:    "Continue.",
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
