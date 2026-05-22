package chat

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

// Runner executes a single chat conversation as part of a scaletest run.
type Runner struct {
	client *codersdk.ExperimentalClient
	cfg    Config

	chatID uuid.UUID
	result runnerResult
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
	result                   runnerResult
	turnStartTime            time.Time
	currentPhase             string
	lastStreamError          string
	lastStatus               codersdk.ChatStatus
	sawTurnRunning           bool
	sawTurnFirstOutput       bool
	shouldMarkTurnStartReady bool
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: codersdk.NewExperimentalClient(client),
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(id)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	span.SetAttributes(
		attribute.String("chat.runner_id", id),
		attribute.String("chat.workspace_id", r.cfg.WorkspaceID.String()),
		attribute.Int("chat.turns_requested", r.cfg.Turns),
		attribute.Int64("chat.turn_start_delay_ms", r.cfg.TurnStartDelay.Milliseconds()),
	)
	span.SetAttributes(attribute.String("chat.model_config_id", r.cfg.ModelConfigID.String()))

	shouldMarkTurnStartReady := r.cfg.TurnStartReadyWaitGroup != nil
	defer func() {
		if shouldMarkTurnStartReady {
			r.cfg.TurnStartReadyWaitGroup.Done()
		}
	}()

	result := runnerResult{}
	conversationStart := time.Time{}
	defer func() {
		if !conversationStart.IsZero() {
			result.totalDuration = time.Since(conversationStart)
			r.cfg.Metrics.ChatConversationDurationSeconds.Observe(result.totalDuration.Seconds())
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

	workspaceID := r.cfg.WorkspaceID
	modelConfigID := r.cfg.ModelConfigID
	logger = logger.With(slog.F("workspace_id", workspaceID))
	logger.Info(ctx, "starting chat runner")

	conversationStart = time.Now()

	createStartedAt := time.Now()
	chat, err := r.client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: r.cfg.OrganizationID,
		WorkspaceID:    &workspaceID,
		ModelConfigID:  &modelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
	})
	if err != nil {
		result.failureStage = failureStageCreateChat
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(result.failureStage).Inc()
		return xerrors.Errorf("create chat: %w", err)
	}
	r.cfg.Metrics.ChatCreateLatencySeconds.Observe(time.Since(createStartedAt).Seconds())

	r.chatID = chat.ID
	span.SetAttributes(attribute.String("chat.chat_id", chat.ID.String()))
	logger = logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "created chat session", slog.F("duration", time.Since(createStartedAt)))

	// CreateChat already queues the first prompt for processing on the
	// server, so the initial turn is in flight as soon as CreateChat
	// returns. Open the stream immediately and let the conversation loop
	// drive the gate at the natural phase boundary (after the first turn
	// reaches a terminal Waiting status), rather than fencing here on a
	// turn that has already started running.
	events, closer, err := r.client.StreamChat(ctx, chat.ID, nil)
	if err != nil {
		result.failureStage = failureStageStreamOpen
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(result.failureStage).Inc()
		return xerrors.Errorf("stream chat: %w", err)
	}

	r.cfg.Metrics.ActiveChatStreams.Inc()
	defer func() {
		r.cfg.Metrics.ActiveChatStreams.Dec()
		_ = closer.Close()
	}()

	logger.Info(ctx, "streaming chat events")

	shouldMarkTurnStartReady = false // we are passing this responsibility to runConversation.
	result, err = r.runConversation(ctx, chat.ID, logger, conversationStart, events)
	return err
}

