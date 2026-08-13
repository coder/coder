package gitprovider

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

func TestCountDiffLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		diff      string
		additions int32
		deletions int32
	}{
		{
			name: "Empty",
		},
		{
			name:      "OnlyAdditions",
			diff:      "+a\n+b\n+c\n",
			additions: 3,
		},
		{
			name:      "OnlyDeletions",
			diff:      "-a\n-b\n",
			deletions: 2,
		},
		{
			name:      "MixedWithHeaders",
			diff:      "--- a/file.txt\n+++ b/file.txt\n@@ -1,2 +1,3 @@\n unchanged\n-old\n+new\n+another\n",
			additions: 2,
			deletions: 1,
		},
		{
			name:      "NoTrailingNewline",
			diff:      "@@ -1 +1 @@\n-old\n+new",
			additions: 1,
			deletions: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			additions, deletions := countDiffLines(tt.diff)
			assert.Equal(t, tt.additions, additions)
			assert.Equal(t, tt.deletions, deletions)
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	clk := quartz.NewMock(t)
	clk.Set(time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC))

	t.Run("RetryAfterSeconds", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "120")
		d := parseRetryAfter(h, "X-Ratelimit-Reset", clk)
		assert.Equal(t, 120*time.Second, d)
	})

	t.Run("GitHubResetHeader", func(t *testing.T) {
		t.Parallel()
		future := clk.Now().Add(90 * time.Second)
		h := http.Header{}
		h.Set("X-Ratelimit-Reset", strconv.FormatInt(future.Unix(), 10))
		d := parseRetryAfter(h, "X-Ratelimit-Reset", clk)
		assert.WithinDuration(t, future, clk.Now().Add(d), time.Second)
	})

	t.Run("GitLabResetHeader", func(t *testing.T) {
		t.Parallel()
		future := clk.Now().Add(45 * time.Second)
		h := http.Header{}
		h.Set("RateLimit-Reset", strconv.FormatInt(future.Unix(), 10))
		d := parseRetryAfter(h, "RateLimit-Reset", clk)
		assert.WithinDuration(t, future, clk.Now().Add(d), time.Second)
	})

	t.Run("NoHeaders", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		d := parseRetryAfter(h, "X-Ratelimit-Reset", clk)
		assert.Equal(t, time.Duration(0), d)
	})

	t.Run("InvalidValue", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "not-a-number")
		d := parseRetryAfter(h, "X-Ratelimit-Reset", clk)
		assert.Equal(t, time.Duration(0), d)
	})

	t.Run("RetryAfterTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "60")
		h.Set("X-Ratelimit-Reset", strconv.FormatInt(clk.Now().Add(120*time.Second).Unix(), 10))
		d := parseRetryAfter(h, "X-Ratelimit-Reset", clk)
		assert.Equal(t, 60*time.Second, d)
	})
}

func TestResponseCacheStore(t *testing.T) {
	t.Parallel()

	// Stores the same key twice using a buffer the caller mutates
	// between stores, and verifies load returns the bodies passed at
	// store time rather than the mutated buffer.
	t.Run("UpdateReplacesBody", func(t *testing.T) {
		t.Parallel()

		cache := newResponseCache(4)
		const key = "k"

		buf := []byte(`{"v":1}`)
		cache.store(key, `"etag-1"`, buf)
		// Mutate the caller's buffer: the cache must hold its own copy.
		for i := range buf {
			buf[i] = 'X'
		}

		etag, body, ok := cache.load(key)
		require.True(t, ok)
		assert.Equal(t, `"etag-1"`, etag)
		assert.Equal(t, `{"v":1}`, string(body))

		// Reuse the same buffer for a second store of the same key.
		buf = append(buf[:0], `{"v":2}`...)
		cache.store(key, `"etag-2"`, buf)
		for i := range buf {
			buf[i] = 'Y'
		}

		etag, body, ok = cache.load(key)
		require.True(t, ok)
		assert.Equal(t, `"etag-2"`, etag)
		assert.Equal(t, `{"v":2}`, string(body))
	})

	// Fills the cache past maxSize and verifies the
	// least-recently-used entry is evicted.
	t.Run("EvictsLeastRecentlyUsed", func(t *testing.T) {
		t.Parallel()

		cache := newResponseCache(2)
		cache.store("a", `"etag-a"`, []byte(`{"k":"a"}`))
		cache.store("b", `"etag-b"`, []byte(`{"k":"b"}`))
		// This third store exceeds maxSize and must evict "a".
		cache.store("c", `"etag-c"`, []byte(`{"k":"c"}`))

		_, _, ok := cache.load("a")
		assert.False(t, ok, "least-recently-used entry must be evicted")

		etag, body, ok := cache.load("b")
		require.True(t, ok)
		assert.Equal(t, `"etag-b"`, etag)
		assert.Equal(t, `{"k":"b"}`, string(body))

		etag, body, ok = cache.load("c")
		require.True(t, ok)
		assert.Equal(t, `"etag-c"`, etag)
		assert.Equal(t, `{"k":"c"}`, string(body))
	})
}

func TestMapGitLabState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect PRState
	}{
		{name: "opened", input: "opened", expect: PRStateOpen},
		{name: "Opened_mixed_case", input: "Opened", expect: PRStateOpen},
		{name: "merged", input: "merged", expect: PRStateMerged},
		{name: "closed", input: "closed", expect: PRStateClosed},
		{name: "locked", input: "locked", expect: PRStateClosed},
		{name: "unknown_defaults_to_closed", input: "something_else", expect: PRStateClosed},
		{name: "empty_defaults_to_closed", input: "", expect: PRStateClosed},
		{name: "whitespace_trimmed", input: "  opened  ", expect: PRStateOpen},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapGitLabState(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}
