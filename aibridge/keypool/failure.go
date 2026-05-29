package keypool

import (
	"net/http"
	"time"
)

// FailoverReason explains why a key attempt failed in a way that should move
// the failover loop to the next key.
type FailoverReason int

const (
	// FailoverRateLimited marks the key temporary and retries with the next
	// key (HTTP 429).
	FailoverRateLimited FailoverReason = iota
	// FailoverUnauthorized marks the key permanent and retries with the next
	// key (HTTP 401).
	FailoverUnauthorized
	// FailoverForbidden marks the key permanent and retries with the next key
	// (HTTP 403).
	FailoverForbidden
)

// Failure describes a key-specific attempt failure that triggers failover. A
// nil *Failure means no key failure: the attempt produced a result the caller
// should keep (a success, a non-key error, a transport error, or a streaming
// attempt that already committed).
type Failure struct {
	Reason FailoverReason
	// Cooldown is honored only for FailoverRateLimited.
	Cooldown time.Duration
}

// Classify maps a key-specific HTTP response to a *Failure. A nil response or
// any non-failover status yields nil. 429 yields FailoverRateLimited carrying
// the parsed Retry-After (or defaultCooldown when absent), 401 yields
// FailoverUnauthorized, and 403 yields FailoverForbidden.
//
// Classify intentionally takes an *http.Response, not a provider error, so
// the pool stays SDK-agnostic. Callers unwrap the response from their SDK's
// error type (e.g. errors.As(err, &apiErr); apiErr.Response) before calling.
func Classify(resp *http.Response) *Failure {
	if resp == nil {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		cooldown := ParseRetryAfter(resp)
		if cooldown <= 0 {
			cooldown = defaultCooldown
		}
		return &Failure{Reason: FailoverRateLimited, Cooldown: cooldown}
	case http.StatusUnauthorized:
		return &Failure{Reason: FailoverUnauthorized}
	case http.StatusForbidden:
		return &Failure{Reason: FailoverForbidden}
	default:
		return nil
	}
}
