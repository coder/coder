package chatd

import (
	"strings"
	"unicode/utf8"
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
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if isInvisibleRune(r) {
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

// isInvisibleRune reports whether r is an invisible Unicode character
// that should be stripped from prompt text. Each range is documented
// with its Unicode name and rationale.
func isInvisibleRune(r rune) bool {
	switch {
	// Soft hyphen — invisible in most renderers, used to hide
	// content boundaries.
	case r == 0x00AD:
		return true

	// Combining grapheme joiner — invisible, no legitimate
	// prompt use.
	case r == 0x034F:
		return true

	// Arabic letter mark — bidi control, invisible.
	case r == 0x061C:
		return true

	// Mongolian vowel separator — invisible spacing character.
	case r == 0x180E:
		return true

	// Zero-width space (U+200B).
	case r == 0x200B:
		return true

	// Zero-width non-joiner (U+200C).
	case r == 0x200C:
		return true

	// Zero-width joiner (U+200D) — also used in compound emoji,
	// but actively exploited in steganography. See package doc.
	case r == 0x200D:
		return true

	// Left-to-right mark (U+200E).
	case r == 0x200E:
		return true

	// Right-to-left mark (U+200F).
	case r == 0x200F:
		return true

	// Bidi embedding and override controls (U+202A–U+202E):
	// LRE, RLE, PDF, LRO, RLO.
	case r >= 0x202A && r <= 0x202E:
		return true

	// Word joiner and invisible operators (U+2060–U+2064):
	// word joiner, function application, invisible times,
	// invisible separator, invisible plus.
	case r >= 0x2060 && r <= 0x2064:
		return true

	// Bidi isolate controls (U+2066–U+2069):
	// LRI, RLI, FSI, PDI.
	case r >= 0x2066 && r <= 0x2069:
		return true

	// Byte order mark / zero-width no-break space (U+FEFF).
	// Common at start of Windows-edited files.
	case r == 0xFEFF:
		return true

	default:
		return false
	}
}

// collapseNewlines replaces runs of 3 or more consecutive newlines
// with exactly 2, preserving single blank lines (paragraph breaks)
// while eliminating scroll-padding attacks.
func collapseNewlines(s string) string {
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
