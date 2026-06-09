package chat

import (
	"context"
	"io"
	"sync"
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
	client chatClient
	cfg    Config

	chatID uuid.UUID
	result runnerResult

	conversationStart  time.Time
	turnStartTime      time.Time
	currentPhase       string
	lastStreamError    string
	lastStatus         codersdk.ChatStatus
	sawTurnRunning     bool
	sawTurnFirstOutput bool
	markTurnStartReady func()
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

	markTurnStartReady := func() {}
	if r.cfg.TurnStartReadyWaitGroup != nil {
		markTurnStartReady = sync.OnceFunc(r.cfg.TurnStartReadyWaitGroup.Done)
	}
	r.markTurnStartReady = markTurnStartReady
	defer r.markTurnStartReady()

	defer func() {
		if !r.conversationStart.IsZero() {
			r.result.totalDuration = time.Since(r.conversationStart)
			r.cfg.Metrics.ChatConversationDurationSeconds.Observe(r.result.totalDuration.Seconds())
		}
		span.SetAttributes(
			attribute.String("chat.final_status", r.result.finalStatus),
			attribute.String("chat.failure_stage", r.result.failureStage),
			attribute.Int("chat.retry_count", r.result.retryCount),
			attribute.Int("chat.turns_completed", r.result.turnsCompleted),
			attribute.Bool("chat.saw_first_output", r.result.sawFirstOutput),
		)
		if r.result.totalDuration > 0 {
			span.SetAttributes(attribute.Float64("chat.total_duration_seconds", r.result.totalDuration.Seconds()))
		}
	}()

	workspaceID := r.cfg.WorkspaceID
	modelConfigID := r.cfg.ModelConfigID
	logger = logger.With(slog.F("workspace_id", workspaceID))
	logger.Info(ctx, "starting chat runner")

	r.resetConversation(time.Now(), markTurnStartReady)

	createStartedAt := time.Now()
	createReq := codersdk.CreateChatRequest{
		OrganizationID: r.cfg.OrganizationID,
		ModelConfigID:  &modelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: r.cfg.Prompt,
		}},
	}
	if workspaceID != uuid.Nil {
		createReq.WorkspaceID = &workspaceID
	}
	chat, err := r.client.CreateChat(ctx, createReq)
	if err != nil {
		r.result.failureStage = failureStageCreateChat
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.result.failureStage).Inc()
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
		r.result.failureStage = failureStageStreamOpen
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.result.failureStage).Inc()
		return xerrors.Errorf("stream chat: %w", err)
	}

	r.cfg.Metrics.ActiveChatStreams.Inc()
	defer func() {
		r.cfg.Metrics.ActiveChatStreams.Dec()
		_ = closer.Close()
	}()

	logger.Info(ctx, "streaming chat events")

	return r.runConversation(ctx, chat.ID, logger, events)
}

func (r *Runner) resetConversation(conversationStart time.Time, markTurnStartReady func()) {
	if markTurnStartReady == nil {
		markTurnStartReady = func() {}
	}

	r.result = runnerResult{}
	r.conversationStart = conversationStart
	r.turnStartTime = conversationStart
	r.currentPhase = phaseInitial
	r.lastStreamError = ""
	r.lastStatus = ""
	r.sawTurnRunning = false
	r.sawTurnFirstOutput = false
	r.markTurnStartReady = markTurnStartReady
}

func (r *Runner) runConversation(ctx context.Context, chatID uuid.UUID, logger slog.Logger, events <-chan codersdk.ChatStreamEvent) error {
	r.chatID = chatID

	for event := range events {
		r.result.eventCount++

		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				continue
			}
			done, err := r.handleStatusEvent(ctx, chatID, logger, event.Status.Status)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		case codersdk.ChatStreamEventTypeMessagePart:
			r.handleMessagePartEvent(ctx, logger)
		case codersdk.ChatStreamEventTypeMessage:
			// StreamChat replays persisted rows as message events, not
			// message_part deltas, when a turn finished server-side before
			// the stream attached. Route assistant rows through the same
			// first-output path; skip user rows so persisted prompts do not
			// count as model output.
			if event.Message == nil || event.Message.Role != codersdk.ChatMessageRoleAssistant {
				continue
			}
			r.handleMessagePartEvent(ctx, logger)
		case codersdk.ChatStreamEventTypeRetry:
			r.handleRetryEvent(ctx, logger, event.Retry)
		case codersdk.ChatStreamEventTypeError:
			r.handleErrorEvent(ctx, logger, event.Error)
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	r.result.failureStage = failureStageStreamEndedEarly
	r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.result.failureStage).Inc()
	if r.lastStreamError != "" {
		return xerrors.Errorf("chat %s stream ended before completing %d of %d turns: %s", chatID, r.result.turnsCompleted, r.cfg.Turns, r.lastStreamError)
	}
	return xerrors.Errorf("chat %s stream ended before completing %d of %d turns", chatID, r.result.turnsCompleted, r.cfg.Turns)
}

