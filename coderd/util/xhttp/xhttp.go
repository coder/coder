// Package xhttp contains small helpers extending the standard net/http
// package for working with HTTP responses from external services.
package xhttp

import "net/http"

// IsRateLimited reports whether resp is a rate-limited rejection:
// a 429, or a 403 with Retry-After or X-RateLimit-Remaining: 0.
//
// Does not catch every secondary rate limit: GitHub can return 403 with
// positive remaining and no Retry-After, requiring body inspection.
func IsRateLimited(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return resp.Header.Get("Retry-After") != "" ||
			resp.Header.Get("X-RateLimit-Remaining") == "0"
	default:
		return false
	}
}
