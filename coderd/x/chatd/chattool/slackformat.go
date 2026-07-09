package chattool

import (
	"fmt"
	"regexp"
	"strings"
)

// slackMessageMaxLen is Slack's practical limit for a section block's
// text. Longer messages are truncated and the tool result tells the
// model to send a follow-up.
const slackMessageMaxLen = 3000

var (
	reSlackCodeBlock  = regexp.MustCompile("(?s)```.*?```")
	reSlackInlineCode = regexp.MustCompile("`[^`]+`")
	reSlackMDLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reSlackBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reSlackHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reSlackCodeLang   = regexp.MustCompile("(?m)^```[a-zA-Z]+\\s*$")
	// Match @U12345 only when NOT preceded by < (avoids double-wrapping
	// <@U12345>).
	reSlackAtUserID = regexp.MustCompile(`([^<])@([UW](?:[A-Z0-9]*\d)[A-Z0-9]{6,})`)
	// Match bare U12345 only when NOT preceded by < or @ (avoids
	// already-bracketed mentions).
	reSlackBareUserID = regexp.MustCompile(`(^|[^A-Z0-9<@])([UW](?:[A-Z0-9]*\d)[A-Z0-9]{6,})(?:[^A-Z0-9>]|$)`)
	reSlackWrapped    = regexp.MustCompile(`<@([^>]+)>`)
	reSlackRealID     = regexp.MustCompile(`^[UW][A-Z0-9]{7,}$`)
)

// formatSlackMessage normalizes model-produced markdown into Slack
// mrkdwn and enforces the message length limit. It returns the
// formatted text, whether the input was truncated, and the original
// length in bytes.
func formatSlackMessage(text string) (string, bool, int) {
	origLen := len(text)
	truncated := origLen > slackMessageMaxLen
	if truncated {
		text = text[:slackMessageMaxLen]
	}

	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\"`, `"`)

	// Preserve code blocks and inline code from formatting transforms.
	var preserved []string
	ph := func(s string) string {
		idx := len(preserved)
		preserved = append(preserved, s)
		return fmt.Sprintf("\x00CODE%d\x00", idx)
	}
	text = reSlackCodeBlock.ReplaceAllStringFunc(text, ph)
	text = reSlackInlineCode.ReplaceAllStringFunc(text, ph)

	text = reSlackMDLink.ReplaceAllString(text, "<$2|$1>")
	text = reSlackBold.ReplaceAllString(text, "*$1*")
	text = reSlackHeading.ReplaceAllString(text, "*$1*")
	// Wrap @U12345 mentions, preserving the preceding character.
	text = reSlackAtUserID.ReplaceAllString(text, "${1}<@${2}>")
	// Wrap bare U12345 IDs, preserving prefix and suffix.
	text = reSlackBareUserID.ReplaceAllString(text, "${1}<@${2}>")

	// Strip <@handle> brackets when handle is not a real Slack user ID.
	text = reSlackWrapped.ReplaceAllStringFunc(text, func(m string) string {
		sub := reSlackWrapped.FindStringSubmatch(m)
		if len(sub) < 2 || reSlackRealID.MatchString(sub[1]) {
			return m
		}
		return "@" + sub[1]
	})

	for i, code := range preserved {
		cleaned := reSlackCodeLang.ReplaceAllString(code, "```")
		text = strings.Replace(text, fmt.Sprintf("\x00CODE%d\x00", i), cleaned, 1)
	}

	return text, truncated, origLen
}
