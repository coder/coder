package chat

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

// Runner executes a single chat conversation as part of a scaletest run.
type Runner struct {
	client *codersdk.Client
	cfg    Config

	archiveChat func(ctx context.Context, chatID uuid.UUID) error

	turnStartGate turnStartGate

	chatID uuid.UUID
	result runnerResult
}

type turnStartGate struct {
	readyWaitGroup *sync.WaitGroup
	releaseChan    <-chan struct{}
	ready          sync.Once
}

func (g *turnStartGate) markReady() {
	if g.readyWaitGroup == nil {
		return
	}
	g.ready.Do(func() {
		g.readyWaitGroup.Done()
	})
}

type runnerResult struct {
	finalStatus    string
	failureStage   string
	totalDuration  time.Duration
	sawFirstOutput bool
	retryCount     int
	eventCount     int
	turnsCompleted int
}

type conversationState struct {
	result             runnerResult
	turnStartTime      time.Time
	currentPhase       string
	lastStreamError    string
	sawRunning         bool
	sawTurnFirstOutput bool
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
		turnStartGate: turnStartGate{
			readyWaitGroup: cfg.TurnStartReadyWaitGroup,
			releaseChan:    cfg.StartTurnsChan,
		},
		archiveChat: func(ctx context.Context, chatID uuid.UUID) error {
			archived := true
			return codersdk.NewExperimentalClient(client).UpdateChat(ctx, chatID, codersdk.UpdateChatRequest{Archived: &archived})
		},
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	span.SetAttributes(
		attribute.String("chat.runner_id", id),
		attribute.String("chat.run_id", r.cfg.RunID),
		attribute.String("chat.workspace_id", r.cfg.WorkspaceID.String()),
		attribute.Int("chat.turns_requested", r.cfg.Turns),
		attribute.Int64("chat.turn_start_delay_ms", r.cfg.TurnStartDelay.Milliseconds()),
	)
	if r.cfg.ModelConfigID != nil {
		span.SetAttributes(attribute.String("chat.model_config_id", r.cfg.ModelConfigID.String()))
	}

	defer r.turnStartGate.markReady()

	result := runnerResult{}
	conversationStart := time.Time{}
	defer func() {
		if !conversationStart.IsZero() {
			result.totalDuration = time.Since(conversationStart)
			r.cfg.Metrics.ChatConversationDurationSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(result.totalDuration.Seconds())
		}
		r.result = result
		span.SetAttributes(
			attribute.String("chat.final_status", result.finalStatus),
			attribute.String("chat.failure_stage", result.failureStage),
			attribute.Int("chat.retry_count", result.retryCount),
			attribute.Int("chat.turns_completed", result.turnsCompleted),
			attribute.Bool("chat.saw_first_output", result.sawFirstOutput),
		)
		if result.totalDuration > 0 {
			span.SetAttributes(attribute.Float64("chat.total_duration_seconds", result.totalDuration.Seconds()))
		}
	}()

	r.cfg.ReadyWaitGroup.Done()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.cfg.StartChan:
	}

	workspaceID := r.cfg.WorkspaceID
	_, _ = fmt.Fprintf(logs, "starting chat runner %s for workspace %s\n", id, workspaceID)

	conversationStart = time.Now()

	createStartedAt := time.Now()
	chat, err := codersdk.NewExperimentalClient(r.client).CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: r.cfg.OrganizationID,
		WorkspaceID:    &workspaceID,
		ModelConfigID:  r.cfg.ModelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
	})
	if err != nil {
		result.failureStage = failureStageCreateChat
		r.recordStageFailure(result.failureStage)
		return xerrors.Errorf("create chat: %w", err)
	}
	r.cfg.Metrics.ChatCreateLatencySeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(time.Since(createStartedAt).Seconds())

	r.chatID = chat.ID
	span.SetAttributes(attribute.String("chat.chat_id", chat.ID.String()))
	_, _ = fmt.Fprintf(logs, "created chat %s in %s\n", chat.ID, time.Since(createStartedAt))

	// CreateChat already queues the first prompt for processing on the
	// server, so the initial turn is in flight as soon as CreateChat
	// returns. Open the stream immediately and let the conversation loop
	// drive the gate at the natural phase boundary (after the first turn
	// reaches a terminal Waiting status), rather than fencing here on a
	// turn that has already started running.
	events, closer, err := codersdk.NewExperimentalClient(r.client).StreamChat(ctx, chat.ID, nil)
	if err != nil {
		result.failureStage = failureStageStreamOpen
		r.recordStageFailure(result.failureStage)
		return xerrors.Errorf("stream chat: %w", err)
	}

	r.cfg.Metrics.ActiveChatStreams.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
	defer func() {
		r.cfg.Metrics.ActiveChatStreams.WithLabelValues(r.cfg.MetricLabelValues...).Dec()
		_ = closer.Close()
	}()

	_, _ = fmt.Fprintf(logs, "streaming chat %s\n", chat.ID)

	sendNextTurn := func(ctx context.Context, nextTurn int, phase string) error {
		messageStartedAt := time.Now()
		_, err := codersdk.NewExperimentalClient(r.client).CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: r.cfg.Prompt,
			}},
			ModelConfigID: r.cfg.ModelConfigID,
		})
		if err != nil {
			return xerrors.Errorf("create chat message for turn %d: %w", nextTurn, err)
		}

		r.cfg.Metrics.ChatMessageLatencySeconds.WithLabelValues(r.labelValues(phase)...).Observe(time.Since(messageStartedAt).Seconds())
		_, _ = fmt.Fprintf(logs, "chat %s sent message for turn %d/%d\n", chat.ID, nextTurn, r.cfg.Turns)
		return nil
	}

	result, err = r.runConversation(ctx, chat.ID, logs, conversationStart, events, sendNextTurn)
	return err
}

