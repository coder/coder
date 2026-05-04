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
	"go.opentelemetry.io/otel/trace"
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

	followUpGate followUpGate

	mu     sync.Mutex
	chatID uuid.UUID
	result runnerResult
}

type followUpGate struct {
	config followUpGateConfig
	ready  sync.Once
}

func newFollowUpGate(cfg Config) followUpGate {
	return followUpGate{config: cfg.followUpGateConfig()}
}

func (g *followUpGate) enabled() bool {
	return g.config.enabled
}

func (g *followUpGate) markReady() {
	if !g.enabled() || g.config.readyWaitGroup == nil {
		return
	}
	g.ready.Do(func() {
		g.config.readyWaitGroup.Done()
	})
}

func (g *followUpGate) wait(ctx context.Context) error {
	if !g.enabled() || g.config.releaseChan == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.config.releaseChan:
		return nil
	}
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

type followUpSender func(ctx context.Context, nextTurn int, phase string) error

type conversationState struct {
	result             runnerResult
	turnStartTime      time.Time
	currentPhase       string
	lastStreamError    string
	sawRunning         bool
	sawTurnFirstOutput bool
}

func newConversationState(conversationStart time.Time) conversationState {
	return conversationState{
		turnStartTime: conversationStart,
		currentPhase:  phaseInitial,
	}
}

func (s *conversationState) startNextTurn() {
	s.currentPhase = phaseFollowUp
	s.turnStartTime = time.Now()
	s.lastStreamError = ""
	s.sawRunning = false
	s.sawTurnFirstOutput = false
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:       client,
		cfg:          cfg,
		followUpGate: newFollowUpGate(cfg),
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
		attribute.Int64("chat.follow_up_start_delay_ms", r.cfg.FollowUpStartDelay.Milliseconds()),
	)
	if r.cfg.ModelConfigID != nil {
		span.SetAttributes(attribute.String("chat.model_config_id", r.cfg.ModelConfigID.String()))
	}

	readySignaled := false
	defer func() {
		if !readySignaled && r.cfg.ReadyWaitGroup != nil {
			r.cfg.ReadyWaitGroup.Done()
		}
		r.followUpGate.markReady()
	}()

	result := runnerResult{}
	conversationStart := time.Time{}
	defer func() {
		if !conversationStart.IsZero() {
			result.totalDuration = time.Since(conversationStart)
			r.cfg.Metrics.ChatConversationDurationSeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(result.totalDuration.Seconds())
		}
		r.setResult(result)
		setRunnerSpanResultAttributes(span, result)
	}()

	readySignaled = true
	r.cfg.ReadyWaitGroup.Done()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.cfg.StartChan:
	}

	workspaceID := r.cfg.WorkspaceID
	_, _ = fmt.Fprintf(logs, "starting chat runner %s for workspace %s\n", id, workspaceID)

	conversationStart = time.Now()

	initialPrompt, err := r.cfg.PromptForTurn(0)
	if err != nil {
		result.failureStage = failureStageCreateChat
		r.recordStageFailure(result.failureStage)
		return xerrors.Errorf("build prompt for turn 1: %w", err)
	}

	createStartedAt := time.Now()
	chat, err := codersdk.NewExperimentalClient(r.client).CreateChat(ctx, codersdk.CreateChatRequest{
		WorkspaceID:   &workspaceID,
		ModelConfigID: r.cfg.ModelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: initialPrompt,
		}},
	})
	if err != nil {
		result.failureStage = failureStageCreateChat
		r.recordStageFailure(result.failureStage)
		return xerrors.Errorf("create chat: %w", err)
	}
	r.cfg.Metrics.ChatCreateLatencySeconds.WithLabelValues(r.cfg.MetricLabelValues...).Observe(time.Since(createStartedAt).Seconds())

	r.setChatID(chat.ID)
	span.SetAttributes(attribute.String("chat.chat_id", chat.ID.String()))
	_, _ = fmt.Fprintf(logs, "created chat %s in %s\n", chat.ID, time.Since(createStartedAt))

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

	sendFollowUp := func(ctx context.Context, nextTurn int, phase string) error {
		followUpPrompt, err := r.cfg.PromptForTurn(nextTurn - 1)
		if err != nil {
			return xerrors.Errorf("build prompt for turn %d: %w", nextTurn, err)
		}

		messageStartedAt := time.Now()
		_, err = codersdk.NewExperimentalClient(r.client).CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: followUpPrompt,
			}},
			ModelConfigID: r.cfg.ModelConfigID,
		})
		if err != nil {
			return xerrors.Errorf("create chat message for turn %d: %w", nextTurn, err)
		}

		r.cfg.Metrics.ChatMessageLatencySeconds.WithLabelValues(r.labelValues(phase)...).Observe(time.Since(messageStartedAt).Seconds())
		_, _ = fmt.Fprintf(logs, "chat %s sent follow-up message for turn %d/%d\n", chat.ID, nextTurn, r.cfg.Turns)
		return nil
	}

	result, err = r.runConversation(ctx, chat.ID, logs, conversationStart, events, sendFollowUp)
	return err
}

