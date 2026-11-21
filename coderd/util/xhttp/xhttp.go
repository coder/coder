// Package xhttp contains small helpers extending the standard net/http
// package for working with HTTP responses from external services.
package xhttp

import (
	"fmt"
	"io"
	"net/http"
)

// MaxProviderResponseBytes bounds how much of a response body is read from an
// identity or OAuth2 provider. Discovery documents, user profiles, revocation
// replies, and device authorization responses are a few kilobytes at most. The
// value matches the limit golang.org/x/oauth2 applies to token responses.
const MaxProviderResponseBytes int64 = 1 << 20 // 1 MiB

// ResponseBodyTooLargeError is returned by a reader from LimitResponseBody
// once the body exceeds the limit.
type ResponseBodyTooLargeError struct {
	Limit int64
}

func (e *ResponseBodyTooLargeError) Error() string {
	return fmt.Sprintf("response body exceeds %d bytes", e.Limit)
}

// LimitResponseBody returns a reader that reads at most limit bytes from body,
// plus one byte to detect that the limit was exceeded. Unlike io.LimitReader,
// a read past the limit fails with a *ResponseBodyTooLargeError instead of
// io.EOF, so a truncated body is not mistaken for a complete one.
//
// It bounds how much is read, not the size of the response: a caller that
// stops early, such as a json.Decoder that has decoded a complete value, sees
// no error however long the rest of the body is. The caller still owns
// closing body.
func LimitResponseBody(body io.Reader, limit int64) io.Reader {
	return &limitedBody{r: body, remaining: limit, limit: limit}
}

type limitedBody struct {
	r io.Reader
	// remaining is the number of bytes still permitted, or negative once the
	// limit has been exceeded.
	remaining int64
	limit     int64
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.remaining < 0 {
		return 0, &ResponseBodyTooLargeError{Limit: l.limit}
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Read one byte past the limit so a body of exactly limit bytes can be
	// told apart from a longer one.
	if int64(len(p)) > l.remaining+1 {
		p = p[:l.remaining+1]
	}
	n, err := l.r.Read(p)
	if int64(n) <= l.remaining {
		l.remaining -= int64(n)
		return n, err
	}
	n = int(l.remaining)
	l.remaining = -1
	return n, &ResponseBodyTooLargeError{Limit: l.limit}
}

// IsRateLimited reports whether resp is a rate-limited rejection:
// a 429, or a 403 with Retry-After present or a zeroed remaining count.
// The remaining count is read from X-RateLimit-Remaining (GitHub) or the
// unprefixed RateLimit-Remaining (GitLab, IETF draft).
//
// Reset headers are not a signal: providers attach them to non-throttled
// responses as well. GitHub can return 403 with positive remaining and no
// Retry-After; those require body inspection and are not detected.
func IsRateLimited(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return resp.Header.Get("Retry-After") != "" ||
			resp.Header.Get("X-RateLimit-Remaining") == "0" ||
			resp.Header.Get("RateLimit-Remaining") == "0"
	default:
		return false
	}
}
