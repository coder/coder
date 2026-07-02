package xhttp_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/util/xhttp"
)

func TestIsRateLimited(t *testing.T) {
	t.Parallel()

	hdr := func(headers map[string]string) http.Header {
		h := http.Header{}
		for k, v := range headers {
			h.Set(k, v)
		}
		return h
	}

	cases := []struct {
		name    string
		status  int
		nilResp bool
		header  map[string]string
		want    bool
	}{
		{name: "Nil", nilResp: true, want: false},
		{name: "OK", status: http.StatusOK, want: false},
		// A successful response with a zeroed remaining count is not a
		// rate-limited rejection.
		{name: "OKZeroRemaining", status: http.StatusOK, header: map[string]string{"X-RateLimit-Remaining": "0"}, want: false},
		{name: "TooManyRequests", status: http.StatusTooManyRequests, want: true},
		{name: "ForbiddenZeroRemaining", status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "0"}, want: true},
		{name: "ForbiddenRetryAfter", status: http.StatusForbidden, header: map[string]string{"Retry-After": "60"}, want: true},
		{name: "ForbiddenPositiveRemaining", status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "5000"}, want: false},
		{name: "ForbiddenNoHeaders", status: http.StatusForbidden, want: false},
		{name: "Unauthorized", status: http.StatusUnauthorized, header: map[string]string{"X-RateLimit-Remaining": "0"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var resp *http.Response
			if !tc.nilResp {
				resp = &http.Response{StatusCode: tc.status, Header: hdr(tc.header)}
			}
			assert.Equal(t, tc.want, xhttp.IsRateLimited(resp))
		})
	}
}
