package cli

import (
	"regexp"
	"strings"
	"unicode"
)

var terminalEscapeSequenceRegexp = regexp.MustCompile(
	`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]|` +
		"" + `[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]|` +
		`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|` +
		"" + `[^\x07\x1b]*(?:\x07|\x1b\\)|` +
		`\x1b[^\[\]].`,
)

func sanitizeTerminalRenderableText(text string) string {
	if text == "" {
		return ""
	}

	text = terminalEscapeSequenceRegexp.ReplaceAllString(text, "")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}
