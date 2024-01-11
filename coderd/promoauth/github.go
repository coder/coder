package promoauth

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type rateLimits struct {
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
	Resource  string
}

// githubRateLimits checks the returned response headers and
func githubRateLimits(resp *http.Response, err error) (rateLimits, bool) {
	if err != nil || resp == nil {
		return rateLimits{}, false
	}

	p := headerParser{header: resp.Header}
	// See
	// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#checking-the-status-of-your-rate-limit
	limits := rateLimits{
		Limit:     p.int("x-ratelimit-limit"),
		Remaining: p.int("x-ratelimit-remaining"),
		Used:      p.int("x-ratelimit-used"),
		Resource:  p.string("x-ratelimit-resource"),
	}

	if limits.Limit == 0 &&
		limits.Remaining == 0 &&
		limits.Used == 0 {
		// For some requests, github has no rate limit. In which case,
		// it returns all 0s. We can just omit these.
		return limits, false
	}

	// Reset is when the rate limit "used" will be reset to 0.
	// If it's unix 0, then we do not know when it will reset.
	// Change it to a zero time as that is easier to handle in golang.
	unix := p.int("x-ratelimit-reset")
	resetAt := time.Unix(int64(unix), 0)
	if unix == 0 {
		resetAt = time.Time{}
	}
	limits.Reset = resetAt

	// Unauthorized requests have their own rate limit, so we should
	// track them separately.
	if resp.StatusCode == http.StatusUnauthorized {
		limits.Resource += "-unauthorized"
	}

	// A 401 or 429 means too many requests. This might mess up the
	// "resource" string because we could hit the unauthorized limit,
	// and we do not want that to override the authorized one.
	// However, in testing, it seems a 401 is always a 401, even if
	// the limit is hit.

	if len(p.errors) > 0 {
		// If we are missing any headers, then do not try and guess
		// what the rate limits are.
		return limits, false
	}
	return limits, true
}

type headerParser struct {
	errors map[string]error
	header http.Header
}

func (p *headerParser) string(key string) string {
	if p.errors == nil {
		p.errors = make(map[string]error)
	}

	v := p.header.Get(key)
	if v == "" {
		p.errors[key] = fmt.Errorf("missing header %q", key)
	}
	return v
}

func (p *headerParser) int(key string) int {
	v := p.string(key)
	if v == "" {
		return -1
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		p.errors[key] = err
	}
	return i
}
