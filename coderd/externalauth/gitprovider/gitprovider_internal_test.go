package gitprovider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
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

	t.Run("RetainsTenfoldWorkingSet", func(t *testing.T) {
		t.Parallel()

		// Dogfood on 2026-11-07 (CODAGT-1543): two keys per chat-owner/PR
		// pair plus one per chat-owner/branch lookup.
		const (
			pullRequests = 1201
			branches     = 329
			cacheKeys    = 2*pullRequests + branches
			keys         = 10 * cacheKeys
		)

		cache := newResponseCache(0, 0)
		body := []byte(`{"title":"` + strings.Repeat("t", 256) + `","state":"open","number":42}`)
		storedKeys := make([]string, keys)
		for i := range keys {
			requestURL := fmt.Sprintf("http://localhost/repos/owner/repo/pulls/%d", i)
			key := responseCacheKey(requestURL, "test-token")
			storedKeys[i] = key
			cache.store(key, `"v"`, body)
		}

		for _, key := range storedKeys {
			_, cachedBody, ok := cache.load(key)
			require.True(t, ok, "a tenfold working set must survive a full scan")
			require.Equal(t, body, cachedBody)
		}
		t.Logf("retained %d keys using %d accounted bytes", keys, cache.usedBytes)
	})

	// Stores the same key twice using a buffer the caller mutates
	// between stores, and verifies load returns the bodies passed at
	// store time rather than the mutated buffer.
	t.Run("UpdateReplacesBody", func(t *testing.T) {
		t.Parallel()

		cache := newResponseCache(4, 0)
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

	// Fills the cache past maxEntries and verifies the
	// least-recently-used entry is evicted, not merely the
	// oldest-inserted one.
	t.Run("EvictsLeastRecentlyUsed", func(t *testing.T) {
		t.Parallel()

		cache := newResponseCache(2, 0)
		cache.store("a", `"etag-a"`, []byte(`{"k":"a"}`))
		cache.store("b", `"etag-b"`, []byte(`{"k":"b"}`))
		// Access "a" so "b" becomes the least-recently-used entry.
		_, _, ok := cache.load("a")
		require.True(t, ok)
		// This third store exceeds maxEntries and must evict "b",
		// not the older "a".
		cache.store("c", `"etag-c"`, []byte(`{"k":"c"}`))

		_, _, ok = cache.load("b")
		assert.False(t, ok, "least-recently-used entry must be evicted")

		etag, body, ok := cache.load("a")
		require.True(t, ok, "recently-accessed entry must survive eviction")
		assert.Equal(t, `"etag-a"`, etag)
		assert.Equal(t, `{"k":"a"}`, string(body))

		etag, body, ok = cache.load("c")
		require.True(t, ok)
		assert.Equal(t, `"etag-c"`, etag)
		assert.Equal(t, `{"k":"c"}`, string(body))
	})

	t.Run("EvictsToByteLimit", func(t *testing.T) {
		t.Parallel()

		// These allocator-size-class lengths keep cap equal to len after
		// bytes.Clone. Three 84-byte entries use 252 of the 256-byte limit.
		cache := newResponseCache(8, 256)
		for _, key := range []string{"a", "b", "c"} {
			cache.store(key, `"v"`, make([]byte, 80))
		}

		_, _, ok := cache.load("a")
		require.True(t, ok)

		// The 164-byte "d" raises the total to 416; evicting "b" and "c"
		// leaves 248 accounted bytes.
		cache.store("d", `"v"`, make([]byte, 160))

		for _, key := range []string{"b", "c"} {
			_, _, ok := cache.load(key)
			require.False(t, ok, "multiple least-recently-used entries must be evicted")
		}

		var retainedBytes int
		for _, key := range []string{"a", "d"} {
			etag, body, ok := cache.load(key)
			require.True(t, ok)
			retainedBytes += len(key) + len(etag) + cap(body)
		}
		require.LessOrEqual(t, retainedBytes, 256)
		require.Equal(t, retainedBytes, cache.usedBytes)
	})

	t.Run("ReplacementRespectsByteLimit", func(t *testing.T) {
		t.Parallel()

		// Each entry accounts for 84 bytes, so three use 252 of 256 bytes.
		// The projection lengths are allocator size classes.
		cache := newResponseCache(8, 256)
		for _, key := range []string{"a", "b", "c"} {
			cache.store(key, `"v"`, make([]byte, 80))
		}

		// Growing "a" to 170 bytes exceeds the byte limit and evicts "b".
		cache.store("a", `"updated"`, make([]byte, 160))

		_, _, ok := cache.load("b")
		require.False(t, ok, "a larger replacement must evict older entries")

		etag, body, ok := cache.load("a")
		require.True(t, ok)
		require.Equal(t, `"updated"`, etag)
		require.Len(t, body, 160)

		// Shrinking "a" to 20 bytes leaves room for the 132-byte "d"
		// without evicting "c".
		cache.store("a", `"v"`, make([]byte, 16))
		cache.store("d", `"v"`, make([]byte, 128))

		var retainedBytes int
		for _, key := range []string{"a", "c", "d"} {
			etag, body, ok := cache.load(key)
			require.True(t, ok, "a smaller replacement must free space below the byte limit")
			retainedBytes += len(key) + len(etag) + cap(body)
		}
		require.LessOrEqual(t, retainedBytes, 256)
		require.Equal(t, retainedBytes, cache.usedBytes)
	})

	t.Run("EntryExceedingByteLimitNotStored", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name string
			key  string
			etag string
			body []byte
		}{
			{
				name: "JSONProjection",
				key:  "large",
				etag: `"v"`,
				body: make([]byte, 65),
			},
			{
				name: "Key",
				key:  strings.Repeat("k", 65),
				etag: `"v"`,
				body: []byte("{}"),
			},
			{
				name: "ETag",
				key:  "large",
				etag: strings.Repeat("e", 65),
				body: []byte("{}"),
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				cache := newResponseCache(8, 64)
				cache.store("a", `"v"`, []byte("{}"))
				cache.store(tt.key, tt.etag, tt.body)

				_, _, ok := cache.load(tt.key)
				require.False(t, ok)
				_, _, ok = cache.load("a")
				require.True(t, ok, "an uncacheable entry must not evict cached responses")
			})
		}
	})

	// Bodies larger than maxCachedBodyBytes are not stored, so a
	// single oversized response cannot unbound the cache.
	t.Run("OversizedBodyNotStored", func(t *testing.T) {
		t.Parallel()

		cache := newResponseCache(4, 0)
		cache.store("big", `"etag-big"`, make([]byte, maxCachedBodyBytes+1))

		_, _, ok := cache.load("big")
		assert.False(t, ok, "bodies exceeding maxCachedBodyBytes must not be cached")
	})
}

