package chatdebug

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// This branch-02 compatibility shim forward-declares recorder, service,
// and summary symbols that land in later stacked branches. Delete this
// file once recorder.go, service.go, and summary.go are available here.

// RecorderOptions identifies the chat/model context for debug recording.
type RecorderOptions struct {
	ChatID   uuid.UUID
	OwnerID  uuid.UUID
	Provider string
	Model    string
}

// Service is a placeholder for the later chat debug persistence service.
type Service struct{}

// NewService constructs the branch-02 placeholder chat debug service.
func NewService(_ database.Store, _ slog.Logger, _ pubsub.Pubsub) *Service {
	return &Service{}
}

type attemptSink struct{}

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
var runRefCounts sync.Map // map[uuid.UUID]*atomic.Int32

func trackRunRef(runID uuid.UUID) {
	val, _ := runRefCounts.LoadOrStore(runID, &atomic.Int32{})
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: runRefCounts contains non-*atomic.Int32 value")
	}
	counter.Add(1)
}

// releaseRunRef decrements the reference count for runID and cleans up
// shared state when the last reference is released. The cleanup uses
// CompareAndSwap to ensure that a concurrent trackRunRef that
// incremented the counter between our Add and Delete does not lose
// its reference.
func releaseRunRef(runID uuid.UUID) {
	val, ok := runRefCounts.Load(runID)
	if !ok {
		return
	}
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: runRefCounts contains non-*atomic.Int32 value")
	}
	newVal := counter.Add(-1)
	if newVal > 0 {
		return
	}
	// Attempt to claim the zero state. If another goroutine already
	// incremented the counter (trackRunRef raced with us), the CAS
	// fails and we leave the entries alone.
	if !counter.CompareAndSwap(newVal, newVal) {
		return
	}
	// Only delete if the map still points to our counter instance.
	// A concurrent LoadOrStore in trackRunRef may have replaced it.
	runRefCounts.CompareAndDelete(runID, val)
	stepCounters.Delete(runID)
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

type stepHandle struct {
	stepCtx  *StepContext
	sink     *attemptSink
	mu       sync.Mutex
	status   Status
	response any
	usage    any
	err      any
	metadata any
}

func beginStep(
	ctx context.Context,
	svc *Service,
	opts RecorderOptions,
	op Operation,
	_ any,
) (*stepHandle, context.Context) {
	if svc == nil {
		return nil, ctx
	}

	rc, ok := RunFromContext(ctx)
	if !ok || rc.RunID == uuid.Nil {
		return nil, ctx
	}

	if holder, reuseStep := reuseHolderFromContext(ctx); reuseStep {
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

		handle, enriched := newStepHandle(ctx, rc, opts, op)
		holder.handle = handle
		return handle, enriched
	}

	return newStepHandle(ctx, rc, opts, op)
}

func newStepHandle(
	ctx context.Context,
	rc *RunContext,
	opts RecorderOptions,
	op Operation,
) (*stepHandle, context.Context) {
	if rc == nil || rc.RunID == uuid.Nil {
		return nil, ctx
	}

	chatID := opts.ChatID
	if chatID == uuid.Nil {
		chatID = rc.ChatID
	}

	handle := &stepHandle{
		stepCtx: &StepContext{
			StepID:              uuid.New(),
			RunID:               rc.RunID,
			ChatID:              chatID,
			StepNumber:          nextStepNumber(rc.RunID),
			Operation:           op,
			HistoryTipMessageID: rc.HistoryTipMessageID,
		},
		sink: &attemptSink{},
	}
	enriched := ContextWithStep(ctx, handle.stepCtx)
	enriched = withAttemptSink(enriched, handle.sink)
	return handle, enriched
}

func (h *stepHandle) finish(
	_ context.Context,
	status Status,
	response any,
	usage any,
	err any,
	metadata any,
) {
	if h == nil || h.stepCtx == nil {
		return
	}
	// Guard with a mutex so concurrent callers (e.g. retried stream
	// wrappers sharing a reused handle) don't race. Unlike sync.Once,
	// later retries are allowed to overwrite earlier failure results so
	// the step reflects the final outcome.
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = status
	h.response = response
	h.usage = usage
	h.err = err
	h.metadata = metadata
}

// whitespaceRun matches one or more consecutive whitespace characters.
var whitespaceRun = regexp.MustCompile(`\s+`)

// truncateRunes truncates s to maxLen runes, appending an ellipsis
// when truncation occurs. Returns "" when maxLen <= 0.
func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}

// TruncateLabel whitespace-normalizes and truncates text to maxLen runes.
// Returns "" if input is empty or whitespace-only.
func TruncateLabel(text string, maxLen int) string {
	normalized := strings.TrimSpace(whitespaceRun.ReplaceAllString(text, " "))
	if normalized == "" {
		return ""
	}
	return truncateRunes(normalized, maxLen)
}
