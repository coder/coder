package chatd

import (
	"regexp"
	"strings"
	"unicode"
)

// htmlCommentRe matches HTML comments, including comments that span
// multiple lines. A hidden <!-- ... --> in an untrusted instruction
// or skill file would otherwise survive into the prompt where a human
// reviewer cannot see it.
var htmlCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)

// SanitizePromptText strips HTML comments and invisible Unicode
// characters that could hide prompt-injection content from human
// reviewers, normalizes line endings, collapses excessive blank
// lines, and trims surrounding whitespace.
//
// The stripped codepoints are invisible or non-rendering and have no
// legitimate use in prompt text. An explicit codepoint list is used
// rather than blanket unicode.Cf stripping so that format characters
// with real linguistic value, such as U+200C (ZWNJ) for Persian,
// Urdu, and Kurdish, are preserved. Known steganography and
// ASCII-smuggling channels, including the variation selectors and the
// Unicode Tags block, are stripped even though that decomposes
// subdivision flag emoji, because prompt text is not emoji art.
//
// Note: U+200D (ZWJ) is stripped even though it joins compound emoji
// (e.g. 👨‍👩‍👦 → 👨👩👦). This is an acceptable trade-off because
// system prompts are not emoji art, and ZWJ is actively exploited in
// zero-width steganography schemes as a delimiter character.
func SanitizePromptText(s string) string {
	// 1. Normalize line endings: \r\n → \n, lone \r → \n.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// 2. Strip HTML comments, including multi-line comments. A
	//    hidden <!-- ... --> in an untrusted instruction or skill
	//    file would otherwise survive into the prompt unseen by a
	//    human reviewer. This runs before invisible-rune stripping
	//    so the comment markers are matched as written.
	s = htmlCommentRe.ReplaceAllString(s, "")

	// 3. Strip invisible characters rune-by-rune.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isVisible(r) {
			continue
		}
		_, _ = b.WriteRune(r)
	}
	s = b.String()

	// 4. Collapse 3+ consecutive newlines down to 2 (one blank
	//    line between paragraphs). This runs after invisible-char
	//    stripping so that lines containing only stripped chars
	//    become empty and get collapsed.
	s = collapseNewlines(s)

	// 5. Final trim.
	return strings.TrimSpace(s)
}

// isVisible reports whether r is a visible Unicode character that
// should be preserved in prompt text. Each invisible range is
// documented with its Unicode name and rationale.
func isVisible(r rune) bool {
	switch {
	// Soft hyphen. Invisible in most renderers, used to hide
	// content boundaries.
	case r == 0x00AD:
		return false

	// Combining grapheme joiner. Invisible, no legitimate prompt
	// use.
	case r == 0x034F:
		return false

	// Arabic letter mark. Bidi control, invisible.
	case r == 0x061C:
		return false

	// Syriac abbreviation mark. Format control with no visible
	// glyph of its own.
	case r == 0x070F:
		return false

	// Hangul choseong filler and jungseong filler. Render as blank
	// and are used to fake empty or invisible text.
	case r == 0x115F, r == 0x1160:
		return false

	// Khmer inherent vowels AQ and AA. Non-spacing marks abused as
	// invisible characters outside Khmer text.
	case r >= 0x17B4 && r <= 0x17B5:
		return false

	// Mongolian free variation selectors one through three. Format
	// controls with no rendering of their own.
	case r >= 0x180B && r <= 0x180D:
		return false

	// Mongolian vowel separator. Invisible spacing character.
	case r == 0x180E:
		return false

	// Mongolian free variation selector four. Format control with
	// no rendering of its own.
	case r == 0x180F:
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

	// Zero-width joiner (U+200D). Also used in compound emoji,
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

	// Bidi embedding and override controls (U+202A-U+202E):
	// LRE, RLE, PDF, LRO, RLO.
	case r >= 0x202A && r <= 0x202E:
		return false

	// Invisible operators, bidi isolates, and deprecated format
	// characters (U+2060-U+206F): word joiner, function
	// application, invisible times, invisible separator, invisible
	// plus, the unassigned U+2065, the bidi isolate controls
	// (LRI, RLI, FSI, PDI), and the deprecated symmetric-swapping
	// and digit-shape controls. The range is kept contiguous so
	// the unassigned U+2065 cannot be used as a gap.
	case r >= 0x2060 && r <= 0x206F:
		return false

	// Hangul filler. Renders as blank and is used to fake
	// invisible text.
	case r == 0x3164:
		return false

	// Variation selectors (U+FE00-U+FE0F). The whole block is
	// stripped to close a steganography channel that hides data in
	// selector sequences. U+FE0F is the emoji presentation
	// selector and is intentionally stripped for prompt text:
	// prompts are not emoji art, and parity across the block
	// matters more than emoji styling.
	case r >= 0xFE00 && r <= 0xFE0F:
		return false

	// Byte order mark / zero-width no-break space (U+FEFF).
	// Common at start of Windows-edited files.
	case r == 0xFEFF:
		return false

	// Halfwidth Hangul filler. Renders as blank and is used to
	// fake invisible text.
	case r == 0xFFA0:
		return false

	// Reserved slots in the Specials block (U+FFF0-U+FFF8). These
	// are unassigned format codepoints with no visible rendering.
	case r >= 0xFFF0 && r <= 0xFFF8:
		return false

	// Interlinear annotation anchor, separator, and
	// terminator (U+FFF9-U+FFFB).
	case r >= 0xFFF9 && r <= 0xFFFB:
		return false

	// Unicode Tags block (U+E0000-U+E007F). These mirror ASCII as
	// invisible characters and are the highest-signal channel for
	// smuggling hidden instructions into prompt text. Stripping
	// them decomposes subdivision flag emoji, which is an accepted
	// trade-off for prompt text.
	case r >= 0xE0000 && r <= 0xE007F:
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