func (r *Runner) runConversation(ctx context.Context, chatID uuid.UUID, logs io.Writer, conversationStart time.Time, events <-chan codersdk.ChatStreamEvent, sendNextTurn func(ctx context.Context, nextTurn int, phase string) error) (runnerResult, error) {
	state := conversationState{
		turnStartTime: conversationStart,
		currentPhase:  phaseInitial,
	}

	for event := range events {
		state.result.eventCount++

		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				continue
			}
			done, err := r.handleStatusEvent(ctx, chatID, logs, conversationStart, &state, event.Status.Status, sendNextTurn)
			if err != nil {
				return state.result, err
			}
			if done {
				return state.result, nil
			}
		case codersdk.ChatStreamEventTypeMessagePart:
			r.handleMessagePartEvent(chatID, logs, &state)
		case codersdk.ChatStreamEventTypeRetry:
			r.handleRetryEvent(chatID, logs, &state, event.Retry)
		case codersdk.ChatStreamEventTypeError:
			handleErrorEvent(chatID, logs, &state, event.Error)
		}
	}

	if ctx.Err() != nil {
		return state.result, ctx.Err()
	}

	state.result.failureStage = failureStageStreamEndedEarly
	r.recordStageFailure(state.result.failureStage)
	if state.lastStreamError != "" {
		return state.result, xerrors.Errorf("chat %s stream ended before completing %d of %d turns: %s", chatID, state.result.turnsCompleted, r.cfg.Turns, state.lastStreamError)
	}
	return state.result, xerrors.Errorf("chat %s stream ended before completing %d of %d turns", chatID, state.result.turnsCompleted, r.cfg.Turns)
}

func (r *Runner) handleStatusEvent(ctx context.Context, chatID uuid.UUID, logs io.Writer, conversationStart time.Time, state *conversationState, status codersdk.ChatStatus, sendNextTurn func(ctx context.Context, nextTurn int, phase string) error) (bool, error) {
	switch status {
	case codersdk.ChatStatusRunning:
		if state.sawRunning {
			return false, nil
		}
		state.sawRunning = true
		r.cfg.Metrics.ChatTimeToRunningSeconds.WithLabelValues(r.labelValues(state.currentPhase)...).Observe(time.Since(state.turnStartTime).Seconds())
		_, _ = fmt.Fprintf(logs, "chat %s reached running status for %s phase\n", chatID, state.currentPhase)
		return false, nil
	case codersdk.ChatStatusWaiting:
		state.result.turnsCompleted++
		turnDuration := time.Since(state.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.labelValues(state.currentPhase)...).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(r.labelValues(string(codersdk.ChatStatusWaiting))...).Inc()
		r.cfg.Metrics.ChatTurnsCompletedTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
		_, _ = fmt.Fprintf(logs, "chat %s completed turn %d/%d in %s\n", chatID, state.result.turnsCompleted, r.cfg.Turns, turnDuration)
		if state.result.turnsCompleted >= r.cfg.Turns {
			state.result.finalStatus = string(codersdk.ChatStatusWaiting)
			conversationDuration := time.Since(conversationStart)
			_, _ = fmt.Fprintf(logs, "chat %s reached terminal status %q in %s after %d turns\n", chatID, codersdk.ChatStatusWaiting, conversationDuration, state.result.turnsCompleted)
			return true, nil
		}

		// After the very first turn completes, hand off to the CLI-
		// coordinated turn-start gate so that the inter-phase delay
		// measures the gap between every chat actually finishing its
		// initial turn and the start of the follow-up storm, not the gap
		// between CreateChat returning and the storm.
		if state.result.turnsCompleted == 1 {
			if err := r.waitForTurnStartRelease(ctx, logs, chatID); err != nil {
				return false, err
			}
		}

		nextTurn := state.result.turnsCompleted + 1
		state.currentPhase = phaseFollowUp
		state.turnStartTime = time.Now()
		state.lastStreamError = ""
		state.sawRunning = false
		state.sawTurnFirstOutput = false
		if err := sendNextTurn(ctx, nextTurn, state.currentPhase); err != nil {
			state.result.failureStage = failureStageCreateMessage
			r.recordStageFailure(state.result.failureStage)
			return false, err
		}
		return false, nil
	case codersdk.ChatStatusError:
		state.result.finalStatus = string(codersdk.ChatStatusError)
		state.result.failureStage = failureStageStatusError
		turnDuration := time.Since(state.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.labelValues(state.currentPhase)...).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(r.labelValues(string(codersdk.ChatStatusError))...).Inc()
		r.recordStageFailure(state.result.failureStage)

		errMessage := state.lastStreamError
		if errMessage == "" {
			errMessage = "chat reached error status"
		}
		_, _ = fmt.Fprintf(logs, "chat %s reached terminal status %q after %d/%d turns: %s\n", chatID, codersdk.ChatStatusError, state.result.turnsCompleted, r.cfg.Turns, errMessage)
		return false, xerrors.Errorf("chat %s reached error status: %s", chatID, errMessage)
	default:
		return false, nil
	}
}

