package gitprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/quartz"
)

func TestGitHubParseRepositoryOrigin(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name             string
		raw              string
		expectOK         bool
		expectOwner      string
		expectRepo       string
		expectNormalized string
	}{
		{
			name:             "HTTPS URL",
			raw:              "https://github.com/coder/coder",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "HTTPS URL with .git",
			raw:              "https://github.com/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "HTTPS URL with trailing slash",
			raw:              "https://github.com/coder/coder/",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL",
			raw:              "git@github.com:coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL without .git",
			raw:              "git@github.com:coder/coder",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:             "SSH URL with ssh:// prefix",
			raw:              "ssh://git@github.com/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://github.com/coder/coder",
		},
		{
			name:     "GitLab URL does not match",
			raw:      "https://gitlab.com/coder/coder",
			expectOK: false,
		},
		{
			name:     "Empty string",
			raw:      "",
			expectOK: false,
		},
		{
			name:     "Not a URL",
			raw:      "not-a-url",
			expectOK: false,
		},
		{
			name:             "Hyphenated owner and repo",
			raw:              "https://github.com/my-org/my-repo.git",
			expectOK:         true,
			expectOwner:      "my-org",
			expectRepo:       "my-repo",
			expectNormalized: "https://github.com/my-org/my-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owner, repo, normalized, ok := gp.ParseRepositoryOrigin(tt.raw)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectOwner, owner)
				assert.Equal(t, tt.expectRepo, repo)
				assert.Equal(t, tt.expectNormalized, normalized)
			}
		})
	}
}

func TestGitHubParsePullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name         string
		raw          string
		expectOK     bool
		expectOwner  string
		expectRepo   string
		expectNumber int
	}{
		{
			name:         "Standard PR URL",
			raw:          "https://github.com/coder/coder/pull/123",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 123,
		},
		{
			name:         "PR URL with query string",
			raw:          "https://github.com/coder/coder/pull/456?diff=split",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 456,
		},
		{
			name:         "PR URL with fragment",
			raw:          "https://github.com/coder/coder/pull/789#discussion",
			expectOK:     true,
			expectOwner:  "coder",
			expectRepo:   "coder",
			expectNumber: 789,
		},
		{
			name:     "Not a PR URL",
			raw:      "https://github.com/coder/coder",
			expectOK: false,
		},
		{
			name:     "Issue URL (not PR)",
			raw:      "https://github.com/coder/coder/issues/123",
			expectOK: false,
		},
		{
			name:     "GitLab MR URL",
			raw:      "https://gitlab.com/coder/coder/-/merge_requests/123",
			expectOK: false,
		},
		{
			name:     "Empty string",
			raw:      "",
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref, ok := gp.ParsePullRequestURL(tt.raw)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectOwner, ref.Owner)
				assert.Equal(t, tt.expectRepo, ref.Repo)
				assert.Equal(t, tt.expectNumber, ref.Number)
			}
		})
	}
}

func TestGitHubNormalizePullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{
			name:     "Already normalized",
			raw:      "https://github.com/coder/coder/pull/123",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With trailing punctuation",
			raw:      "https://github.com/coder/coder/pull/123).",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With query string",
			raw:      "https://github.com/coder/coder/pull/123?diff=split",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "With whitespace",
			raw:      "  https://github.com/coder/coder/pull/123  ",
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "Not a PR URL",
			raw:      "https://example.com",
			expected: "",
		},
		{
			name:     "Empty string",
			raw:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.NormalizePullRequestURL(tt.raw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubBuildBranchURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		owner    string
		repo     string
		branch   string
		expected string
	}{
		{
			name:     "Simple branch",
			owner:    "coder",
			repo:     "coder",
			branch:   "main",
			expected: "https://github.com/coder/coder/tree/main",
		},
		{
			name:     "Branch with slash",
			owner:    "coder",
			repo:     "coder",
			branch:   "feat/new-thing",
			expected: "https://github.com/coder/coder/tree/feat/new-thing",
		},
		{
			name:     "Empty owner",
			owner:    "",
			repo:     "coder",
			branch:   "main",
			expected: "",
		},
		{
			name:     "Empty repo",
			owner:    "coder",
			repo:     "",
			branch:   "main",
			expected: "",
		},
		{
			name:     "Empty branch",
			owner:    "coder",
			repo:     "coder",
			branch:   "",
			expected: "",
		},
		{
			name:     "Branch with slashes",
			owner:    "my-org",
			repo:     "my-repo",
			branch:   "feat/new-thing",
			expected: "https://github.com/my-org/my-repo/tree/feat/new-thing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.BuildBranchURL(tt.owner, tt.repo, tt.branch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubBuildPullRequestURL(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)

	tests := []struct {
		name     string
		ref      gitprovider.PRRef
		expected string
	}{
		{
			name:     "Valid PR ref",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 123},
			expected: "https://github.com/coder/coder/pull/123",
		},
		{
			name:     "Empty owner",
			ref:      gitprovider.PRRef{Owner: "", Repo: "coder", Number: 123},
			expected: "",
		},
		{
			name:     "Empty repo",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "", Number: 123},
			expected: "",
		},
		{
			name:     "Zero number",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 0},
			expected: "",
		},
		{
			name:     "Negative number",
			ref:      gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: -1},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := gp.BuildPullRequestURL(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubEnterpriseURLs(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("github", "https://ghes.corp.com/api/v3", nil)
	require.NotNil(t, gp)

	t.Run("ParseRepositoryOrigin HTTPS", func(t *testing.T) {
		t.Parallel()
		owner, repo, normalized, ok := gp.ParseRepositoryOrigin("https://ghes.corp.com/org/repo.git")
		assert.True(t, ok)
		assert.Equal(t, "org", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "https://ghes.corp.com/org/repo", normalized)
	})

	t.Run("ParseRepositoryOrigin SSH", func(t *testing.T) {
		t.Parallel()
		owner, repo, normalized, ok := gp.ParseRepositoryOrigin("git@ghes.corp.com:org/repo.git")
		assert.True(t, ok)
		assert.Equal(t, "org", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "https://ghes.corp.com/org/repo", normalized)
	})

	t.Run("ParsePullRequestURL", func(t *testing.T) {
		t.Parallel()
		ref, ok := gp.ParsePullRequestURL("https://ghes.corp.com/org/repo/pull/42")
		assert.True(t, ok)
		assert.Equal(t, "org", ref.Owner)
		assert.Equal(t, "repo", ref.Repo)
		assert.Equal(t, 42, ref.Number)
	})

	t.Run("NormalizePullRequestURL", func(t *testing.T) {
		t.Parallel()
		result := gp.NormalizePullRequestURL("https://ghes.corp.com/org/repo/pull/42?x=y")
		assert.Equal(t, "https://ghes.corp.com/org/repo/pull/42", result)
	})

	t.Run("BuildBranchURL", func(t *testing.T) {
		t.Parallel()
		result := gp.BuildBranchURL("org", "repo", "main")
		assert.Equal(t, "https://ghes.corp.com/org/repo/tree/main", result)
	})

	t.Run("BuildPullRequestURL", func(t *testing.T) {
		t.Parallel()
		result := gp.BuildPullRequestURL(gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 42})
		assert.Equal(t, "https://ghes.corp.com/org/repo/pull/42", result)
	})

	t.Run("github.com URLs do not match GHE instance", func(t *testing.T) {
		t.Parallel()
		_, _, _, ok := gp.ParseRepositoryOrigin("https://github.com/coder/coder")
		assert.False(t, ok, "github.com HTTPS URL should not match GHE instance")

		_, _, _, ok = gp.ParseRepositoryOrigin("git@github.com:coder/coder.git")
		assert.False(t, ok, "github.com SSH URL should not match GHE instance")

		_, ok = gp.ParsePullRequestURL("https://github.com/coder/coder/pull/123")
		assert.False(t, ok, "github.com PR URL should not match GHE instance")
	})
}

func TestNewUnsupportedProvider(t *testing.T) {
	t.Parallel()
	gp := gitprovider.New("unsupported", "", nil)
	assert.Nil(t, gp, "unsupported provider type should return nil")
}

func TestGitHubRatelimit_403WithResetHeader(t *testing.T) {
	t.Parallel()

	resetTime := time.Now().Add(60 * time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
	}))
	defer srv.Close()

	gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NotNil(t, gp)

	_, err := gp.FetchPullRequestStatus(
		context.Background(),
		"test-token",
		gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
	)
	require.Error(t, err)

	var rlErr *gitprovider.RateLimitError
	require.True(t, errors.As(err, &rlErr), "error should be *RateLimitError, got: %T", err)
	assert.WithinDuration(t, resetTime.Add(gitprovider.RateLimitPadding), rlErr.RetryAfter, 2*time.Second)
}

