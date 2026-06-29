package chatloop

import (
	"fmt"
	"strings"
)

const (
	// toolResultContextDivisor bounds how much of a model's context
	// window a single tool result may occupy: at most 1/3 of the
	// window. This caps a single oversized result (most often a large
	// MCP response) so it cannot overflow the prompt on its own, while
	// still letting a useful amount of output through. The divisor is
	// deliberately larger than 2 so the absolute cap stays sane on
	// large-context (e.g. 1M-token) models, where a more generous
	// fraction would still admit multi-megabyte results. Cumulative
	// growth across many results is handled separately by context
	// compaction.
	toolResultContextDivisor = 3

	// bytesPerTokenEstimate converts a token budget into a byte budget.
	// Tool output is capped before tokenization, so this is a coarse,
	// provider-agnostic estimate. It is deliberately conservative at ~3
	// bytes per token: dense payloads (JSON, logs, code, non-ASCII) run
	// well below 4 bytes per token, so the lower estimate yields a
	// smaller byte budget that is less likely to underestimate the true
	// token cost.
	bytesPerTokenEstimate = 3

	// minToolResultBytes is the floor for the per-result byte budget so
	// small or unknown context windows still let useful output through.
	minToolResultBytes = 16 << 10 // 16KB

	// defaultToolResultBytes is the budget used when the model's context
	// window is unknown (context limit <= 0).
	defaultToolResultBytes = 64 << 10 // 64KB

	// truncationMarkerReserve is the number of bytes reserved for the
	// truncation marker when splitting output into a head and tail. It
	// is comfortably larger than the longest marker
	// truncateToolResultText can produce, so the assembled result never
	// exceeds the budget.
	truncationMarkerReserve = 256
)

// maxIntValue is the largest value of the platform int type.
const maxIntValue = int(^uint(0) >> 1)

// toolResultByteBudget converts a model context-window size (in
// tokens) into the maximum number of bytes a single tool result may
// contribute to the prompt. A context limit <= 0 means the window is
// unknown and the default budget is used. The result is never below
// minToolResultBytes.
func toolResultByteBudget(contextLimitTokens int64) int {
	if contextLimitTokens <= 0 {
		return defaultToolResultBytes
	}
	budgetBytes := contextLimitTokens / toolResultContextDivisor * bytesPerTokenEstimate
	if budgetBytes < minToolResultBytes {
		return minToolResultBytes
	}
	// Clamp to the int range for 32-bit safety on very large windows.
	if budgetBytes > int64(maxIntValue) {
		return maxIntValue
	}
	return int(budgetBytes)
}

// truncateToolResultText caps text to at most maxBytes using a
// head-and-tail strategy: it keeps the start and end of the output and
// replaces the middle with a marker noting how many bytes were
// removed. This preserves the most useful context (a tool's leading
// summary and trailing status) while bounding size. The returned
// string is always valid UTF-8 and never exceeds maxBytes. It returns
// (text, false) unchanged when maxBytes <= 0 or the text already fits.
func truncateToolResultText(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, false
	}

	// When the budget is too small to fit the marker plus a meaningful
	// head and tail, hard-cut the head and drop any partial trailing
	// rune.
	if maxBytes <= truncationMarkerReserve*2 {
		return strings.ToValidUTF8(text[:maxBytes], ""), true
	}

	avail := maxBytes - truncationMarkerReserve
	head := avail * 2 / 3
	tail := avail - head

	// ToValidUTF8 with an empty replacement drops invalid byte runs, so
	// a cut that lands in the middle of a multi-byte rune is discarded
	// rather than corrupting the output. This can only shrink the
	// slices, so the assembled result stays within budget.
	headStr := strings.ToValidUTF8(text[:head], "")
	tailStr := strings.ToValidUTF8(text[len(text)-tail:], "")
	removed := len(text) - len(headStr) - len(tailStr)

	marker := fmt.Sprintf(
		"\n\n[... Coder truncated %d bytes to fit the model context; "+
			"narrow the query or read a specific file or range for the "+
			"full output ...]\n\n",
		removed,
	)
	return headStr + marker + tailStr, true
}