func TestGitHubCachedProjection(t *testing.T) {
	t.Parallel()

	const responsePaddingBytes = 26 << 10
	var fullResponses atomic.Int64
	var notModifiedResponses atomic.Int64
	padding := strings.Repeat("p", responsePaddingBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v"`)
		if r.Header.Get("If-None-Match") != "" {
			notModifiedResponses.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}

		fullResponses.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"title":"example","number":42,"body":%q}`, padding)
	}))
	defer srv.Close()

	provider, err := newGitHub(srv.URL, srv.Client(), quartz.NewMock(t))
	require.NoError(t, err)
	github, ok := provider.(*githubProvider)
	require.True(t, ok)

	// The first pass fetches the response body; the second uses its JSON
	// projection after a 304.
	for range 2 {
		var title struct {
			Title string `json:"title"`
		}
		err := github.decodeJSON(t.Context(), srv.URL, "test-token", &title)
		require.NoError(t, err)
		require.Equal(t, "example", title.Title)
		require.Less(t, github.cache.usedBytes, 1024, "unused response fields must not count toward the byte limit")
	}

	require.EqualValues(t, 1, fullResponses.Load())
	require.EqualValues(t, 1, notModifiedResponses.Load())
	t.Logf("retained a JSON projection using %d accounted bytes", github.cache.usedBytes)
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
