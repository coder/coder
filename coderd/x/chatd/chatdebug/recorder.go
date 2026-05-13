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

// replaceByNumber overwrites a previously recorded attempt whose Number
// matches. If no match is found, the attempt is appended. This supports
// the provisional-then-upgrade flow used for SSE bodies where Read()
// records a completed attempt on EOF and Close() later needs to replace
// it with a failed attempt when inner.Close() surfaces an error.
func (s *attemptSink) replaceByNumber(number int, a Attempt) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.attempts {
		if s.attempts[i].Number == number {
			s.attempts[i] = a
			return
		}
	}
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

// runRefCounts tracks how many live RunContext instances reference each
// RunID. Cleanup of shared state (step counters) is deferred until the
// last RunContext for a given RunID is garbage collected.
var (
	runRefCounts sync.Map // map[uuid.UUID]*atomic.Int32
	// refCountMu serializes trackRunRef and releaseRunRef so the
	// decrement-to-zero check and subsequent map deletions are
	// atomic with respect to new references being added.
	refCountMu sync.Mutex
)

func trackRunRef(runID uuid.UUID) {
	refCountMu.Lock()
	defer refCountMu.Unlock()
	val, _ := runRefCounts.LoadOrStore(runID, &atomic.Int32{})
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: runRefCounts contains non-*atomic.Int32 value")
	}
	counter.Add(1)
}

// releaseRunRef decrements the reference count for runID and cleans up
// shared state when the last reference is released. The mutex ensures
// no concurrent trackRunRef can increment between the zero check and
// the map deletions.
func releaseRunRef(runID uuid.UUID) {
	refCountMu.Lock()
	defer refCountMu.Unlock()
	val, ok := runRefCounts.Load(runID)
	if !ok {
		return
	}
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: runRefCounts contains non-*atomic.Int32 value")
	}
	if counter.Add(-1) <= 0 {
		runRefCounts.Delete(runID)
		stepCounters.Delete(runID)
	}
}

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
	mu       sync.Mutex
	status   Status
	response any
	usage    any
	err      any
	metadata any
	// hadError tracks whether a prior finalization wrote an error
	// payload. Used to decide whether a successful retry needs to
	// explicitly clear the error field via jsonClear.
	hadError bool
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

// finish updates the debug step with final status and data.  A mutex
// guards the write so concurrent callers (e.g. retried stream wrappers
// sharing a reuse handle) don't race.  Later retries are allowed to
// overwrite earlier failure results so the step reflects the final
// outcome, but stale callbacks cannot regress a terminal state.
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

	h.mu.Lock()
	defer h.mu.Unlock()

	// Reject stale callbacks that would regress a terminal state.
	// Status priority: in_progress < interrupted < error < completed.
	// A tardy safety-net writing "interrupted" cannot clobber a step
	// that already reached "completed" or "error" from a real retry.
	// Equal-priority updates are allowed so that retries ending in the
	// same terminal class (e.g. error → error under ReuseStep) can
	// still update the step with newer attempt data.
	if h.status.IsTerminal() && status.Priority() < h.status.Priority() {
		return
	}

	h.status = status
	h.response = response
	h.usage = usage
	h.err = errPayload
	h.metadata = metadata
	if errPayload != nil {
		h.hadError = true
	}
	if h.svc == nil {
		return
	}

	updateCtx, cancel := stepFinalizeContext(ctx)
	defer cancel()

	// When the step completes successfully after a prior failed
	// attempt, the error field must be explicitly cleared.  A plain
	// nil would leave the COALESCE-based SQL untouched, so we send
	// jsonClear{} which serializes as a valid JSONB null.  Only do
	// this when a prior error was actually recorded; otherwise
	// clean successes would get a spurious JSONB null that downstream
	// aggregation could misread as an error.
	errValue := errPayload
	if errValue == nil && status == StatusCompleted && h.hadError {
		errValue = jsonClear{}
	}

	if _, updateErr := h.svc.UpdateStep(updateCtx, UpdateStepParams{
		ID:                 h.stepCtx.StepID,
		ChatID:             h.stepCtx.ChatID,
		Status:             status,
		NormalizedResponse: response,
		Usage:              usage,
		Attempts:           h.sink.snapshot(),
		Error:              errValue,
		Metadata:           metadata,
		FinishedAt:         h.svc.clock.Now(),
	}); updateErr != nil {
		h.svc.log.Warn(updateCtx, "failed to finalize chat debug step",
			slog.Error(updateErr),
			slog.F("step_id", h.stepCtx.StepID),
			slog.F("chat_id", h.stepCtx.ChatID),
			slog.F("status", status),
		)
	}
}
