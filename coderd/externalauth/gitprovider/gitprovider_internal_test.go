package gitprovider

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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

	t.Run("NilClock", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "1")
		d := parseRetryAfter(h, "X-Ratelimit-Reset", nil)
		assert.Equal(t, time.Second, d)
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
