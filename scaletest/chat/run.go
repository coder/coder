package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type workspaceBuildRunner interface {
	RunReturningWorkspace(ctx context.Context, id string, logs io.Writer) (workspacebuild.SlimWorkspace, error)
	Cleanup(ctx context.Context, id string, logs io.Writer) error
}

type Runner struct {
	client     *codersdk.Client
	chatClient *codersdk.ExperimentalClient
	cfg        Config

	archiveChat             func(ctx context.Context, chatID uuid.UUID) error
	newWorkspaceBuildRunner func(client *codersdk.Client, cfg workspacebuild.Config) workspaceBuildRunner

	workspacebuildRunner workspaceBuildRunner

	// Set during Run, used in Cleanup and GetMetrics.
	mu             sync.Mutex
	workspaceID    uuid.UUID
	chatID         uuid.UUID
	finalStatus    string
	failureStage   string
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
	chatClient := codersdk.NewExperimentalClient(client)
	return &Runner{
		client:     client,
		chatClient: chatClient,
		cfg:        cfg,
		archiveChat: func(ctx context.Context, chatID uuid.UUID) error {
			archived := true
			return chatClient.UpdateChat(ctx, chatID, codersdk.UpdateChatRequest{Archived: &archived})
		},
		newWorkspaceBuildRunner: func(client *codersdk.Client, cfg workspacebuild.Config) workspaceBuildRunner {
			return workspacebuild.NewRunner(client, cfg)
		},
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) (err error) {
	readySignaled := false
	followUpGateSignaled := false
	defer func() {
		if !readySignaled && r.cfg.ReadyWaitGroup != nil {
			r.cfg.ReadyWaitGroup.Done()
		}
		r.signalFollowUpGateReady(&followUpGateSignaled)
	}()

	workspaceID := r.cfg.WorkspaceID
	if r.cfg.CreatesWorkspace() {
		_, _ = fmt.Fprintln(logs, "Creating workspace...")
		r.workspacebuildRunner = r.newWorkspaceBuildRunner(r.client, r.cfg.Workspace)
		workspace, err := r.workspacebuildRunner.RunReturningWorkspace(ctx, id, logs)
		if err != nil {
			r.recordStageFailure(failureStageCreateWorkspace)
			r.storeResults("", failureStageCreateWorkspace, 0, false, 0, 0, 0)
			return xerrors.Errorf("create workspace: %w", err)
		}
		workspaceID = workspace.ID
	}
	r.setWorkspaceID(workspaceID)

	r.cfg.ReadyWaitGroup.Done()
	readySignaled = true

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.cfg.StartChan:
	}

	_, _ = fmt.Fprintf(logs, "starting chat runner %s for workspace %s\n", id, workspaceID)

	conversationStart := time.Now()
	turnStartTime := conversationStart
	currentPhase := phaseInitial
	var (
		sawRunning         bool
		sawTurnFirstOutput bool
		sawAnyFirstOutput  bool
		retryCount         int
		eventCount         int
		lastStreamError    string
		turnsCompleted     int
		finalStatus        string
		failureStage       string
	)
	defer func() {
		totalDuration := time.Since(conversationStart)
		r.cfg.Metrics.ChatConversationDurationSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(totalDuration.Seconds())
		r.signalFollowUpGateReady(&followUpGateSignaled)
		r.storeResults(finalStatus, failureStage, totalDuration, sawAnyFirstOutput, retryCount, eventCount, turnsCompleted)
	}()

	chat, err := r.chatClient.CreateChat(ctx, codersdk.CreateChatRequest{
		WorkspaceID:   &workspaceID,
		ModelConfigID: r.cfg.ModelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
	})
	createDuration := time.Since(conversationStart)
	r.cfg.Metrics.ChatCreateLatencySeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(createDuration.Seconds())
	if err != nil {
		failureStage = failureStageCreateChat
		r.cfg.Metrics.ChatCreateErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		r.recordStageFailure(failureStage)
		return xerrors.Errorf("create chat: %w", err)
	}

	r.setChatID(chat.ID)
	_, _ = fmt.Fprintf(logs, "created chat %s in %s\n", chat.ID, createDuration)

	events, closer, err := r.chatClient.StreamChat(ctx, chat.ID, nil)
	if err != nil {
		failureStage = failureStageStreamOpen
		r.cfg.Metrics.ChatStreamErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		r.recordStageFailure(failureStage)
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
				r.cfg.Metrics.ChatTimeToRunningSeconds.WithLabelValues(r.phaseMetricLabelValues(currentPhase)...).Observe(time.Since(turnStartTime).Seconds())
				_, _ = fmt.Fprintf(logs, "chat %s reached running status for %s phase\n", chat.ID, currentPhase)
			case codersdk.ChatStatusWaiting:
				turnsCompleted++
				turnDuration := time.Since(turnStartTime)
				r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.phaseMetricLabelValues(currentPhase)...).Observe(turnDuration.Seconds())
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
				if nextTurn == 2 && r.cfg.ShouldGateFollowUps() {
					r.signalFollowUpGateReady(&followUpGateSignaled)
					_, _ = fmt.Fprintf(logs, "chat %s waiting for delayed follow-up phase release (%s)\n", chat.ID, r.cfg.FollowUpStartDelay)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-r.cfg.StartFollowUpChan:
					}
				}

				currentPhase = phaseFollowUp
				sawRunning = false
				sawTurnFirstOutput = false
				turnStartTime = time.Now()
				messageStartTime := turnStartTime
				_, err = r.chatClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
					Content: []codersdk.ChatInputPart{{
						Type: codersdk.ChatInputPartTypeText,
						Text: r.cfg.FollowUpPrompt,
					}},
					ModelConfigID: r.cfg.ModelConfigID,
				})
				r.cfg.Metrics.ChatMessageLatencySeconds.WithLabelValues(r.phaseMetricLabelValues(currentPhase)...).Observe(time.Since(messageStartTime).Seconds())
				if err != nil {
					failureStage = failureStageCreateMessage
					r.recordStageFailure(failureStage)
					return xerrors.Errorf("create chat message for turn %d: %w", nextTurn, err)
				}
				_, _ = fmt.Fprintf(logs, "chat %s sent follow-up message for turn %d/%d\n", chat.ID, nextTurn, r.cfg.Turns)
			case codersdk.ChatStatusError:
				finalStatus = string(codersdk.ChatStatusError)
				failureStage = failureStageStatusError
				turnDuration := time.Since(turnStartTime)
				r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.phaseMetricLabelValues(currentPhase)...).Observe(turnDuration.Seconds())
				r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(r.terminalMetricLabelValues(string(codersdk.ChatStatusError))...).Inc()
				r.recordStageFailure(failureStage)

				errMessage := lastStreamError
				if errMessage == "" {
					errMessage = "chat reached error status"
				}
				_, _ = fmt.Fprintf(logs, "chat %s reached terminal status %q after %d/%d turns: %s\n", chat.ID, codersdk.ChatStatusError, turnsCompleted, r.cfg.Turns, errMessage)
				return xerrors.Errorf("chat %s reached error status: %s", chat.ID, errMessage)
			}
		case codersdk.ChatStreamEventTypeMessagePart:
			if sawTurnFirstOutput {
				continue
			}
			sawTurnFirstOutput = true
			sawAnyFirstOutput = true
			firstOutputDuration := time.Since(turnStartTime)
			r.cfg.Metrics.ChatTimeToFirstOutputSeconds.WithLabelValues(r.phaseMetricLabelValues(currentPhase)...).Observe(firstOutputDuration.Seconds())
			_, _ = fmt.Fprintf(logs, "chat %s received first output for %s phase in %s\n", chat.ID, currentPhase, firstOutputDuration)
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
			r.recordStageFailure(failureStageStreamEvent)
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

	failureStage = failureStageStreamClosed
	r.cfg.Metrics.ChatStreamErrorsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
	r.recordStageFailure(failureStage)
	if lastStreamError != "" {
		return xerrors.Errorf("chat %s stream ended before completing %d of %d turns: %s", chat.ID, turnsCompleted, r.cfg.Turns, lastStreamError)
	}
	return xerrors.Errorf("chat %s stream ended before completing %d of %d turns", chat.ID, turnsCompleted, r.cfg.Turns)
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	chatID := r.getChatID()
	var resultErr error

	if chatID != uuid.Nil {
		_, _ = fmt.Fprintf(logs, "archiving chat %s for runner %s\n", chatID, id)
		if err := r.archiveChat(ctx, chatID); err != nil {
			_, _ = fmt.Fprintf(logs, "failed to archive chat %s: %v\n", chatID, err)
			resultErr = errors.Join(resultErr, xerrors.Errorf("archive chat: %w", err))
		} else {
			_, _ = fmt.Fprintf(logs, "archived chat %s\n", chatID)
		}
	}

	if r.workspacebuildRunner == nil {
		return resultErr
	}

	_, _ = fmt.Fprintln(logs, "Cleaning up workspace...")
	if err := r.workspacebuildRunner.Cleanup(ctx, id, logs); err != nil {
		resultErr = errors.Join(resultErr, xerrors.Errorf("cleanup workspace: %w", err))
	}

	return resultErr
}