func (r *Runner) handleStatusEvent(ctx context.Context, chatID uuid.UUID, logger slog.Logger, status codersdk.ChatStatus) (bool, error) {
	if status == r.lastStatus {
		return false, nil
	}
	if status == codersdk.ChatStatusWaiting &&
		!r.sawTurnFirstOutput &&
		(r.sawTurnRunning || r.result.turnsCompleted > 0) {
		return false, nil
	}
	r.lastStatus = status

	switch status {
	case codersdk.ChatStatusRunning:
		r.sawTurnRunning = true
		r.cfg.Metrics.ChatTimeToRunningSeconds.WithLabelValues(r.currentPhase).Observe(time.Since(r.turnStartTime).Seconds())
		logger.Info(ctx, "chat reached running status",
			slog.F("phase", r.currentPhase),
		)
		return false, nil
	case codersdk.ChatStatusWaiting:
		r.result.turnsCompleted++
		turnDuration := time.Since(r.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.currentPhase).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(string(codersdk.ChatStatusWaiting)).Inc()
		r.cfg.Metrics.ChatTurnsCompletedTotal.Inc()
		logger.Info(ctx, "chat completed turn",
			slog.F("turn", r.result.turnsCompleted),
			slog.F("turns", r.cfg.Turns),
			slog.F("duration", turnDuration),
		)
		if r.result.turnsCompleted >= r.cfg.Turns {
			r.result.finalStatus = string(codersdk.ChatStatusWaiting)
			conversationDuration := time.Since(r.conversationStart)
			logger.Info(ctx, "chat reached terminal status",
				slog.F("status", codersdk.ChatStatusWaiting),
				slog.F("duration", conversationDuration),
				slog.F("turns_completed", r.result.turnsCompleted),
			)
			return true, nil
		}

		// After the very first turn completes, mark this runner ready
		// for the CLI-coordinated turn-start gate. The inter-phase
		// delay measures the gap between every chat actually finishing its
		// initial turn and the start of the follow-up turns, not the gap
		// between CreateChat returning and the next turn.
		if r.result.turnsCompleted == 1 {
			r.markTurnStartReady()
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

		nextTurn := r.result.turnsCompleted + 1
		r.currentPhase = phaseFollowUp
		r.turnStartTime = time.Now()
		r.lastStreamError = ""
		r.lastStatus = ""
		r.sawTurnRunning = false
		r.sawTurnFirstOutput = false
		if err := r.sendNextTurn(ctx, chatID, logger, nextTurn, r.currentPhase); err != nil {
			r.result.failureStage = failureStageCreateMessage
			r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.result.failureStage).Inc()
			return false, err
		}
		return false, nil
	case codersdk.ChatStatusError:
		r.result.finalStatus = string(codersdk.ChatStatusError)
		r.result.failureStage = failureStageStatusError
		turnDuration := time.Since(r.turnStartTime)
		r.cfg.Metrics.ChatTimeToTerminalStatusSeconds.WithLabelValues(r.currentPhase).Observe(turnDuration.Seconds())
		r.cfg.Metrics.ChatTerminalStatusTotal.WithLabelValues(string(codersdk.ChatStatusError)).Inc()
		r.cfg.Metrics.ChatStageFailuresTotal.WithLabelValues(r.result.failureStage).Inc()

		errMessage := r.lastStreamError
		if errMessage == "" {
			errMessage = "chat reached error status"
		}
		logger.Error(ctx, "chat reached terminal status",
			slog.F("status", codersdk.ChatStatusError),
			slog.F("turns_completed", r.result.turnsCompleted),
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

func (r *Runner) handleMessagePartEvent(ctx context.Context, logger slog.Logger) {
	if r.sawTurnFirstOutput {
		return
	}
	r.sawTurnFirstOutput = true
	r.result.sawFirstOutput = true
	firstOutputDuration := time.Since(r.turnStartTime)
	r.cfg.Metrics.ChatTimeToFirstOutputSeconds.WithLabelValues(r.currentPhase).Observe(firstOutputDuration.Seconds())
	logger.Info(ctx, "chat received first output",
		slog.F("phase", r.currentPhase),
		slog.F("duration", firstOutputDuration),
	)
}

func (r *Runner) handleRetryEvent(ctx context.Context, logger slog.Logger, retry *codersdk.ChatStreamRetry) {
	r.result.retryCount++
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

func (r *Runner) handleErrorEvent(ctx context.Context, logger slog.Logger, eventErr *codersdk.ChatError) {
	if eventErr != nil && eventErr.Message != "" {
		r.lastStreamError = eventErr.Message
		logger.Warn(ctx, "chat stream error",
			slog.F("error", r.lastStreamError),
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