func TestGitHubRatelimit_429WithRetryAfter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message": "secondary rate limit"}`))
	}))
	defer srv.Close()

	gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NotNil(t, gp)

	_, err := gp.FetchPullRequestStatus(
		context.Background(),
		"test-token",
		gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
	)
	require.Error(t, err)

	var rlErr *gitprovider.RateLimitError
	require.True(t, errors.As(err, &rlErr), "error should be *RateLimitError, got: %T", err)

	// Retry-After: 120 means ~120s from now.
	expected := time.Now().Add(120 * time.Second)
	assert.WithinDuration(t, expected.Add(gitprovider.RateLimitPadding), rlErr.RetryAfter, 5*time.Second)
}

func TestGitHubRatelimit_403NormalError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer srv.Close()

	gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NotNil(t, gp)

	_, err := gp.FetchPullRequestStatus(
		context.Background(),
		"bad-token",
		gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
	)
	require.Error(t, err)

	var rlErr *gitprovider.RateLimitError
	assert.False(t, errors.As(err, &rlErr), "error should NOT be *RateLimitError")
	assert.Contains(t, err.Error(), "403")
}

func TestGitHubFetchPullRequestDiff(t *testing.T) {
	t.Parallel()

	const smallDiff = "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(smallDiff))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		diff, err := gp.FetchPullRequestDiff(
			context.Background(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.NoError(t, err)
		assert.Equal(t, smallDiff, diff)
	})

	t.Run("ExactlyMaxSize", func(t *testing.T) {
		t.Parallel()

		exactDiff := string(make([]byte, gitprovider.MaxDiffSize))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(exactDiff))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		diff, err := gp.FetchPullRequestDiff(
			context.Background(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		require.NoError(t, err)
		assert.Len(t, diff, gitprovider.MaxDiffSize)
	})

	t.Run("TooLarge", func(t *testing.T) {
		t.Parallel()

		oversizeDiff := string(make([]byte, gitprovider.MaxDiffSize+1024))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(oversizeDiff))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		_, err := gp.FetchPullRequestDiff(
			context.Background(),
			"test-token",
			gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
		)
		assert.ErrorIs(t, err, gitprovider.ErrDiffTooLarge)
	})
}

func TestFetchPullRequestDiff_Ratelimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message": "rate limit"}`))
	}))
	defer srv.Close()

	gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NotNil(t, gp)

	_, err := gp.FetchPullRequestDiff(
		context.Background(),
		"test-token",
		gitprovider.PRRef{Owner: "org", Repo: "repo", Number: 1},
	)
	require.Error(t, err)

	var rlErr *gitprovider.RateLimitError
	require.True(t, errors.As(err, &rlErr), "error should be *RateLimitError, got: %T", err)
	expected := time.Now().Add(60 * time.Second)
	assert.WithinDuration(t, expected.Add(gitprovider.RateLimitPadding), rlErr.RetryAfter, 5*time.Second)
}

func TestFetchBranchDiff_Ratelimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/compare/") {
			// Second request: compare endpoint returns 429.
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message": "rate limit"}`))
			return
		}
		// First request: repo metadata.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	}))
	defer srv.Close()

	gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
	require.NotNil(t, gp)

	_, err := gp.FetchBranchDiff(
		context.Background(),
		"test-token",
		gitprovider.BranchRef{Owner: "org", Repo: "repo", Branch: "feat"},
	)
	require.Error(t, err)

	var rlErr *gitprovider.RateLimitError
	require.True(t, errors.As(err, &rlErr), "error should be *RateLimitError, got: %T", err)
	expected := time.Now().Add(60 * time.Second)
	assert.WithinDuration(t, expected.Add(gitprovider.RateLimitPadding), rlErr.RetryAfter, 5*time.Second)
}