func (r *Runner) runConversation(ctx context.Context, chatID uuid.UUID, logger slog.Logger, conversationStart time.Time, events <-chan codersdk.ChatStreamEvent) (runnerResult, error) {
	state := conversationState{
		turnStartTime:            conversationStart,
		currentPhase:             phaseInitial,
		shouldMarkTurnStartReady: r.cfg.TurnStartReadyWaitGroup != nil,
	}
	defer func() {
		if state.shouldMarkTurnStartReady {
			r.cfg.TurnStartReadyWaitGroup.Done()
		}
	}()

	for event := range events {
		state.result.eventCount++

		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				continue
			}
			done, err := r.handleStatusEvent(ctx, chatID, logger, conversationStart, &state, event.Status.Status)
			if err != nil {
				return state.result, err
			}
			if done {
				return state.result, nil
			}
		case codersdk.ChatStreamEventTypeMessagePart:
			r.handleMessagePartEvent(ctx, logger, &state)
		case codersdk.ChatStreamEventTypeMessage:
			// StreamChat replays persisted rows as message events, not
			// message_part deltas, when a turn finished server-side before
			// the stream attached. Route assistant rows through the same
			// first-output path; skip user rows so persisted prompts do not
			// count as model output.
			if event.Message == nil || event.Message.Role != codersdk.ChatMessageRoleAssistant {
				continue
			}
			r.handleMessagePartEvent(ctx, logger, &state)
		case codersdk.ChatStreamEventTypeRetry:
			r.handleRetryEvent(ctx, logger, &state, event.Retry)
		case codersdk.ChatStreamEventTypeError:
			handleErrorEvent(ctx, logger, &state, event.Error)
		}
	}

	if ctx.Err() != nil {
		return state.result, ctx.Err()
	}

	state.result.failureStage = failureStageStreamEndedEarly
	r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(state.result.failureStage).Inc()
	if state.lastStreamError != "" {
		return state.result, xerrors.Errorf("chat %s stream ended before completing %d of %d turns: %s", chatID, state.result.turnsCompleted, r.cfg.Turns, state.lastStreamError)
	}
	return state.result, xerrors.Errorf("chat %s stream ended before completing %d of %d turns", chatID, state.result.turnsCompleted, r.cfg.Turns)
}

func (r *Runner) handleStatusEvent(ctx context.Context, chatID uuid.UUID, logger slog.Logger, conversationStart time.Time, state *conversationState, status codersdk.ChatStatus) (bool, error) {
	if status == state.lastStatus {
		return false, nil
	}
	if status == codersdk.ChatStatusWaiting &&
		!state.sawTurnFirstOutput &&
		(state.sawTurnRunning || state.result.turnsCompleted > 0) {
		return false, nil
	}
	state.lastStatus = status

	switch status {
	case codersdk.ChatStatusRunning:
		state.sawTurnRunning = true
		r.cfg.Metrics.ChatTimeToRunningSeconds.WithLabelValues(state.currentPhase).Observe(time.Since(state.turnStartTime).Seconds())
		logger.Info(ctx, "chat reached running status",
			slog.F("phase", state.currentPhase),
		)
		return false, nil
	case codersdk.ChatStatusWaiting:
		state.result.turnsCompleted++
		turnDuration := time.Since(state.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(state.currentPhase).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(string(codersdk.ChatStatusWaiting)).Inc()
		r.cfg.Metrics.ChatTurnsCompletedTotal.Inc()
		logger.Info(ctx, "chat completed turn",
			slog.F("turn", state.result.turnsCompleted),
			slog.F("turns", r.cfg.Turns),
			slog.F("duration", turnDuration),
		)
		if state.result.turnsCompleted >= r.cfg.Turns {
			state.result.finalStatus = string(codersdk.ChatStatusWaiting)
			conversationDuration := time.Since(conversationStart)
			logger.Info(ctx, "chat reached terminal status",
				slog.F("status", codersdk.ChatStatusWaiting),
				slog.F("duration", conversationDuration),
				slog.F("turns_completed", state.result.turnsCompleted),
			)
			return true, nil
		}

		// After the very first turn completes, hand off to the CLI-
		// coordinated turn-start gate so that the inter-phase delay
		// measures the gap between every chat actually finishing its
		// initial turn and the start of the follow-up turns, not the gap
		// between CreateChat returning and the next turn.
		if state.result.turnsCompleted == 1 {
			if state.shouldMarkTurnStartReady {
				state.shouldMarkTurnStartReady = false
				r.cfg.TurnStartReadyWaitGroup.Done()
			}
			if r.cfg.StartTurnsChan != nil {
				logger.Info(ctx, "chat waiting for turn start release",
					slog.F("turn_start_delay", r.cfg.TurnStartDelay),
				)
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				case <-r.cfg.StartTurnsChan:
				}
			}
		}

		nextTurn := state.result.turnsCompleted + 1
		state.currentPhase = phaseFollowUp
		state.turnStartTime = time.Now()
		state.lastStreamError = ""
		state.lastStatus = ""
		state.sawTurnRunning = false
		state.sawTurnFirstOutput = false
		if err := r.sendNextTurn(ctx, chatID, logger, nextTurn, state.currentPhase); err != nil {
			state.result.failureStage = failureStageCreateMessage
			r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(state.result.failureStage).Inc()
			return false, err
		}
		return false, nil
	case codersdk.ChatStatusError:
		state.result.finalStatus = string(codersdk.ChatStatusError)
		state.result.failureStage = failureStageStatusError
		turnDuration := time.Since(state.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(state.currentPhase).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(string(codersdk.ChatStatusError)).Inc()
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(state.result.failureStage).Inc()

		errMessage := state.lastStreamError
		if errMessage == "" {
			errMessage = "chat reached error status"
		}
		logger.Error(ctx, "chat reached terminal status",
			slog.F("status", codersdk.ChatStatusError),
			slog.F("turns_completed", state.result.turnsCompleted),
			slog.F("turns", r.cfg.Turns),
			slog.F("error", errMessage),
		)
		return false, xerrors.Errorf("chat %s reached error status: %s", chatID, errMessage)
	default:
		return false, nil
	}
}

func (r *Runner) sendNextTurn(ctx context.Context, chatID uuid.UUID, logger slog.Logger, nextTurn int, phase string) error {
	messageStartedAt := time.Now()
	modelConfigID := r.cfg.ModelConfigID
	_, err := r.client.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
		ModelConfigID: &modelConfigID,
	})
	if err != nil {
		return xerrors.Errorf("create chat message for turn %d: %w", nextTurn, err)
	}

	r.cfg.Metrics.ChatMessageLatencySeconds.WithLabelValues(phase).Observe(time.Since(messageStartedAt).Seconds())
	logger.Info(ctx, "chat sent message",
		slog.F("turn", nextTurn),
		slog.F("turns", r.cfg.Turns),
	)
	return nil
}

