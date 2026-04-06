package chatd

import (
	"strings"
	"unicode"
)

// SanitizePromptText strips invisible Unicode characters that could
// hide prompt-injection content from human reviewers, normalizes line
// endings, collapses excessive blank lines, and trims surrounding
// whitespace.
//
// The stripped codepoints are truly invisible and have no legitimate
// use in prompt text. An explicit codepoint list is used rather than
// blanket unicode.Cf stripping to avoid breaking subdivision flag
// emoji (🏴󠁧󠁢󠁥󠁮󠁧󠁿) and other legitimate format characters.
//
// Note: U+200D (ZWJ) is stripped even though it joins compound emoji
// (e.g. 👨‍👩‍👦 → 👨👩👦). This is an acceptable trade-off because
// system prompts are not emoji art, and ZWJ is actively exploited in
// zero-width steganography schemes as a delimiter character.
func SanitizePromptText(s string) string {
	// 1. Normalize line endings: \r\n → \n, lone \r → \n.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// 2. Strip invisible characters rune-by-rune.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isVisible(r) {
			continue
		}
		_, _ = b.WriteRune(r)
	}
	s = b.String()

	// 3. Collapse 3+ consecutive newlines down to 2 (one blank
	//    line between paragraphs). This runs after invisible-char
	//    stripping so that lines containing only stripped chars
	//    become empty and get collapsed.
	s = collapseNewlines(s)

	// 4. Final trim.
	return strings.TrimSpace(s)
}

// isVisible reports whether r is a visible Unicode character that
// should be preserved in prompt text. Each invisible range is
// documented with its Unicode name and rationale.
func isVisible(r rune) bool {
	switch {
	// Soft hyphen — invisible in most renderers, used to hide
	// content boundaries.
	case r == 0x00AD:
		return false

	// Combining grapheme joiner — invisible, no legitimate
	// prompt use.
	case r == 0x034F:
		return false

	// Arabic letter mark — bidi control, invisible.
	case r == 0x061C:
		return false

	// Mongolian vowel separator — invisible spacing character.
	case r == 0x180E:
		return false

	// Zero-width space (U+200B).
	case r == 0x200B:
		return false

	// U+200C (ZWNJ) is deliberately NOT stripped. It is
	// required for correct rendering of Persian, Urdu, and
	// Kurdish scripts where it controls cursive joining.
	// Stripping ZWS (U+200B) and ZWJ (U+200D) already breaks
	// zero-width steganography encodings regardless of whether
	// ZWNJ survives.

	// Zero-width joiner (U+200D) — also used in compound emoji,
	// but actively exploited in steganography. See
	// SanitizePromptText doc comment.
	case r == 0x200D:
		return false

	// Left-to-right mark (U+200E).
	case r == 0x200E:
		return false

	// Right-to-left mark (U+200F).
	case r == 0x200F:
		return false

	// Bidi embedding and override controls (U+202A–U+202E):
	// LRE, RLE, PDF, LRO, RLO.
	case r >= 0x202A && r <= 0x202E:
		return false

	// Word joiner and invisible operators (U+2060–U+2064):
	// word joiner, function application, invisible times,
	// invisible separator, invisible plus.
	case r >= 0x2060 && r <= 0x2064:
		return false

	// Bidi isolate controls (U+2066–U+2069):
	// LRI, RLI, FSI, PDI.
	case r >= 0x2066 && r <= 0x2069:
		return false

	// Deprecated format characters (U+206A–U+206F): inhibit
	// symmetric swapping through nominal digit shapes.
	case r >= 0x206A && r <= 0x206F:
		return false

	// Byte order mark / zero-width no-break space (U+FEFF).
	// Common at start of Windows-edited files.
	case r == 0xFEFF:
		return false

	// Interlinear annotation anchor, separator, and
	// terminator (U+FFF9–U+FFFB).
	case r >= 0xFFF9 && r <= 0xFFFB:
		return false

	default:
		return true
	}
}

// collapseNewlines replaces runs of 3 or more consecutive newlines
// with exactly 2, preserving single blank lines (paragraph breaks)
// while eliminating scroll-padding attacks. Trailing whitespace on
// each line is stripped first so that whitespace-only lines become
// empty and collapse naturally.
func collapseNewlines(s string) string {
	// Step 1: Trim trailing whitespace from each line, preserving
	// leading whitespace for indentation.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	s = strings.Join(lines, "\n")

	// Step 2: Collapse runs of 3+ consecutive newlines down to 2.
	var b strings.Builder
	b.Grow(len(s))
	consecutiveNewlines := 0
	for _, r := range s {
		if r == '\n' {
			consecutiveNewlines++
			if consecutiveNewlines <= 2 {
				_, _ = b.WriteRune(r)
			}
			continue
		}
		consecutiveNewlines = 0
		_, _ = b.WriteRune(r)
	}
	return b.String()
}