func TestFetchPullRequestStatus(t *testing.T) {
	t.Parallel()

	type review struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	makeReview := func(id int64, state, login string) review {
		r := review{ID: id, State: state}
		r.User.Login = login
		return r
	}

	tests := []struct {
		name             string
		pullJSON         string
		reviews          []review
		expectedState    gitprovider.PRState
		expectedDraft    bool
		changesRequested bool
	}{
		{
			name:             "OpenPR/NoReviews",
			pullJSON:         `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews:          []review{},
			expectedState:    gitprovider.PRStateOpen,
			expectedDraft:    false,
			changesRequested: false,
		},
		{
			name:             "OpenPR/SingleChangesRequested",
			pullJSON:         `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews:          []review{makeReview(1, "CHANGES_REQUESTED", "alice")},
			expectedState:    gitprovider.PRStateOpen,
			changesRequested: true,
		},
		{
			name:     "OpenPR/ChangesRequestedThenApproved",
			pullJSON: `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews: []review{
				makeReview(1, "CHANGES_REQUESTED", "alice"),
				makeReview(2, "APPROVED", "alice"),
			},
			expectedState:    gitprovider.PRStateOpen,
			changesRequested: false,
		},
		{
			name:     "OpenPR/ChangesRequestedThenDismissed",
			pullJSON: `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews: []review{
				makeReview(1, "CHANGES_REQUESTED", "alice"),
				makeReview(2, "DISMISSED", "alice"),
			},
			expectedState:    gitprovider.PRStateOpen,
			changesRequested: false,
		},
		{
			name:     "OpenPR/MultipleReviewersMixed",
			pullJSON: `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews: []review{
				makeReview(1, "APPROVED", "alice"),
				makeReview(2, "CHANGES_REQUESTED", "bob"),
			},
			expectedState:    gitprovider.PRStateOpen,
			changesRequested: true,
		},
		{
			name:     "OpenPR/CommentedDoesNotAffect",
			pullJSON: `{"state":"open","merged":false,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews: []review{
				makeReview(1, "COMMENTED", "alice"),
			},
			expectedState:    gitprovider.PRStateOpen,
			changesRequested: false,
		},
		{
			name:             "MergedPR",
			pullJSON:         `{"state":"closed","merged":true,"draft":false,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews:          []review{},
			expectedState:    gitprovider.PRStateMerged,
			changesRequested: false,
		},
		{
			name:             "DraftPR",
			pullJSON:         `{"state":"open","merged":false,"draft":true,"additions":10,"deletions":5,"changed_files":3,"head":{"sha":"abc123","ref":"feature-branch"}}`,
			reviews:          []review{},
			expectedState:    gitprovider.PRStateOpen,
			expectedDraft:    true,
			changesRequested: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reviewsJSON, err := json.Marshal(tc.reviews)
			require.NoError(t, err)

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v3/repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(reviewsJSON)
			})
			mux.HandleFunc("/api/v3/repos/owner/repo/pulls/1", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.pullJSON))
			})

			srv := httptest.NewServer(mux)
			defer srv.Close()

			gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
			require.NotNil(t, gp)

			before := time.Now().UTC()
			status, err := gp.FetchPullRequestStatus(
				context.Background(),
				"test-token",
				gitprovider.PRRef{Owner: "owner", Repo: "repo", Number: 1},
			)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedState, status.State)
			assert.Equal(t, tc.expectedDraft, status.Draft)
			assert.Equal(t, tc.changesRequested, status.ChangesRequested)
			assert.Equal(t, "abc123", status.HeadSHA)
			assert.Equal(t, "feature-branch", status.HeadBranch)
			assert.Equal(t, int32(10), status.DiffStats.Additions)
			assert.Equal(t, int32(5), status.DiffStats.Deletions)
			assert.Equal(t, int32(3), status.DiffStats.ChangedFiles)
			assert.False(t, status.FetchedAt.IsZero())
			assert.True(t, !status.FetchedAt.Before(before), "FetchedAt should be >= test start time")
		})
	}
}

func TestResolveBranchPullRequest(t *testing.T) {
	t.Parallel()

	t.Run("Found", func(t *testing.T) {
		t.Parallel()

		var srvURL string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify query parameters.
			assert.Equal(t, "open", r.URL.Query().Get("state"))
			assert.Equal(t, "owner:feat", r.URL.Query().Get("head"))
			w.Header().Set("Content-Type", "application/json")
			// Use the test server's URL so ParsePullRequestURL
			// matches the provider's derived web host.
			htmlURL := fmt.Sprintf("https://%s/owner/repo/pull/42",
				strings.TrimPrefix(strings.TrimPrefix(srvURL, "http://"), "https://"))
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"html_url":%q,"number":42}]`, htmlURL)))
		}))
		defer srv.Close()
		srvURL = srv.URL

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		prRef, err := gp.ResolveBranchPullRequest(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		require.NotNil(t, prRef)
		assert.Equal(t, "owner", prRef.Owner)
		assert.Equal(t, "repo", prRef.Repo)
		assert.Equal(t, 42, prRef.Number)
	})

	t.Run("NoneOpen", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		prRef, err := gp.ResolveBranchPullRequest(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		assert.Nil(t, prRef)
	})

	t.Run("InvalidHTMLURL", func(t *testing.T) {
		t.Parallel()

		// If html_url can't be parsed as a PR URL, ResolveBranchPullRequest
		// returns nil, nil.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"html_url":"not-a-valid-url","number":42}]`))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		prRef, err := gp.ResolveBranchPullRequest(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "owner", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		assert.Nil(t, prRef)
	})
}