func (r *Runner) handleMessagePartEvent(ctx context.Context, logger slog.Logger, state *conversationState) {
	if state.sawTurnFirstOutput {
		return
	}
	state.sawTurnFirstOutput = true
	state.result.sawFirstOutput = true
	firstOutputDuration := time.Since(state.turnStartTime)
	r.cfg.Metrics.ChatTimeToFirstOutputSeconds.WithLabelValues(state.currentPhase).Observe(firstOutputDuration.Seconds())
	logger.Info(ctx, "chat received first output",
		slog.F("phase", state.currentPhase),
		slog.F("duration", firstOutputDuration),
	)
}

func (r *Runner) handleRetryEvent(ctx context.Context, logger slog.Logger, state *conversationState, retry *codersdk.ChatStreamRetry) {
	state.result.retryCount++
	r.cfg.Metrics.ChatRetryEventsTotal.Inc()
	if retry != nil {
		logger.Warn(ctx, "chat retry event",
			slog.F("attempt", retry.Attempt),
			slog.F("delay_ms", retry.DelayMs),
			slog.F("error", retry.Error),
		)
		return
	}
	logger.Warn(ctx, "chat retry event")
}

func handleErrorEvent(ctx context.Context, logger slog.Logger, state *conversationState, eventErr *codersdk.ChatError) {
	if eventErr != nil && eventErr.Message != "" {
		state.lastStreamError = eventErr.Message
		logger.Warn(ctx, "chat stream error",
			slog.F("error", state.lastStreamError),
		)
		return
	}
	logger.Warn(ctx, "chat stream error event")
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if r.chatID == uuid.Nil {
		return nil
	}

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug).Named(id).With(slog.F("chat_id", r.chatID))
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	archived := true
	logger.Info(ctx, "archiving chat session")
	if err := r.client.UpdateChat(ctx, r.chatID, codersdk.UpdateChatRequest{Archived: &archived}); err != nil {
		logger.Error(ctx, "failed to archive chat", slog.Error(err))
		return xerrors.Errorf("archive chat: %w", err)
	}
	logger.Info(ctx, "archived chat session")
	return nil
}

func (r *Runner) GetMetrics() map[string]any {
	return map[string]any{
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
