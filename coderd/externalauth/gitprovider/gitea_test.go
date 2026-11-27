package gitprovider_test

import (
	"errors"
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

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
)

func newGiteaProvider(t *testing.T, apiBaseURL string, client *http.Client) gitprovider.Provider {
	t.Helper()
	gp, err := gitprovider.New("gitea", apiBaseURL, client)
	require.NoError(t, err)
	require.NotNil(t, gp)
	return gp
}

func TestGiteaInvalidAPIBaseURL(t *testing.T) {
	t.Parallel()
	gp, err := gitprovider.New("gitea", "git.example.com/api/v1", nil)
	require.Error(t, err)
	assert.Nil(t, gp)
}

func TestGiteaParseRepositoryOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		apiBaseURL       string
		raw              string
		expectOK         bool
		expectOwner      string
		expectRepo       string
		expectNormalized string
	}{
		{
			name:             "HTTPS",
			raw:              "https://git.example.com/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://git.example.com/coder/coder",
		},
		{
			name:             "HTTPWithPortAndTrailingSlash",
			apiBaseURL:       "http://git.example.com:3000/api/v1",
			raw:              "http://git.example.com:3000/coder/coder/",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "http://git.example.com:3000/coder/coder",
		},
		{
			name:             "SCPStyleSSH",
			raw:              "git@git.example.com:coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://git.example.com/coder/coder",
		},
		{
			// Self-hosted instances commonly serve SSH on a port other
			// than 22, which is not part of the web host.
			name:             "SSHURLWithCustomPort",
			raw:              "ssh://git@git.example.com:2222/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://git.example.com/coder/coder",
		},
		{
			name:             "Subpath",
			apiBaseURL:       "https://example.com/forgejo/api/v1",
			raw:              "https://example.com/forgejo/coder/coder.git",
			expectOK:         true,
			expectOwner:      "coder",
			expectRepo:       "coder",
			expectNormalized: "https://example.com/forgejo/coder/coder",
		},
		{
			name:       "SubpathMissing",
			apiBaseURL: "https://example.com/forgejo/api/v1",
			raw:        "https://example.com/coder/coder.git",
		},
		{
			name: "OtherHost",
			raw:  "https://github.com/coder/coder.git",
		},
		{
			// Gitea has no nested groups, so a third path segment is
			// not a repository root.
			name: "TooManySegments",
			raw:  "https://git.example.com/coder/coder/src/branch/main",
		},
		{
			name: "OwnerOnly",
			raw:  "https://git.example.com/coder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			apiBaseURL := tt.apiBaseURL
			if apiBaseURL == "" {
				apiBaseURL = "https://git.example.com/api/v1"
			}
			gp := newGiteaProvider(t, apiBaseURL, nil)
			owner, repo, normalized, ok := gp.ParseRepositoryOrigin(tt.raw)
			require.Equal(t, tt.expectOK, ok)
			assert.Equal(t, tt.expectOwner, owner)
			assert.Equal(t, tt.expectRepo, repo)
			assert.Equal(t, tt.expectNormalized, normalized)
		})
	}
}

func TestGiteaPullRequestURLs(t *testing.T) {
	t.Parallel()
	gp := newGiteaProvider(t, "https://example.com/gitea/api/v1/", nil)

	tests := []struct {
		name             string
		raw              string
		expectRef        gitprovider.PRRef
		expectOK         bool
		expectNormalized string
	}{
		{
			name:             "Plain",
			raw:              "https://example.com/gitea/coder/coder/pulls/42",
			expectRef:        gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 42},
			expectOK:         true,
			expectNormalized: "https://example.com/gitea/coder/coder/pulls/42",
		},
		{
			name:             "FilesTabQueryAndPunctuation",
			raw:              "https://example.com/gitea/coder/coder/pulls/42/files?style=split#diff).",
			expectRef:        gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 42},
			expectOK:         true,
			expectNormalized: "https://example.com/gitea/coder/coder/pulls/42",
		},
		{
			// GitHub-style /pull/ paths are issues or 404s on Gitea.
			name: "GitHubStylePath",
			raw:  "https://example.com/gitea/coder/coder/pull/42",
		},
		{
			name: "IssueURL",
			raw:  "https://example.com/gitea/coder/coder/issues/42",
		},
		{
			name: "ZeroIndex",
			raw:  "https://example.com/gitea/coder/coder/pulls/0",
		},
		{
			name: "OtherHost",
			raw:  "https://gitea.com/coder/coder/pulls/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref, ok := gp.ParsePullRequestURL(strings.TrimRight(tt.raw, ")."))
			require.Equal(t, tt.expectOK, ok)
			assert.Equal(t, tt.expectRef, ref)
			assert.Equal(t, tt.expectNormalized, gp.NormalizePullRequestURL(tt.raw))
		})
	}
}

