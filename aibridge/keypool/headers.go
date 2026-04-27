package keypool

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter extracts the cooldown duration from response
// headers. It prefers the OpenAI-specific "retry-after-ms"
// header (milliseconds) over the standard "Retry-After" header
// (seconds). Returns zero if neither header is present or
// parseable.
func ParseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	// OpenAI convention: millisecond precision.
	if val := resp.Header.Get("retry-after-ms"); val != "" {
		ms, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err == nil && ms > 0 {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}

	// Standard header: seconds.
	if val := resp.Header.Get("Retry-After"); val != "" {
		seconds, err := strconv.Atoi(strings.TrimSpace(val))
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	return 0
}