func (r *Runner) handleMessagePartEvent(chatID uuid.UUID, logs io.Writer, state *conversationState) {
	if state.sawTurnFirstOutput {
		return
	}
	state.sawTurnFirstOutput = true
	state.result.sawFirstOutput = true
	firstOutputDuration := time.Since(state.turnStartTime)
	r.cfg.Metrics.ChatTimeToFirstOutputSeconds.WithLabelValues(r.labelValues(state.currentPhase)...).Observe(firstOutputDuration.Seconds())
	_, _ = fmt.Fprintf(logs, "chat %s received first output for %s phase in %s\n", chatID, state.currentPhase, firstOutputDuration)
}

func (r *Runner) handleRetryEvent(chatID uuid.UUID, logs io.Writer, state *conversationState, retry *codersdk.ChatStreamRetry) {
	state.result.retryCount++
	r.cfg.Metrics.ChatRetryEventsTotal.WithLabelValues(r.cfg.MetricLabelValues...).Inc()
	if retry != nil {
		_, _ = fmt.Fprintf(logs, "chat %s retry attempt %d in %dms: %s\n", chatID, retry.Attempt, retry.DelayMs, retry.Error)
		return
	}
	_, _ = fmt.Fprintf(logs, "chat %s received retry event\n", chatID)
}

func handleErrorEvent(chatID uuid.UUID, logs io.Writer, state *conversationState, eventErr *codersdk.ChatError) {
	if eventErr != nil && eventErr.Message != "" {
		state.lastStreamError = eventErr.Message
		_, _ = fmt.Fprintf(logs, "chat %s stream error: %s\n", chatID, state.lastStreamError)
		return
	}
	_, _ = fmt.Fprintf(logs, "chat %s received stream error event\n", chatID)
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	logs = loadtestutil.NewSyncWriter(logs)

	if r.chatID == uuid.Nil {
		return nil
	}

	_, _ = fmt.Fprintf(logs, "archiving chat %s for runner %s\n", r.chatID, id)
	if err := r.archiveChat(ctx, r.chatID); err != nil {
		_, _ = fmt.Fprintf(logs, "failed to archive chat %s: %v\n", r.chatID, err)
		return xerrors.Errorf("archive chat: %w", err)
	}
	_, _ = fmt.Fprintf(logs, "archived chat %s\n", r.chatID)
	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	return map[string]any{
		"run_id":                 r.cfg.RunID,
		"workspace_id":           r.cfg.WorkspaceID.String(),
		"turn_start_delay_ms":    r.cfg.TurnStartDelay.Milliseconds(),
		"chat_id":                r.chatID.String(),
		"final_status":           r.result.finalStatus,
		"failure_stage":          r.result.failureStage,
		"total_duration_seconds": r.result.totalDuration.Seconds(),
		"saw_first_output":       r.result.sawFirstOutput,
		"retry_count":            r.result.retryCount,
		"event_count":            r.result.eventCount,
		"turns_requested":        r.cfg.Turns,
		"turns_completed":        r.result.turnsCompleted,
	}
}

func (r *Runner) labelValues(extra string) []string {
	return append(slices.Clone(r.cfg.MetricLabelValues), extra)
}

// waitForTurnStartRelease signals that this runner has completed its first
// turn and then blocks until the CLI releases the follow-up turn storm.
// markReady is idempotent, so calling this on the natural path and again
// via the deferred safety-net signal in Run is safe. When the release
// channel is not configured this returns immediately after signaling.
func (r *Runner) waitForTurnStartRelease(ctx context.Context, logs io.Writer, chatID uuid.UUID) error {
	r.turnStartGate.markReady()
	if r.turnStartGate.releaseChan == nil {
		return nil
	}
	_, _ = fmt.Fprintf(logs, "chat %s waiting for turn start release (%s)\n", chatID, r.cfg.TurnStartDelay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.turnStartGate.releaseChan:
		return nil
	}
}

func (r *Runner) recordStageFailure(stage string) {
	if stage == "" {
		return
	}
	r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.labelValues(stage)...).Inc()
}
