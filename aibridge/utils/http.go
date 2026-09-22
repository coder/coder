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

const (
	ActorHeaderPrefix      = "X-AI-Bridge-Actor"
	actorHeaderPrefixLower = "x-ai-bridge-actor"
)

// IsActorHeader reports whether name is an AI Bridge actor header.
func IsActorHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), actorHeaderPrefixLower)
}

// NewStreamingTransport returns an HTTP transport for long-lived provider
// responses. It intentionally omits both dial and response-header timeouts so
// slow connection establishment and first-token latency are not cut off here.
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

// proxyHeaders describe the path the inbound request took to reach AI Gateway.
// They are not meaningful on the outbound request to the provider.
var proxyHeaders = []string{
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Forwarded-Port",
	"Forwarded",
}

// agentFirewallHeaders carry Agent Firewall correlation data used by
// AI Gateway for session correlation. AI Gateway records the values
// from the incoming request and strips the headers so they are never
// forwarded to upstream LLM providers.
var agentFirewallHeaders = []string{
	"X-Coder-Agent-Firewall-Session-Id",
	"X-Coder-Agent-Firewall-Sequence-Number",
}

// StripProxyAndFirewallHeaders removes inbound proxy and Agent Firewall
// correlation headers, which must not reach upstream providers.
func StripProxyAndFirewallHeaders(headers http.Header) {
	for _, h := range proxyHeaders {
		headers.Del(h)
	}
	for _, h := range agentFirewallHeaders {
		headers.Del(h)
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
