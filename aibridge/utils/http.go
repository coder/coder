package utils

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ActorHeaderPrefix = "X-AI-Bridge-Actor"

// IsActorHeader reports whether name is an AI Bridge actor header.
func IsActorHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(ActorHeaderPrefix))
}

// NewStreamingTransport returns an HTTP transport tuned for streaming AI
// provider responses. It deliberately has no ResponseHeaderTimeout because the
// first model response can take an unbounded amount of time.
func NewStreamingTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// StripCoderHeaders removes Coder-internal headers from headers. Provider
// authentication and standard HTTP headers are left unchanged.
func StripCoderHeaders(headers http.Header) {
	for name := range headers {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "coder-") || strings.HasPrefix(lower, "x-coder-") {
			delete(headers, name)
		}
	}
}

// NewJSONErrorResponse builds an *http.Response with a JSON body
// and optional Retry-After header. Used to synthesize bridge-side
// error responses (e.g. key-pool exhaustion, marshaling
// fallbacks). Retry-After is set to whole seconds (rounded up)
// when retryAfter is positive, and omitted otherwise.
func NewJSONErrorResponse(status int, retryAfter time.Duration, body []byte) *http.Response {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	if retryAfter > 0 {
		h.Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}