func TestGiteaBuildURLs(t *testing.T) {
	t.Parallel()
	gp := newGiteaProvider(t, "", nil)

	assert.Equal(t, "https://gitea.com/coder/coder", gp.BuildRepositoryURL("coder", "coder"))
	assert.Equal(t, "https://gitea.com/coder/coder/src/branch/feat/new%20thing", gp.BuildBranchURL("coder", "coder", "feat/new thing"))
	assert.Equal(t, "https://gitea.com/coder/coder/pulls/7", gp.BuildPullRequestURL(gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 7}))
	assert.Empty(t, gp.BuildBranchURL("coder", "coder", ""))
	assert.Empty(t, gp.BuildPullRequestURL(gitprovider.PRRef{Owner: "coder", Repo: "coder"}))
}

func TestGiteaFetchPullRequestStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/repos/coder/coder/pulls/42":
			_, _ = fmt.Fprint(w, `{
				"number": 42, "title": "feat: add thing", "state": "closed",
				"merged": true, "draft": false,
				"additions": 10, "deletions": 3, "changed_files": 2,
				"head": {"ref": "feat/thing", "sha": "abc123", "repo": {"full_name": "coder/coder"}},
				"base": {"ref": "main"},
				"user": {"login": "alice", "avatar_url": "https://git.example.com/avatars/alice"}
			}`)
		case "/api/v1/repos/coder/coder/pulls/42/commits":
			assert.Equal(t, "1", r.URL.Query().Get("limit"))
			w.Header().Set("X-Total-Count", "5")
			_, _ = fmt.Fprint(w, `[{"sha": "abc123"}]`)
		case "/api/v1/repos/coder/coder/pulls/42/reviews":
			// bob requested changes, then approved. carol approved but
			// the approval was dismissed. dave only commented. The
			// review with a null user belongs to a deleted account.
			_, _ = fmt.Fprint(w, `[
				{"id": 1, "state": "REQUEST_CHANGES", "dismissed": false, "user": {"login": "bob"}},
				{"id": 2, "state": "APPROVED", "dismissed": true, "user": {"login": "carol"}},
				{"id": 3, "state": "COMMENT", "dismissed": false, "user": {"login": "dave"}},
				{"id": 4, "state": "APPROVED", "dismissed": false, "user": {"login": "bob"}},
				{"id": 5, "state": "APPROVED", "dismissed": false, "user": null}
			]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
	ctx := testutil.Context(t, testutil.WaitShort)
	status, err := gp.FetchPullRequestStatus(ctx, "test-token", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 42})
	require.NoError(t, err)

	assert.Equal(t, "feat: add thing", status.Title)
	assert.Equal(t, gitprovider.PRStateMerged, status.State)
	assert.Equal(t, "abc123", status.HeadSHA)
	assert.Equal(t, "feat/thing", status.HeadBranch)
	assert.Equal(t, "main", status.BaseBranch)
	assert.Equal(t, gitprovider.DiffStats{Additions: 10, Deletions: 3, ChangedFiles: 2}, status.DiffStats)
	assert.Equal(t, int32(5), status.Commits)
	assert.Equal(t, "alice", status.AuthorLogin)
	assert.Equal(t, 42, status.PRNumber)
	assert.True(t, status.Approved)
	assert.False(t, status.ChangesRequested)
	assert.Equal(t, int32(2), status.ReviewerCount, "bob and carol left decisive reviews")
	assert.WithinDuration(t, time.Now(), status.FetchedAt, time.Minute)
}

func TestGiteaFetchPullRequestStatusReviews(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		reviews          string
		approved         bool
		changesRequested bool
		reviewerCount    int32
	}{
		{
			name: "ChangesRequestedOutstanding",
			reviews: `[
				{"id": 1, "state": "APPROVED", "user": {"login": "bob"}},
				{"id": 2, "state": "REQUEST_CHANGES", "user": {"login": "carol"}}
			]`,
			changesRequested: true,
			reviewerCount:    2,
		},
		{
			// A dismissed approval is decisive but does not approve.
			name:          "OnlyApprovalDismissed",
			reviews:       `[{"id": 1, "state": "APPROVED", "dismissed": true, "user": {"login": "bob"}}]`,
			reviewerCount: 1,
		},
		{
			name: "LatestReviewPerReviewerWins",
			reviews: `[
				{"id": 2, "state": "APPROVED", "user": {"login": "bob"}},
				{"id": 1, "state": "REQUEST_CHANGES", "user": {"login": "bob"}}
			]`,
			approved:      true,
			reviewerCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/repos/coder/coder/pulls/7":
					_, _ = fmt.Fprint(w, `{"number": 7, "state": "open", "draft": true, "head": {"ref": "wip"}, "user": null}`)
				case "/api/v1/repos/coder/coder/pulls/7/commits":
					// No X-Total-Count header: fall back to the page length.
					_, _ = fmt.Fprint(w, `[{"sha": "a"}]`)
				case "/api/v1/repos/coder/coder/pulls/7/reviews":
					_, _ = fmt.Fprint(w, tt.reviews)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
			status, err := gp.FetchPullRequestStatus(testutil.Context(t, testutil.WaitShort), "", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 7})
			require.NoError(t, err)

			assert.True(t, status.Draft)
			assert.Equal(t, int32(1), status.Commits)
			assert.Empty(t, status.AuthorLogin)
			assert.Equal(t, tt.approved, status.Approved)
			assert.Equal(t, tt.changesRequested, status.ChangesRequested)
			assert.Equal(t, tt.reviewerCount, status.ReviewerCount)
		})
	}
}

func TestGiteaResolveBranchPullRequest(t *testing.T) {
	t.Parallel()

	pull := func(number int, ref, fullName string) string {
		return fmt.Sprintf(`{"number": %d, "head": {"ref": %q, "repo": {"full_name": %q}}}`, number, ref, fullName)
	}

	t.Run("ServerFiltersByHead", func(t *testing.T) {
		t.Parallel()
		// Forgejo applies the head filter, so the first page holds
		// only matching pull requests.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/repos/coder/coder/pulls", r.URL.Path)
			assert.Equal(t, "open", r.URL.Query().Get("state"))
			assert.Equal(t, "feat/thing", r.URL.Query().Get("head"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, "["+pull(12, "feat/thing", "coder/coder")+"]")
		}))
		defer srv.Close()

		gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
		ref, err := gp.ResolveBranchPullRequest(testutil.Context(t, testutil.WaitShort), "", gitprovider.BranchRef{Owner: "coder", Repo: "coder", Branch: "feat/thing"})
		require.NoError(t, err)
		require.NotNil(t, ref)
		assert.Equal(t, gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 12}, *ref)
	})

	t.Run("ServerIgnoresHeadFilter", func(t *testing.T) {
		t.Parallel()
		// Gitea ignores head and returns every open pull request, so
		// the provider filters and pages. A fork's pull request with
		// the same branch name must not match.
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			var items []string
			switch page {
			case 1:
				items = append(items, pull(30, "feat/thing", "someone/coder"))
				for i := range 49 {
					items = append(items, pull(100+i, fmt.Sprintf("other-%d", i), "coder/coder"))
				}
			case 2:
				items = append(items, pull(29, "feat/thing", "coder/coder"))
			}
			_, _ = fmt.Fprint(w, "["+strings.Join(items, ",")+"]")
		}))
		defer srv.Close()

		gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
		ref, err := gp.ResolveBranchPullRequest(testutil.Context(t, testutil.WaitShort), "", gitprovider.BranchRef{Owner: "coder", Repo: "coder", Branch: "feat/thing"})
		require.NoError(t, err)
		require.NotNil(t, ref)
		assert.Equal(t, 29, ref.Number)
		assert.Equal(t, int32(2), requests.Load())
	})

	t.Run("NoOpenPullRequest", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, "["+pull(3, "other", "coder/coder")+"]")
		}))
		defer srv.Close()

		gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
		ref, err := gp.ResolveBranchPullRequest(testutil.Context(t, testutil.WaitShort), "", gitprovider.BranchRef{Owner: "coder", Repo: "coder", Branch: "feat/thing"})
		require.NoError(t, err)
		assert.Nil(t, ref)
	})
}

func TestGiteaFetchPullRequestDiff(t *testing.T) {
	t.Parallel()

	const diff = "diff --git a/a.txt b/a.txt\n+hello\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/coder/coder/pulls/1.diff":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprint(w, diff)
		case "/api/v1/repos/coder/coder/pulls/2.diff":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(strings.Repeat("x", gitprovider.MaxDiffSize+1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
	ctx := testutil.Context(t, testutil.WaitShort)

	got, err := gp.FetchPullRequestDiff(ctx, "", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 1})
	require.NoError(t, err)
	assert.Equal(t, diff, got)

	_, err = gp.FetchPullRequestDiff(ctx, "", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 2})
	require.ErrorIs(t, err, gitprovider.ErrDiffTooLarge)

	_, err = gp.FetchPullRequestDiff(ctx, "", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 3})
	require.ErrorContains(t, err, "status 404")
}

func TestGiteaFetchBranchDiff(t *testing.T) {
	t.Parallel()

	const diff = "diff --git a/a.txt b/a.txt\n+hello\n"

	tests := []struct {
		name string
		// rawCompareAPI is true for servers whose compare API honors
		// output=diff (Gitea 1.27 and later).
		rawCompareAPI bool
	}{
		{name: "CompareAPIReturnsDiff", rawCompareAPI: true},
		{name: "CompareAPIReturnsJSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case "/api/v1/repos/coder/coder":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"default_branch": "main"}`)
				case "/api/v1/repos/coder/coder/compare/main...feat/thing":
					assert.Equal(t, "diff", r.URL.Query().Get("output"))
					if tt.rawCompareAPI {
						w.Header().Set("Content-Type", "text/plain; charset=utf-8")
						_, _ = fmt.Fprint(w, diff)
						return
					}
					w.Header().Set("Content-Type", "application/json;charset=utf-8")
					_, _ = fmt.Fprint(w, `{"total_commits": 1, "commits": [], "files": []}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
			got, err := gp.FetchBranchDiff(testutil.Context(t, testutil.WaitShort), "", gitprovider.BranchRef{Owner: "coder", Repo: "coder", Branch: "feat/thing"})
			if !tt.rawCompareAPI {
				require.ErrorIs(t, err, errors.ErrUnsupported)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, diff, got)
		})
	}
}

func TestGiteaRateLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	gp := newGiteaProvider(t, srv.URL+"/api/v1", srv.Client())
	_, err := gp.FetchPullRequestStatus(testutil.Context(t, testutil.WaitShort), "", gitprovider.PRRef{Owner: "coder", Repo: "coder", Number: 1})
	var rlErr *gitprovider.RateLimitError
	require.ErrorAs(t, err, &rlErr)
	assert.WithinDuration(t, time.Now().Add(60*time.Second+gitprovider.RateLimitPadding), rlErr.RetryAfter, 10*time.Second)
}