func (r *Runner) GetMetrics() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	return map[string]any{
		"run_id":                   r.cfg.RunID,
		"workspace_id":             r.workspaceID.String(),
		"follow_up_delay_enabled":  r.cfg.ShouldGateFollowUps(),
		"follow_up_start_delay_ms": r.cfg.FollowUpStartDelay.Milliseconds(),
		"chat_id":                  r.chatID.String(),
		"final_status":             r.finalStatus,
		"failure_stage":            r.failureStage,
		"total_duration":           r.totalDuration.Seconds(),
		"saw_first_output":         r.sawFirstOutput,
		"retry_count":              r.retryCount,
		"event_count":              r.eventCount,
		"turns_requested":          r.cfg.Turns,
		"turns_completed":          r.turnsCompleted,
	}
}

func (r *Runner) setWorkspaceID(workspaceID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceID = workspaceID
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

func (r *Runner) storeResults(status string, failureStage string, totalDuration time.Duration, sawFirstOutput bool, retryCount int, eventCount int, turnsCompleted int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalStatus = status
	r.failureStage = failureStage
	r.totalDuration = totalDuration
	r.sawFirstOutput = sawFirstOutput
	r.retryCount = retryCount
	r.eventCount = eventCount
	r.turnsCompleted = turnsCompleted
}

func (r *Runner) phaseMetricLabelValues(phase string) []string {
	labelValues := make([]string, 0, len(r.cfg.MetricLabelValues)+1)
	labelValues = append(labelValues, r.cfg.MetricLabelValues...)
	labelValues = append(labelValues, phase)
	return labelValues
}

func (r *Runner) terminalMetricLabelValues(status string) []string {
	labelValues := make([]string, 0, len(r.cfg.MetricLabelValues)+1)
	labelValues = append(labelValues, r.cfg.MetricLabelValues...)
	labelValues = append(labelValues, status)
	return labelValues
}

func (r *Runner) stageMetricLabelValues(stage string) []string {
	labelValues := make([]string, 0, len(r.cfg.MetricLabelValues)+1)
	labelValues = append(labelValues, r.cfg.MetricLabelValues...)
	labelValues = append(labelValues, stage)
	return labelValues
}

func (r *Runner) recordStageFailure(stage string) {
	if stage == "" {
		return
	}
	r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.stageMetricLabelValues(stage)...).Inc()
}

func (r *Runner) signalFollowUpGateReady(signaled *bool) {
	if signaled == nil || *signaled || !r.cfg.ShouldGateFollowUps() || r.cfg.FollowUpReadyWaitGroup == nil {
		return
	}
	r.cfg.FollowUpReadyWaitGroup.Done()
	*signaled = true
}
