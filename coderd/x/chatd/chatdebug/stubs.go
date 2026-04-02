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

func nextStepNumber(runID uuid.UUID) int32 {
	val, _ := stepCounters.LoadOrStore(runID, &atomic.Int32{})
	counter, ok := val.(*atomic.Int32)
	if !ok {
		panic("chatdebug: invalid step counter type")
	}
	return counter.Add(1)
}

// CleanupStepCounter removes per-run step counter state. This is used by
// branch-02 tests and later stacked branches that have a real run lifecycle.
func CleanupStepCounter(runID uuid.UUID) {
	stepCounters.Delete(runID)
}

type stepHandle struct {
	stepCtx *StepContext
	sink    *attemptSink
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
	if !ok {
		return nil, ctx
	}

	if holder, reuseStep := reuseHolderFromContext(ctx); reuseStep {
		holder.mu.Lock()
		defer holder.mu.Unlock()
		if holder.handle != nil {
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
	_ Status,
	_ any,
	_ any,
	_ any,
	_ any,
) {
	if h == nil || h.stepCtx == nil {
		return
	}
}

// whitespaceRun matches one or more consecutive whitespace characters.
var whitespaceRun = regexp.MustCompile(`\s+`)

// TruncateLabel whitespace-normalizes and truncates text to maxLen runes.
// Returns "" if input is empty or whitespace-only.
func TruncateLabel(text string, maxLen int) string {
	if maxLen < 0 {
		maxLen = 0
	}

	normalized := strings.TrimSpace(whitespaceRun.ReplaceAllString(text, " "))
	if normalized == "" || maxLen == 0 {
		return ""
	}

	if utf8.RuneCountInString(normalized) <= maxLen {
		return normalized
	}
	if maxLen == 1 {
		return "…"
	}

	// Truncate to leave room for the trailing ellipsis within maxLen.
	runes := []rune(normalized)
	return string(runes[:maxLen-1]) + "…"
}
