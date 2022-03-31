package coderd

import (
	"fmt"
	"strings"
)

const websocketCloseMaxLen = 123

// fmtWebsocketCloseMsg formats a websocket close message and ensures it is
// truncated to the maximum allowed length.
func fmtWebsocketCloseMsg(format string, vars ...any) string {
	msg := fmt.Sprintf(format, vars...)

	// Cap msg length at 123 bytes. nhooyr/websocket only allows close messages
	// of this length.
	if len(msg) > websocketCloseMaxLen {
		return truncateString(msg, websocketCloseMaxLen)
	}

	return msg
}

// truncateString safely truncates a string to a maximum size of byteLen. It
// writes whole runes until a single rune would increase the string size above
// byteLen.
func truncateString(str string, byteLen int) string {
	builder := strings.Builder{}
	builder.Grow(byteLen)

	for _, char := range str {
		if builder.Len()+len(string(char)) > byteLen {
			break
		}

		_, _ = builder.WriteRune(char)
	}

	return builder.String()
}
