package chatdebug

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
)

// RecorderOptions identifies the chat/model context for debug recording.
type RecorderOptions struct {
	ChatID   uuid.UUID
	OwnerID  uuid.UUID
	Provider string
	Model    string
}

// WrapModel returns model unchanged when debug recording is disabled, or a
// debug wrapper when a service is available.
func WrapModel(
	model fantasy.LanguageModel,
	svc *Service,
	opts RecorderOptions,
) fantasy.LanguageModel {
	if model == nil {
		panic("chatdebug: nil LanguageModel")
	}
	if svc == nil {
		return model
	}
	return &debugModel{inner: model, svc: svc, opts: opts}
}

type attemptSink struct {
	mu             sync.Mutex
	attempts       []Attempt
	attemptCounter atomic.Int32
}

func (s *attemptSink) nextAttemptNumber() int {
	if s == nil {
		panic("chatdebug: nil attemptSink")
	}
	return int(s.attemptCounter.Add(1))
}

func (s *attemptSink) record(a Attempt) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.attempts = append(s.attempts, a)
}

func (s *attemptSink) snapshot() []Attempt {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempts := make([]Attempt, len(s.attempts))
	copy(attempts, s.attempts)
	return attempts
}

type attemptSinkKey struct{}

func withAttemptSink(ctx context.Context, sink *attemptSink) context.Context {
	if sink == nil {
		panic("chatdebug: nil attemptSink")
	}
	return context.WithValue(ctx, attemptSinkKey{}, sink)
}

func attemptSinkFromContext(ctx context.Context) *attemptSink {
	sink, _ := ctx.Value(attemptSinkKey{}).(*attemptSink)
	return sink
}

var stepCounters sync.Map // map[uuid.UUID]*atomic.Int32

func nextStepNumber(runID uuid.UUID) int32 {
	val, _ := stepCounters.LoadOrStore(runID, &atomic.Int32{})
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: invalid step counter type")
	}
	return counter.Add(1)
}

// CleanupStepCounter removes per-run step counter and reference count
// state. This is used by tests and later stacked branches that have a
// real run lifecycle.
func CleanupStepCounter(runID uuid.UUID) {
	stepCounters.Delete(runID)
	runRefCounts.Delete(runID)
}

const stepFinalizeTimeout = 5 * time.Second

func stepFinalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		panic("chatdebug: nil context")
	}
	return context.WithTimeout(context.WithoutCancel(ctx), stepFinalizeTimeout)
}

func syncStepCounter(runID uuid.UUID, stepNumber int32) {
	val, _ := stepCounters.LoadOrStore(runID, &atomic.Int32{})
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: invalid step counter type")
	}
	for {
		current := counter.Load()
		if current >= stepNumber {
			return
		}
		if counter.CompareAndSwap(current, stepNumber) {
			return
		}
	}
}

type stepHandle struct {
	stepCtx  *StepContext
	sink     *attemptSink
	svc      *Service
	opts     RecorderOptions
	once     sync.Once
	mu       sync.Mutex
	status   Status
	response any
	usage    any
	err      any
	metadata any
}

// beginStep validates preconditions, creates a debug step, and returns a
// handle plus an enriched context carrying StepContext and attemptSink.
// Returns (nil, original ctx) when debug recording should be skipped.
func beginStep(
	ctx context.Context,
	svc *Service,
	opts RecorderOptions,
	op Operation,
	normalizedReq any,
) (*stepHandle, context.Context) {
	if svc == nil {
		return nil, ctx
	}

	rc, ok := RunFromContext(ctx)
	if !ok || rc.RunID == uuid.Nil {
		return nil, ctx
	}

	chatID := opts.ChatID
	if chatID == uuid.Nil {
		chatID = rc.ChatID
	}
	if !svc.IsEnabled(ctx, chatID, opts.OwnerID) {
		return nil, ctx
	}

	holder, reuseStep := reuseHolderFromContext(ctx)
	if reuseStep {
		holder.mu.Lock()
		defer holder.mu.Unlock()
		// Only reuse the cached handle if it belongs to the same run.
		// A different RunContext means a new logical run, so we must
		// create a fresh step to avoid cross-run attribution.
		if holder.handle != nil && holder.handle.stepCtx.RunID == rc.RunID {
			enriched := ContextWithStep(ctx, holder.handle.stepCtx)
			enriched = withAttemptSink(enriched, holder.handle.sink)
			return holder.handle, enriched
		}
	}

	stepNum := nextStepNumber(rc.RunID)
	step, err := svc.CreateStep(ctx, CreateStepParams{
		RunID:               rc.RunID,
		ChatID:              chatID,
		StepNumber:          stepNum,
		Operation:           op,
		Status:              StatusInProgress,
		HistoryTipMessageID: rc.HistoryTipMessageID,
		NormalizedRequest:   normalizedReq,
	})
	if err != nil {
		svc.log.Warn(ctx, "failed to create chat debug step",
			slog.Error(err),
			slog.F("chat_id", chatID),
			slog.F("run_id", rc.RunID),
			slog.F("operation", op),
		)
		return nil, ctx
	}

	syncStepCounter(rc.RunID, step.StepNumber)
	actualStepNumber := step.StepNumber
	if actualStepNumber == 0 {
		actualStepNumber = stepNum
	}

	sc := &StepContext{
		StepID:              step.ID,
		RunID:               rc.RunID,
		ChatID:              chatID,
		StepNumber:          actualStepNumber,
		Operation:           op,
		HistoryTipMessageID: rc.HistoryTipMessageID,
	}
	handle := &stepHandle{stepCtx: sc, sink: &attemptSink{}, svc: svc, opts: opts}
	enriched := ContextWithStep(ctx, handle.stepCtx)
	enriched = withAttemptSink(enriched, handle.sink)
	if reuseStep {
		holder.handle = handle
	}

	return handle, enriched
}

// finish updates the debug step with final status and data.
// sync.Once prevents data races when concurrent callers (e.g.
// retried stream wrappers sharing a reuse handle) both attempt
// to finalize the same step. Only the first finish call takes
// effect.
func (h *stepHandle) finish(
	ctx context.Context,
	status Status,
	response any,
	usage any,
	errPayload any,
	metadata any,
) {
	if h == nil || h.stepCtx == nil {
		return
	}

	h.once.Do(func() {
		h.mu.Lock()
		h.status = status
		h.response = response
		h.usage = usage
		h.err = errPayload
		h.metadata = metadata
		h.mu.Unlock()
		if h.svc == nil {
			return
		}

		updateCtx, cancel := stepFinalizeContext(ctx)
		defer cancel()

		if _, updateErr := h.svc.UpdateStep(updateCtx, UpdateStepParams{
			ID:                 h.stepCtx.StepID,
			ChatID:             h.stepCtx.ChatID,
			Status:             status,
			NormalizedResponse: response,
			Usage:              usage,
			Attempts:           h.sink.snapshot(),
			Error:              errPayload,
			Metadata:           metadata,
			FinishedAt:         time.Now(),
		}); updateErr != nil {
			h.svc.log.Warn(updateCtx, "failed to finalize chat debug step",
				slog.Error(updateErr),
				slog.F("step_id", h.stepCtx.StepID),
				slog.F("chat_id", h.stepCtx.ChatID),
				slog.F("status", status),
			)
		}
	})
}
