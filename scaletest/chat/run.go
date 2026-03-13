package chat

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	// Set during Run, used in Cleanup and GetMetrics.
	mu             sync.Mutex
	chatID         uuid.UUID
	finalStatus    string
	totalDuration  time.Duration
	sawFirstOutput bool
	retryCount     int
	eventCount     int
	turnsCompleted int
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{client: client, cfg: cfg}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) (err error) {
	r.cfg.ReadyWaitGroup.Done()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.cfg.StartChan:
	}

	_, _ = fmt.Fprintf(logs, "starting chat runner %s for workspace %s\n", id, r.cfg.WorkspaceID)

	conversationStart := time.Now()
	turnStartTime := conversationStart
	var (
		sawRunning      bool
		sawFirstOutput  bool
		retryCount      int
		eventCount      int
		lastStreamError string
		turnsCompleted  int
		finalStatus     string
	)
	defer func() {
		r.storeResults(finalStatus, time.Since(conversationStart), sawFirstOutput, retryCount, eventCount, turnsCompleted)
	}()

	chat, err := r.client.CreateChat(ctx, codersdk.CreateChatRequest{
		WorkspaceID:   &r.cfg.WorkspaceID,
		ModelConfigID: r.cfg.ModelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
	})
	createDuration := time.Since(conversationStart)
	r.cfg.Metrics.ChatCreateLatencySeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(createDuration.Seconds())
	if err != nil {
		r.cfg.Metrics.ChatCreateErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		return xerrors.Errorf("create chat: %w", err)
	}

	r.setChatID(chat.ID)
	_, _ = fmt.Fprintf(logs, "created chat %s in %s\n", chat.ID, createDuration)

	events, closer, err := r.client.StreamChat(ctx, chat.ID, nil)
	if err != nil {
		r.cfg.Metrics.ChatStreamErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		return xerrors.Errorf("stream chat: %w", err)
	}

	r.cfg.Metrics.ActiveChatStreams.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
	defer func() {
		r.cfg.Metrics.ActiveChatStreams.WithLabelValues(r.cfg.MetricLabelValues...).Dec()
		_ = closer.Close()
	}()

	_, _ = fmt.Fprintf(logs, "streaming chat %s\n", chat.ID)

	for event := range events {
		eventCount++

		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				continue
			}

			switch event.Status.Status {
			case codersdk.ChatStatusRunning:
				if sawRunning {
					continue
				}
				sawRunning = true
				r.cfg.Metrics.ChatTimeToRunningSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(time.Since(turnStartTime).Seconds())
				_, _ = fmt.Fprintf(logs, "chat %s reached running status\n", chat.ID)
			case codersdk.ChatStatusWaiting:
				turnsCompleted++
				turnDuration := time.Since(turnStartTime)
				r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(turnDuration.Seconds())
				r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(r.terminalMetricLabelValues(string(codersdk.ChatStatusWaiting))...).Inc()
				r.cfg.Metrics.ChatTurnsCompletedTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
				_, _ = fmt.Fprintf(logs, "chat %s completed turn %d/%d in %s\n", chat.ID, turnsCompleted, r.cfg.Turns, turnDuration)
				if turnsCompleted >= r.cfg.Turns {
					finalStatus = string(codersdk.ChatStatusWaiting)
					conversationDuration := time.Since(conversationStart)
					_, _ = fmt.Fprintf(logs, "chat %s reached terminal status %q in %s after %d turns\n", chat.ID, codersdk.ChatStatusWaiting, conversationDuration, turnsCompleted)
					return nil
				}

				nextTurn := turnsCompleted + 1
				sawRunning = false
				turnStartTime = time.Now()
				_, err = r.client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
					Content: []codersdk.ChatInputPart{{
						Type: codersdk.ChatInputPartTypeText,
						Text: r.cfg.FollowUpPrompt,
					}},
					ModelConfigID: r.cfg.ModelConfigID,
				})
				if err != nil {
					return xerrors.Errorf("create chat message for turn %d: %w", nextTurn, err)
				}
				_, _ = fmt.Fprintf(logs, "chat %s sent follow-up message for turn %d/%d\n", chat.ID, nextTurn, r.cfg.Turns)
			case codersdk.ChatStatusError:
				finalStatus = string(codersdk.ChatStatusError)
				turnDuration := time.Since(turnStartTime)
				r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(turnDuration.Seconds())
				r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(r.terminalMetricLabelValues(string(codersdk.ChatStatusError))...).Inc()

				errMessage := lastStreamError
				if errMessage == "" {
					errMessage = "chat reached error status"
				}
				_, _ = fmt.Fprintf(logs, "chat %s reached terminal status %q after %d/%d turns: %s\n", chat.ID, codersdk.ChatStatusError, turnsCompleted, r.cfg.Turns, errMessage)
				return xerrors.Errorf("chat %s reached error status: %s", chat.ID, errMessage)
			}
		case codersdk.ChatStreamEventTypeMessagePart:
			if sawFirstOutput {
				continue
			}
			sawFirstOutput = true
			firstOutputDuration := time.Since(conversationStart)
			r.cfg.Metrics.ChatTimeToFirstOutputSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(firstOutputDuration.Seconds())
			_, _ = fmt.Fprintf(logs, "chat %s received first output in %s\n", chat.ID, firstOutputDuration)
		case codersdk.ChatStreamEventTypeRetry:
			retryCount++
			r.cfg.Metrics.ChatRetryEventsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
			if event.Retry != nil {
				_, _ = fmt.Fprintf(logs, "chat %s retry attempt %d in %dms: %s\n", chat.ID, event.Retry.Attempt, event.Retry.DelayMs, event.Retry.Error)
				continue
			}
			_, _ = fmt.Fprintf(logs, "chat %s received retry event\n", chat.ID)
		case codersdk.ChatStreamEventTypeError:
			r.cfg.Metrics.ChatStreamErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
			if event.Error != nil && event.Error.Message != "" {
				lastStreamError = event.Error.Message
				_, _ = fmt.Fprintf(logs, "chat %s stream error: %s\n", chat.ID, lastStreamError)
				continue
			}
			_, _ = fmt.Fprintf(logs, "chat %s received stream error event\n", chat.ID)
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if lastStreamError != "" {
		return xerrors.Errorf("chat %s stream ended before completing %d of %d turns: %s", chat.ID, turnsCompleted, r.cfg.Turns, lastStreamError)
	}
	return xerrors.Errorf("chat %s stream ended before completing %d of %d turns", chat.ID, turnsCompleted, r.cfg.Turns)
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	chatID := r.getChatID()
	if chatID == uuid.Nil {
		return nil
	}

	_, _ = fmt.Fprintf(logs, "archiving chat %s for runner %s\n", chatID, id)
	if err := r.client.ArchiveChat(ctx, chatID); err != nil {
		_, _ = fmt.Fprintf(logs, "failed to archive chat %s: %v\n", chatID, err)
		return xerrors.Errorf("archive chat: %w", err)
	}
	_, _ = fmt.Fprintf(logs, "archived chat %s\n", chatID)
	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	return map[string]any{
		"chat_id":          r.chatID.String(),
		"final_status":     r.finalStatus,
		"total_duration":   r.totalDuration.Seconds(),
		"saw_first_output": r.sawFirstOutput,
		"retry_count":      r.retryCount,
		"event_count":      r.eventCount,
		"turns_requested":  r.cfg.Turns,
		"turns_completed":  r.turnsCompleted,
	}
}

func (r *Runner) setChatID(chatID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chatID = chatID
}

func (r *Runner) getChatID() uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.chatID
}

func (r *Runner) storeResults(status string, totalDuration time.Duration, sawFirstOutput bool, retryCount int, eventCount int, turnsCompleted int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalStatus = status
	r.totalDuration = totalDuration
	r.sawFirstOutput = sawFirstOutput
	r.retryCount = retryCount
	r.eventCount = eventCount
	r.turnsCompleted = turnsCompleted
}

func (r *Runner) terminalMetricLabelValues(status string) []string {
	labelValues := make([]string, 0, len(r.cfg.MetricLabelValues)+1)
	labelValues = append(labelValues, r.cfg.MetricLabelValues...)
	labelValues = append(labelValues, status)
	return labelValues
}
