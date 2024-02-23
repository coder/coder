package promoauth

import (
	"net/http"
	"strconv"
	"time"

	"golang.org/x/xerrors"
)

type rateLimits struct {
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
	Resource  string
}

// githubRateLimits returns rate limit information from a GitHub response.
// GitHub rate limits are on a per-user basis, and tracking each user as
// a prometheus label might be too much. So only track rate limits for
// unauthorized responses.
//
// Unauthorized responses have a much stricter rate limit of 60 per hour.
// Tracking this is vital to ensure we do not hit the limit.
func githubRateLimits(resp *http.Response, err error) (rateLimits, bool) {
	if err != nil || resp == nil {
		return rateLimits{}, false
	}

	// Only track 401 responses which indicates we are using the 60 per hour
	// rate limit.
	if resp.StatusCode != http.StatusUnauthorized {
		return rateLimits{}, false
	}

	p := headerParser{header: resp.Header}
	// See
	// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#checking-the-status-of-your-rate-limit
	limits := rateLimits{
		Limit:     p.int("x-ratelimit-limit"),
		Remaining: p.int("x-ratelimit-remaining"),
		Used:      p.int("x-ratelimit-used"),
		Resource:  p.string("x-ratelimit-resource") + "-unauthorized",
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
		p.errors[key] = xerrors.Errorf("missing header %q", key)
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
