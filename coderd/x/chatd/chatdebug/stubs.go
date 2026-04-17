package chatdebug

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// This compatibility shim forward-declares service and summary symbols
// that land in later stacked branches. Delete this file once service.go
// and summary.go are available here.

// Service is a placeholder for the later chat debug persistence service.
type Service struct {
	log slog.Logger
}

// CreateStepParams identifies the data recorded when a debug step starts.
type CreateStepParams struct {
	RunID               uuid.UUID
	ChatID              uuid.UUID
	StepNumber          int32
	Operation           Operation
	Status              Status
	HistoryTipMessageID int64
	NormalizedRequest   any
}

// UpdateStepParams identifies the data recorded when a debug step finishes.
type UpdateStepParams struct {
	ID                 uuid.UUID
	ChatID             uuid.UUID
	Status             Status
	NormalizedResponse any
	Usage              any
	Attempts           []Attempt
	Error              any
	Metadata           any
	FinishedAt         time.Time
}

// NewService constructs the placeholder chat debug service.
func NewService(_ database.Store, log slog.Logger, _ pubsub.Pubsub) *Service {
	return &Service{log: log}
}

// IsEnabled reports whether debug recording is enabled for a chat owner.
func (*Service) IsEnabled(context.Context, uuid.UUID, uuid.UUID) bool {
	return true
}

// CreateStep synthesizes a debug step so recorder tests can exercise the
// wrapper without requiring the later persistence service implementation.
func (*Service) CreateStep(
	_ context.Context,
	params CreateStepParams,
) (database.ChatDebugStep, error) {
	return database.ChatDebugStep{
		ID:         uuid.New(),
		RunID:      params.RunID,
		ChatID:     params.ChatID,
		StepNumber: params.StepNumber,
		Operation:  string(params.Operation),
		Status:     string(params.Status),
	}, nil
}

// UpdateStep accepts final step state once recording completes.
func (*Service) UpdateStep(
	_ context.Context,
	params UpdateStepParams,
) (database.ChatDebugStep, error) {
	return database.ChatDebugStep{
		ID:     params.ID,
		ChatID: params.ChatID,
		Status: string(params.Status),
	}, nil
}

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
