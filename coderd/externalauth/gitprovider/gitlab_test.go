package gitprovider_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/quartz"
)

func TestGitLabFetchPullRequestStatus(t *testing.T) {
	t.Parallel()

	t.Run("HeadSHAFallback", func(t *testing.T) {
		t.Parallel()

		// When diff_refs.head_sha is empty, FetchPullRequestStatus
		// should fall back to the top-level sha field.
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/merge_requests/1", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"title":"T","state":"opened","source_branch":"feat","target_branch":"main","sha":"fallback-sha","draft":false,"iid":1,"changes_count":"1","web_url":"http://HOST/owner/repo/-/merge_requests/1","author":{"username":"u"},"diff_refs":{"head_sha":""}}`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/merge_requests/1/approvals", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"approved":false,"approved_by":[]}`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/merge_requests/1/commits", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[{"id":"abc","short_id":"abc","title":"c1"}]`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/merge_requests/1/diffs", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Two file diffs: first has +5/-2, second has +3/-1
			_, _ = w.Write([]byte(`[{"diff":"@@ -1,3 +1,6 @@\n+a\n+b\n+c\n+d\n+e\n-x\n-y\n","new_path":"file1.txt","old_path":"file1.txt"},{"diff":"@@ -1,2 +1,4 @@\n+a\n+b\n+c\n-x\n","new_path":"file2.txt","old_path":"file2.txt"}]`))
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		status, err := gp.FetchPullRequestStatus(
			t.Context(),
			"token",
			gitprovider.PRRef{Owner: "owner", Repo: "repo", Number: 1},
		)
		require.NoError(t, err)
		assert.Equal(t, "fallback-sha", status.HeadSHA)
		assert.Equal(t, int32(2), status.Commits)
		assert.Equal(t, int32(8), status.DiffStats.Additions)
		assert.Equal(t, int32(3), status.DiffStats.Deletions)
		assert.Equal(t, int32(1), status.DiffStats.ChangedFiles)
	})
}

