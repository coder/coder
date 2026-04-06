package chatdebug

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// This branch-03 compatibility shim forward-declares service and summary
// symbols that land in later stacked branches. Delete this file once
// service.go and summary.go are available here.

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

// NewService constructs the branch-03 placeholder chat debug service.
func NewService(_ database.Store, log slog.Logger, _ pubsub.Pubsub) *Service {
	return &Service{log: log}
}

// IsEnabled reports whether debug recording is enabled for a chat owner.
// The branch-03 recorder tests exercise wrapper behavior before the real
// persistence service lands, so the placeholder service opts in whenever a
// recorder is present.
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
