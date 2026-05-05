package chatdebug

import (
	"regexp"
	"strings"

	"charm.land/fantasy"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

// MaxLabelLength is the maximum number of runes kept when building
// first_message labels for debug run summaries.
const MaxLabelLength = 200

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