func TestGitLabFetchPullRequestDiff(t *testing.T) {
	t.Parallel()

	t.Run("TooLarge", func(t *testing.T) {
		t.Parallel()

		oversizeDiff := string(make([]byte, gitprovider.MaxDiffSize+1024))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(oversizeDiff))
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		_, err = gp.FetchPullRequestDiff(
			t.Context(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		assert.ErrorIs(t, err, gitprovider.ErrDiffTooLarge)
	})
}

func TestGitLabFetchBranchDiff(t *testing.T) {
	t.Parallel()

	t.Run("TrailingNewlineAppended", func(t *testing.T) {
		t.Parallel()

		// When a file diff does not end with a newline, FetchBranchDiff
		// should append one so the unified diff is well-formed.
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/repository/compare", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// diff field intentionally lacks a trailing newline.
			_, _ = w.Write([]byte(`{"diffs":[{"old_path":"a.txt","new_path":"a.txt","diff":"@@ -1 +1 @@\n-old\n+new"}]}`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		diff, err := gp.FetchBranchDiff(
			t.Context(),
			"token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		// Must end with newline even though the API response did not.
		assert.True(t, len(diff) > 0 && diff[len(diff)-1] == '\n')
		assert.Equal(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n", diff)
	})

	t.Run("EmptyDefaultBranch", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":""}`))
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		_, err = gp.FetchBranchDiff(
			t.Context(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default branch is empty")
	})

	t.Run("CompareTimeout", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/repository/compare", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"compare_timeout":true,"diffs":[]}`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		_, err = gp.FetchBranchDiff(
			t.Context(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
	})

	t.Run("TooLarge", func(t *testing.T) {
		t.Parallel()

		buf := make([]byte, gitprovider.MaxDiffSize+1024)
		for i := range buf {
			buf[i] = 'x'
		}
		oversizeDiff := string(buf)
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/projects/owner%2Frepo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		})
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/repository/compare", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"diffs":[{"old_path":"big.txt","new_path":"big.txt","diff":"%s"}]}`, oversizeDiff)
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		_, err = gp.FetchBranchDiff(
			t.Context(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		assert.ErrorIs(t, err, gitprovider.ErrDiffTooLarge)
	})
}

func TestGitLabResolveBranchPullRequest(t *testing.T) {
	t.Parallel()

	t.Run("FallbackOnUnparsableWebURL", func(t *testing.T) {
		t.Parallel()

		// When the MR's web_url cannot be parsed by ParsePullRequestURL,
		// ResolveBranchPullRequest falls back to constructing the PRRef
		// from the known owner/repo and the returned IID.
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/projects/owner%2Frepo/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return a web_url that won't match the provider's host.
			_, _ = w.Write([]byte(`[{"iid":99,"web_url":"https://other-host.example.com/x/y/-/merge_requests/99"}]`))
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		prRef, err := gp.ResolveBranchPullRequest(
			t.Context(),
			"token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		require.NotNil(t, prRef)
		assert.Equal(t, "owner", prRef.Owner)
		assert.Equal(t, "repo", prRef.Repo)
		assert.Equal(t, 99, prRef.Number)
	})

	t.Run("EmptyRef", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("server should not be called for empty branch ref")
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		prRef, err := gp.ResolveBranchPullRequest(
			t.Context(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: ""},
		)
		require.NoError(t, err)
		assert.Nil(t, prRef)
	})
}

func TestGitLabRateLimit(t *testing.T) {
	t.Parallel()

	t.Run("429WithRetryAfter", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`))
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client(), gitprovider.WithClock(mClock))
		require.NoError(t, err)

		_, err = gp.FetchPullRequestStatus(
			t.Context(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.Error(t, err)

		rlErr, ok := errors.AsType[*gitprovider.RateLimitError](err)
		require.True(t, ok, "error should be *RateLimitError, got: %T", err)

		expected := mClock.Now().Add(120*time.Second + gitprovider.RateLimitPadding)
		assert.True(t, rlErr.RetryAfter.Equal(expected), "expected %v, got %v", expected, rlErr.RetryAfter)
	})

	t.Run("403WithRateLimitReset", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)

		resetTime := mClock.Now().Add(60 * time.Second)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`))
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client(), gitprovider.WithClock(mClock))
		require.NoError(t, err)

		_, err = gp.FetchPullRequestStatus(
			t.Context(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.Error(t, err)

		rlErr, ok := errors.AsType[*gitprovider.RateLimitError](err)
		require.True(t, ok, "error should be *RateLimitError, got: %T", err)

		expected := resetTime.Add(gitprovider.RateLimitPadding)
		assert.True(t, rlErr.RetryAfter.Equal(expected), "expected %v, got %v", expected, rlErr.RetryAfter)
	})

	t.Run("429OnRawDiffEndpoint", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "raw_diffs") {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		mClock := quartz.NewMock(t)
		mClock.Set(time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC))

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client(), gitprovider.WithClock(mClock))
		require.NoError(t, err)

		_, err = gp.FetchPullRequestDiff(
			t.Context(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.Error(t, err)

		rlErr, ok := errors.AsType[*gitprovider.RateLimitError](err)
		require.True(t, ok, "error should be *RateLimitError, got: %T", err)

		expected := mClock.Now().Add(60*time.Second + gitprovider.RateLimitPadding)
		assert.Equal(t, expected, rlErr.RetryAfter)
	})

	t.Run("403WithoutRateLimitHeaders", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"forbidden"}`))
		}))
		defer srv.Close()

		gp, err := gitprovider.New("gitlab", srv.URL, srv.Client())
		require.NoError(t, err)

		_, err = gp.FetchPullRequestStatus(
			t.Context(),
			"bad-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.Error(t, err)

		_, ok := errors.AsType[*gitprovider.RateLimitError](err)
		assert.False(t, ok, "error should NOT be *RateLimitError")
		assert.Contains(t, err.Error(), "403")
	})
}

func TestGitLabSelfHosted(t *testing.T) {
	t.Parallel()

	gp, err := gitprovider.New("gitlab", "https://gitlab.corp.com", nil)
	require.NoError(t, err)

	t.Run("ParseRepositoryOriginMatches", func(t *testing.T) {
		t.Parallel()
		owner, repo, _, ok := gp.ParseRepositoryOrigin("https://gitlab.corp.com/org/repo.git")
		assert.True(t, ok)
		assert.Equal(t, "org", owner)
		assert.Equal(t, "repo", repo)
	})

	t.Run("ParseRepositoryOriginRejectsGitLabCom", func(t *testing.T) {
		t.Parallel()
		_, _, _, ok := gp.ParseRepositoryOrigin("https://gitlab.com/org/repo.git")
		assert.False(t, ok, "gitlab.com URL should not match self-hosted instance")
	})

	t.Run("ParsePullRequestURLMatches", func(t *testing.T) {
		t.Parallel()
		ref, ok := gp.ParsePullRequestURL("https://gitlab.corp.com/org/repo/-/merge_requests/1")
		assert.True(t, ok)
		assert.Equal(t, "org", ref.Owner)
		assert.Equal(t, "repo", ref.Repo)
		assert.Equal(t, 1, ref.Number)
	})

	t.Run("ParsePullRequestURLRejectsGitLabCom", func(t *testing.T) {
		t.Parallel()
		_, ok := gp.ParsePullRequestURL("https://gitlab.com/org/repo/-/merge_requests/1")
		assert.False(t, ok, "gitlab.com MR URL should not match self-hosted instance")
	})

	t.Run("BuildPullRequestURL", func(t *testing.T) {
		t.Parallel()
		result := gp.BuildPullRequestURL(gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42})
		assert.Equal(t, "https://gitlab.corp.com/org/repo/-/merge_requests/42", result)
	})
}