func (r *Runner) runConversation(ctx context.Context, chatID uuid.UUID, logs io.Writer, conversationStart time.Time, events <-chan codersdk.ChatStreamEvent, sendFollowUp followUpSender) (runnerResult, error) {
	state := newConversationState(conversationStart)

	for event := range events {
		state.result.eventCount++

		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				continue
			}
			done, err := r.handleStatusEvent(ctx, chatID, logs, conversationStart, &state, event.Status.Status, sendFollowUp)
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

func (r *Runner) handleStatusEvent(ctx context.Context, chatID uuid.UUID, logs io.Writer, conversationStart time.Time, state *conversationState, status codersdk.ChatStatus, sendFollowUp followUpSender) (bool, error) {
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

		nextTurn := state.result.turnsCompleted + 1
		if nextTurn == 2 {
			r.followUpGate.markReady()
			if r.followUpGate.enabled() {
				_, _ = fmt.Fprintf(logs, "chat %s waiting for delayed follow-up phase release (%s)\n", chatID, r.cfg.FollowUpStartDelay)
			}
			if err := r.followUpGate.wait(ctx); err != nil {
				return false, err
			}
		}

		state.startNextTurn()
		if err := sendFollowUp(ctx, nextTurn, state.currentPhase); err != nil {
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

	r.mu.Lock()
	chatID := r.chatID
	r.mu.Unlock()

	if chatID == uuid.Nil {
		return nil
	}

	_, _ = fmt.Fprintf(logs, "archiving chat %s for runner %s\n", chatID, id)
	if err := r.archiveChat(ctx, chatID); err != nil {
		_, _ = fmt.Fprintf(logs, "failed to archive chat %s: %v\n", chatID, err)
		return xerrors.Errorf("archive chat: %w", err)
	}
	_, _ = fmt.Fprintf(logs, "archived chat %s\n", chatID)
	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	r.mu.Lock()
	chatID := r.chatID
	result := r.result
	r.mu.Unlock()

	return map[string]any{
		"run_id":                   r.cfg.RunID,
		"workspace_id":             r.cfg.WorkspaceID.String(),
		"follow_up_delay_enabled":  r.cfg.ShouldGateFollowUps(),
		"follow_up_start_delay_ms": r.cfg.FollowUpStartDelay.Milliseconds(),
		"chat_id":                  chatID.String(),
		"final_status":             result.finalStatus,
		"failure_stage":            result.failureStage,
		"total_duration_seconds":   result.totalDuration.Seconds(),
		"saw_first_output":         result.sawFirstOutput,
		"retry_count":              result.retryCount,
		"event_count":              result.eventCount,
		"turns_requested":          r.cfg.Turns,
		"turns_completed":          result.turnsCompleted,
		"tool_calls_per_chat":      r.cfg.ToolCallsPerChat,
		"tool_call_seed":           r.cfg.ToolCallSeed,
		"tool_call_command":        r.cfg.ToolCallCommand,
	}
}

func (r *Runner) setChatID(chatID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chatID = chatID
}

func (r *Runner) setResult(result runnerResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.result = result
}

func (r *Runner) labelValues(extra string) []string {
	return append(slices.Clone(r.cfg.MetricLabelValues), extra)
}

func (r *Runner) recordStageFailure(stage string) {
	if stage == "" {
		return
	}
	r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.labelValues(stage)...).Inc()
}

func setRunnerSpanResultAttributes(span trace.Span, result runnerResult) {
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
}
