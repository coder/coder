package chatdebug

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

// whitespaceRun matches one or more consecutive whitespace characters.
var whitespaceRun = regexp.MustCompile(`\s+`)

// TruncateLabel whitespace-normalizes and truncates text to maxLen runes.
// Returns "" if input is empty or whitespace-only.
func TruncateLabel(text string, maxLen int) string {
	normalized := strings.TrimSpace(whitespaceRun.ReplaceAllString(text, " "))
	if normalized == "" {
		return ""
	}
	return stringutil.Truncate(normalized, maxLen, stringutil.TruncateWithEllipsis)
}

// SeedSummary builds a base summary map with a first_message label.
// Returns nil if label is empty.
func SeedSummary(label string) map[string]any {
	if label == "" {
		return nil
	}
	return map[string]any{"first_message": label}
}

// ExtractFirstUserText extracts the plain text content from a
// fantasy.Prompt for the first user message. Used to derive
// first_message labels at run creation time.
func ExtractFirstUserText(prompt fantasy.Prompt) string {
	for _, msg := range prompt {
		if msg.Role != fantasy.MessageRoleUser {
			continue
		}

		var sb strings.Builder
		for _, part := range msg.Content {
			tp, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if !ok {
				continue
			}
			_, _ = sb.WriteString(tp.Text)
		}
		return sb.String()
	}
	return ""
}

// AggregateRunSummary reads all steps for the given run, computes token
// totals, and merges them with the run's existing summary (preserving any
// seeded first_message label). The baseSummary parameter should be the
// current run summary (may be nil).
func (s *Service) AggregateRunSummary(
	ctx context.Context,
	runID uuid.UUID,
	baseSummary map[string]any,
) (map[string]any, error) {
	if runID == uuid.Nil {
		return baseSummary, nil
	}

	steps, err := s.db.GetChatDebugStepsByRunID(chatdContext(ctx), runID)
	if err != nil {
		return nil, err
	}

	// Start from a shallow copy of baseSummary to avoid mutating the
	// caller's map.
	// Capacity hint: baseSummary entries plus 8 derived keys
	// (step_count, total_input_tokens, total_output_tokens,
	// total_reasoning_tokens, total_cache_creation_tokens,
	// total_cache_read_tokens, has_error, endpoint_label).
	result := make(map[string]any, len(baseSummary)+8)
	for k, v := range baseSummary {
		result[k] = v
	}

	// Clear derived fields before recomputing them so stale values from a
	// previous aggregation do not survive when the new totals are zero or
	// the endpoint label is unavailable.
	for _, key := range []string{
		"step_count",
		"total_input_tokens",
		"total_output_tokens",
		"total_reasoning_tokens",
		"total_cache_creation_tokens",
		"total_cache_read_tokens",
		"endpoint_label",
		"has_error",
	} {
		delete(result, key)
	}
	var (
		totalInput         int64
		totalOutput        int64
		totalReasoning     int64
		totalCacheCreation int64
		totalCacheRead     int64
		hasError           bool
	)

	for _, step := range steps {
		// Flag runs that hit a real error.  Interrupted steps represent
		// user-initiated cancellation (e.g. clicking Stop) and should
		// not trigger the error indicator in the debug panel.
		// A JSONB null (used by jsonClear to erase a prior error) is
		// Valid but carries no meaningful content, so exclude it.
		errorIsReal := step.Error.Valid &&
			len(step.Error.RawMessage) > 0 &&
			!bytes.Equal(step.Error.RawMessage, []byte("null"))
		if step.Status == string(StatusError) ||
			(errorIsReal && step.Status != string(StatusInterrupted)) {
			hasError = true
		}
		if !step.Usage.Valid || len(step.Usage.RawMessage) == 0 {
			continue
		}

		var usage fantasy.Usage
		if err := json.Unmarshal(step.Usage.RawMessage, &usage); err != nil {
			s.log.Warn(ctx, "skipping malformed step usage JSON",
				slog.Error(err),
				slog.F("run_id", runID),
				slog.F("step_id", step.ID),
			)
			continue
		}

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalReasoning += usage.ReasoningTokens
		totalCacheCreation += usage.CacheCreationTokens
		totalCacheRead += usage.CacheReadTokens
	}

	result["step_count"] = len(steps)
	result["total_input_tokens"] = totalInput
	result["total_output_tokens"] = totalOutput

	// Only include reasoning/cache fields when non-zero to keep the
	// summary compact for the common case.
	if totalReasoning > 0 {
		result["total_reasoning_tokens"] = totalReasoning
	}
	if totalCacheCreation > 0 {
		result["total_cache_creation_tokens"] = totalCacheCreation
	}
	if totalCacheRead > 0 {
		result["total_cache_read_tokens"] = totalCacheRead
	}

	if hasError {
		result["has_error"] = true
	}

	// Derive endpoint_label from the first completed attempt's path
	// across all steps. This gives the debug panel a meaningful
	// identifier like "POST /v1/messages" for the run row.
	if label := extractEndpointLabel(steps); label != "" {
		result["endpoint_label"] = label
	}

	return result, nil
}

// attemptLabel is a minimal projection of Attempt used by
// extractEndpointLabel to avoid deserializing large RequestBody and
// ResponseBody fields that are not needed for label derivation.
type attemptLabel struct {
	Status string `json:"status,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// extractEndpointLabel scans steps for the first completed attempt with a
// non-empty path and returns "METHOD /path" (or just "/path").
func extractEndpointLabel(steps []database.ChatDebugStep) string {
	for _, step := range steps {
		if len(step.Attempts) == 0 {
			continue
		}
		var attempts []attemptLabel
		if err := json.Unmarshal(step.Attempts, &attempts); err != nil {
			continue
		}
		for _, a := range attempts {
			if a.Status != attemptStatusCompleted || a.Path == "" {
				continue
			}
			if a.Method != "" {
				return a.Method + " " + a.Path
			}
			return a.Path
		}
	}
	return ""
}