func TestFetchBranchDiff(t *testing.T) {
	t.Parallel()

	const smallDiff = "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/compare/") {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(smallDiff))
				return
			}
			// Repo metadata.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		diff, err := gp.FetchBranchDiff(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "org", Repo: "repo", Branch: "feat"},
		)
		require.NoError(t, err)
		assert.Equal(t, smallDiff, diff)
	})

	t.Run("EmptyDefaultBranch", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":""}`))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		_, err := gp.FetchBranchDiff(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "org", Repo: "repo", Branch: "feat"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default branch is empty")
	})

	t.Run("DiffTooLarge", func(t *testing.T) {
		t.Parallel()

		oversizeDiff := string(make([]byte, gitprovider.MaxDiffSize+1024))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/compare/") {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(oversizeDiff))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		}))
		defer srv.Close()

		gp := gitprovider.New("github", srv.URL+"/api/v3", srv.Client())
		require.NotNil(t, gp)

		_, err := gp.FetchBranchDiff(
			context.Background(),
			"test-token",
			gitprovider.BranchRef{Owner: "org", Repo: "repo", Branch: "feat"},
		)
		assert.ErrorIs(t, err, gitprovider.ErrDiffTooLarge)
	})
}

func TestEscapePathPreserveSlashes(t *testing.T) {
	t.Parallel()
	// The function is unexported, so test it indirectly via BuildBranchURL.
	// A branch with a space in a segment should be escaped, but slashes preserved.
	gp := gitprovider.New("github", "", nil)
	require.NotNil(t, gp)
	got := gp.BuildBranchURL("owner", "repo", "feat/my thing")
	assert.Equal(t, "https://github.com/owner/repo/tree/feat/my%20thing", got)
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	clk := quartz.NewMock(t)
	clk.Set(time.Now())

	t.Run("RetryAfterSeconds", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "120")
		d := gitprovider.ParseRetryAfter(h, clk)
		assert.Equal(t, 120*time.Second, d)
	})

	t.Run("XRatelimitReset", func(t *testing.T) {
		t.Parallel()
		future := clk.Now().Add(90 * time.Second)
		t.Logf("now: %d future: %d", clk.Now().Unix(), future.Unix())
		h := http.Header{}
		h.Set("X-Ratelimit-Reset", strconv.FormatInt(future.Unix(), 10))
		d := gitprovider.ParseRetryAfter(h, clk)
		assert.WithinDuration(t, future, clk.Now().Add(d), time.Second)
	})

	t.Run("NoHeaders", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		d := gitprovider.ParseRetryAfter(h, clk)
		assert.Equal(t, time.Duration(0), d)
	})

	t.Run("InvalidValue", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "not-a-number")
		d := gitprovider.ParseRetryAfter(h, clk)
		assert.Equal(t, time.Duration(0), d)
	})

	t.Run("RetryAfterTakesPrecedence", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "60")
		h.Set("X-Ratelimit-Reset", strconv.FormatInt(
			clk.Now().Unix()+120, 10,
		))
		d := gitprovider.ParseRetryAfter(h, clk)
		assert.Equal(t, 60*time.Second, d)
	})
}
