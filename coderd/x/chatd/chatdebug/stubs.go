package chatdebug

import (
	"context"
	"regexp"
	"strings"
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

func attemptSinkFromContext(_ context.Context) *attemptSink {
	return nil
}

type stepHandle struct{}

func beginStep(
	ctx context.Context,
	_ *Service,
	_ RecorderOptions,
	_ Operation,
	_ any,
) (*stepHandle, context.Context) {
	if holder, ok := reuseHolderFromContext(ctx); ok {
		holder.mu.Lock()
		_ = holder.handle
		holder.mu.Unlock()
	}
	return nil, ctx
}

func (*stepHandle) finish(
	_ context.Context,
	_ Status,
	_ any,
	_ any,
	_ any,
	_ any,
) {
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
	if normalized == "" {
		return ""
	}

	if utf8.RuneCountInString(normalized) <= maxLen {
		return normalized
	}

	// Truncate at maxLen runes and append ellipsis.
	runes := []rune(normalized)
	return string(runes[:maxLen]) + "…"
}
