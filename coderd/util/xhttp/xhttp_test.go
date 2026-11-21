package xhttp_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		// GitHub secondary limits send Retry-After while the primary quota
		// still has remaining requests; Retry-After alone is sufficient.
		{name: "ForbiddenRetryAfterPositiveRemaining", status: http.StatusForbidden, header: map[string]string{"Retry-After": "60", "X-RateLimit-Remaining": "5000"}, want: true},
		{name: "ForbiddenPositiveRemaining", status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "5000"}, want: false},
		// GitLab uses the unprefixed RateLimit-Remaining header.
		{name: "ForbiddenGitLabZeroRemaining", status: http.StatusForbidden, header: map[string]string{"RateLimit-Remaining": "0"}, want: true},
		{name: "ForbiddenGitLabPositiveRemaining", status: http.StatusForbidden, header: map[string]string{"RateLimit-Remaining": "42"}, want: false},
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

// countingReader records how many bytes are pulled from the underlying body,
// which is what bounds memory, as opposed to how many are returned.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestLimitResponseBody(t *testing.T) {
	t.Parallel()

	const limit = 8
	// drain reads r to the end with reads of at most chunk bytes, or with
	// io.ReadAll when chunk is zero.
	drain := func(r io.Reader, chunk int) ([]byte, error) {
		if chunk == 0 {
			return io.ReadAll(r)
		}
		var out []byte
		buf := make([]byte, chunk)
		for {
			n, err := r.Read(buf)
			out = append(out, buf[:n]...)
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			if err != nil {
				return out, err
			}
		}
	}

	cases := []struct {
		name     string
		size     int
		chunk    int
		wantRead int
		wantErr  bool
	}{
		{name: "AtLimit", size: limit, wantRead: limit},
		{name: "OverLimit", size: limit + 1, wantRead: limit, wantErr: true},
		{name: "FarOverLimit", size: 4 * limit, wantRead: limit, wantErr: true},
		{name: "FarOverLimitSmallReads", size: 4 * limit, chunk: 3, wantRead: limit, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &countingReader{r: strings.NewReader(strings.Repeat("a", tc.size))}
			limited := xhttp.LimitResponseBody(body, limit)
			data, err := drain(limited, tc.chunk)
			assert.Len(t, data, tc.wantRead)
			if !tc.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tc.size, body.n)
				return
			}
			tooLarge, ok := errors.AsType[*xhttp.ResponseBodyTooLargeError](err)
			require.True(t, ok, "want *ResponseBodyTooLargeError, got %v", err)
			assert.EqualValues(t, limit, tooLarge.Limit)

			// Once the limit is exceeded, further reads fail without
			// pulling more of the body.
			_, err = limited.Read(make([]byte, tc.size))
			_, ok = errors.AsType[*xhttp.ResponseBodyTooLargeError](err)
			require.True(t, ok, "want *ResponseBodyTooLargeError after the limit, got %v", err)
			assert.LessOrEqual(t, body.n, limit+1,
				"must consume at most the limit plus one detection byte")
		})
	}
}
