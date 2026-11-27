package gitprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// githubStatusServer serves the issue, pull, and reviews endpoints and counts
// requests by endpoint. The pull ETag changes on every request, as when
// unrelated branches are pushed, unless pullETag is set. The issue and
// reviews ETags change only with their own content.
type githubStatusServer struct {
	mu      sync.Mutex
	issue   map[string]any
	pull    map[string]any
	reviews []map[string]any
	// issueVersion and reviewVersion number the issue and reviews ETags.
	issueVersion  int
	reviewVersion int
	// pullRequests counts pull requests and numbers the changing pull ETag.
	pullRequests int
	// pullETag, when set, replaces the changing pull ETag.
	pullETag string
	// requests counts requests by endpoint. charged counts the requests
	// that cost primary rate limit: every response except a 304.
	requests map[string]int
	charged  map[string]int
	// issueHeaders replace the default rate-limit headers of the same name on
	// issue responses.
	issueHeaders http.Header
	// A non-zero status makes every request to that endpoint fail with it.
	issueStatus  int
	pullStatus   int
	reviewStatus int
}

func newGitHubStatusServer(t *testing.T) (*githubStatusServer, *httptest.Server) {
	t.Helper()

	updatedAt := "2026-11-13T12:00:00Z"
	state := &githubStatusServer{
		issue: map[string]any{
			"updated_at": updatedAt,
			"title":      "Original title",
			"state":      "open",
			"draft":      false,
			"pull_request": map[string]any{
				"merged_at": nil,
			},
			"user": map[string]any{
				"login":      "author",
				"avatar_url": "https://example.com/author.png",
			},
		},
		pull: map[string]any{
			"updated_at":    updatedAt,
			"title":         "Original title",
			"state":         "open",
			"draft":         false,
			"merged":        false,
			"number":        1,
			"additions":     10,
			"deletions":     5,
			"changed_files": 3,
			"commits":       2,
			"head": map[string]any{
				"sha": "abc123",
				"ref": "feature",
			},
			"base": map[string]any{
				"ref": "main",
			},
			"user": map[string]any{
				"login":      "author",
				"avatar_url": "https://example.com/author.png",
			},
		},
		reviews:  []map[string]any{},
		requests: make(map[string]int),
		charged:  make(map[string]int),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()

		resetAt := time.Now().Add(30 * time.Minute)
		resetTimestamp := fmt.Sprint(resetAt.Unix())
		w.Header().Set("X-Ratelimit-Limit", "15000")
		w.Header().Set("X-Ratelimit-Remaining", "14999")
		w.Header().Set("X-Ratelimit-Reset", resetTimestamp)

		assert.Contains(t, []string{"Bearer test-token", "Bearer token-a", "Bearer token-b"}, r.Header.Get("Authorization"))
		var endpoint, etag string
		var body any
		var status int
		switch r.URL.Path {
		case "/api/v3/repos/owner/repo/issues/1", "/api/v3/repos/owner/repo/issues/2", "/api/v3/repos/owner/other/issues/1":
			for name, values := range state.issueHeaders {
				w.Header()[name] = values
			}
			if validator := r.Header.Get("If-None-Match"); validator != "" {
				assert.True(t, strings.HasPrefix(validator, `"issue-`), "issue request must use an issue ETag")
			}
			endpoint = "issue"
			etag = fmt.Sprintf(`"issue-%d"`, state.issueVersion)
			body = state.issue
			status = state.issueStatus
		case "/api/v3/repos/owner/repo/pulls/1", "/api/v3/repos/owner/repo/pulls/2", "/api/v3/repos/owner/other/pulls/1":
			endpoint = "pull"
			state.pullRequests++
			etag = fmt.Sprintf(`"pull-%d"`, state.pullRequests)
			if state.pullETag != "" {
				etag = state.pullETag
			}
			pull := maps.Clone(state.pull)
			if strings.HasSuffix(r.URL.Path, "/2") {
				pull["number"] = 2
			}
			body = pull
			status = state.pullStatus
		case "/api/v3/repos/owner/repo/pulls/1/reviews", "/api/v3/repos/owner/repo/pulls/2/reviews", "/api/v3/repos/owner/other/pulls/1/reviews":
			endpoint = "reviews"
			assert.Equal(t, "100", r.URL.Query().Get("per_page"))
			etag = fmt.Sprintf(`"reviews-%d"`, state.reviewVersion)
			body = state.reviews
			status = state.reviewStatus
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		state.requests[endpoint]++
		if status != 0 {
			state.charged[endpoint]++
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"message":%q}`, endpoint+" request error")
			return
		}
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		state.charged[endpoint]++
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(body))
	}))
	t.Cleanup(srv.Close)
	return state, srv
}

func (s *githubStatusServer) change(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// lastPullETag returns the changing pull ETag of the latest pull response.
// Setting pullETag to it makes the next conditional pull request return 304.
// The caller holds s.mu.
func (s *githubStatusServer) lastPullETag() string {
	return fmt.Sprintf(`"pull-%d"`, s.pullRequests)
}

// assertRequests checks the requests by endpoint since the last call, then
// resets the counts. A nil charged skips the check of charged requests.
func (s *githubStatusServer) assertRequests(t *testing.T, requests, charged map[string]int) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Equal(t, requests, s.requests, "requests by endpoint")
	if charged != nil {
		assert.Equal(t, charged, s.charged, "charged requests by endpoint")
	}
	clear(s.requests)
	clear(s.charged)
}

func TestGitHubStatusIssueGate(t *testing.T) {
	t.Parallel()

	state, srv := newGitHubStatusServer(t)
	clock := quartz.NewMock(t)
	clock.Set(time.Date(2026, 11, 13, 12, 1, 0, 0, time.UTC))
	provider, err := New("github", srv.URL+"/api/v3", srv.Client(), WithClock(clock))
	require.NoError(t, err)
	ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
	fetch := func() *PRStatus {
		t.Helper()
		status, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
		require.NoError(t, err)
		return status
	}

	first := fetch()
	require.Equal(t, &PRStatus{
		Title:           "Original title",
		State:           PRStateOpen,
		HeadSHA:         "abc123",
		HeadBranch:      "feature",
		BaseBranch:      "main",
		PRNumber:        1,
		Commits:         2,
		AuthorLogin:     "author",
		AuthorAvatarURL: "https://example.com/author.png",
		DiffStats: DiffStats{
			Additions:    10,
			Deletions:    5,
			ChangedFiles: 3,
		},
		FetchedAt: clock.Now(),
	}, first)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	clock.Advance(2 * time.Minute)
	unchanged := fetch()
	first.FetchedAt = clock.Now()
	require.Equal(t, first, unchanged)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})

	changes := []struct {
		name  string
		issue map[string]any
		pull  map[string]any
		check func(*PRStatus)
	}{
		{
			name:  "Rename",
			issue: map[string]any{"title": "Renamed"},
			pull:  map[string]any{"title": "Renamed"},
			check: func(s *PRStatus) { assert.Equal(t, "Renamed", s.Title) },
		},
		{
			name:  "Draft",
			issue: map[string]any{"draft": true},
			pull:  map[string]any{"draft": true},
			check: func(s *PRStatus) { assert.True(t, s.Draft) },
		},
		{
			name:  "Ready",
			issue: map[string]any{"draft": false},
			pull:  map[string]any{"draft": false},
			check: func(s *PRStatus) { assert.False(t, s.Draft) },
		},
		{
			name:  "Close",
			issue: map[string]any{"state": "closed"},
			pull:  map[string]any{"state": "closed"},
			check: func(s *PRStatus) { assert.Equal(t, PRStateClosed, s.State) },
		},
		{
			name:  "Reopen",
			issue: map[string]any{"state": "open"},
			pull:  map[string]any{"state": "open"},
			check: func(s *PRStatus) { assert.Equal(t, PRStateOpen, s.State) },
		},
		{
			name: "Head",
			pull: map[string]any{"head": map[string]any{"sha": "def456", "ref": "new-feature"}},
			check: func(s *PRStatus) {
				assert.Equal(t, "def456", s.HeadSHA)
				assert.Equal(t, "new-feature", s.HeadBranch)
			},
		},
		{
			name:  "Base",
			pull:  map[string]any{"base": map[string]any{"ref": "release"}},
			check: func(s *PRStatus) { assert.Equal(t, "release", s.BaseBranch) },
		},
		{
			name: "Stats",
			pull: map[string]any{"additions": 100, "deletions": 50, "changed_files": 30, "commits": 20},
			check: func(s *PRStatus) {
				assert.Equal(t, DiffStats{Additions: 100, Deletions: 50, ChangedFiles: 30}, s.DiffStats)
				assert.Equal(t, int32(20), s.Commits)
			},
		},
		{
			name:  "Author",
			issue: map[string]any{"user": map[string]any{"login": "new-author", "avatar_url": "https://example.com/new.png"}},
			pull:  map[string]any{"user": map[string]any{"login": "new-author", "avatar_url": "https://example.com/new.png"}},
			check: func(s *PRStatus) {
				assert.Equal(t, "new-author", s.AuthorLogin)
				assert.Equal(t, "https://example.com/new.png", s.AuthorAvatarURL)
			},
		},
		{
			name: "Merge",
			issue: map[string]any{
				"state":        "closed",
				"pull_request": map[string]any{"merged_at": "2026-11-13T12:05:00Z"},
			},
			pull:  map[string]any{"state": "closed", "merged": true},
			check: func(s *PRStatus) { assert.Equal(t, PRStateMerged, s.State) },
		},
	}
	for i, change := range changes {
		t.Logf("PR change: %s", change.name)
		// Every change also updates the issue timestamp and ETag, including
		// changes to fields that only the pull returns.
		state.change(func() {
			for key, value := range change.issue {
				state.issue[key] = value
			}
			for key, value := range change.pull {
				state.pull[key] = value
			}
			state.issueVersion++
			updatedAt := clock.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339)
			state.issue["updated_at"] = updatedAt
			state.pull["updated_at"] = updatedAt
		})
		change.check(fetch())
		state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1})
		change.check(fetch())
		state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})
	}

	state.change(func() {
		state.reviewVersion++
		state.reviews = []map[string]any{{"id": 1, "state": "APPROVED", "user": map[string]any{"login": "reviewer"}}}
	})
	reviewed := fetch()
	assert.True(t, reviewed.Approved)
	assert.Equal(t, int32(1), reviewed.ReviewerCount)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{"reviews": 1})
}

func TestGitHubStatusPullOnlyChange(t *testing.T) {
	t.Parallel()

	state, srv := newGitHubStatusServer(t)
	clock := quartz.NewMock(t)
	clock.Set(time.Date(2026, 11, 13, 12, 1, 0, 0, time.UTC))
	provider, err := New("github", srv.URL+"/api/v3", srv.Client(), WithClock(clock))
	require.NoError(t, err)
	ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	// A pull change that leaves the issue unchanged is visible once the
	// snapshot reaches githubPullRequestMaxAge.
	state.change(func() {
		state.pull["additions"] = 200
		state.pull["base"] = map[string]any{"ref": "new-base"}
	})
	clock.Advance(59 * time.Minute)
	before, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	assert.Equal(t, int32(10), before.DiffStats.Additions)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})

	clock.Advance(time.Minute)
	after, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	assert.Equal(t, int32(200), after.DiffStats.Additions)
	assert.Equal(t, "new-base", after.BaseBranch)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"pull": 1})

	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})

	// A quiet repository's periodic pull request is also conditional, and a
	// 304 starts a new maximum-age window without charging a request.
	state.change(func() { state.pullETag = state.lastPullETag() })
	clock.Advance(time.Hour)
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{})
	clock.Advance(59 * time.Minute)
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})
}

func TestGitHubStatusEndpointLag(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// issue holds the issue fields that change before the pull catches up.
		issue map[string]any
		// lagWithNotModified makes the lagging pull return 304.
		lagWithNotModified bool
	}{
		{
			name:  "Timestamp",
			issue: map[string]any{"updated_at": "2026-11-13T12:01:00Z"},
		},
		{
			name:               "TimestampNotModified",
			issue:              map[string]any{"updated_at": "2026-11-13T12:01:00Z"},
			lagWithNotModified: true,
		},
		{
			name:  "Title",
			issue: map[string]any{"title": "Renamed"},
		},
		{
			name:  "State",
			issue: map[string]any{"state": "closed"},
		},
		{
			name:  "Draft",
			issue: map[string]any{"draft": true},
		},
		{
			name:  "Merged",
			issue: map[string]any{"pull_request": map[string]any{"merged_at": "2026-11-13T12:01:00Z"}},
		},
		{
			name:  "AuthorLogin",
			issue: map[string]any{"user": map[string]any{"login": "new-author", "avatar_url": "https://example.com/author.png"}},
		},
		{
			name:  "AuthorAvatarURL",
			issue: map[string]any{"user": map[string]any{"login": "author", "avatar_url": "https://example.com/new.png"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, srv := newGitHubStatusServer(t)
			provider, err := New("github", srv.URL+"/api/v3", srv.Client())
			require.NoError(t, err)
			ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

			// The issue changes while the pull still returns the old fields.
			state.change(func() {
				state.issueVersion++
				for key, value := range tc.issue {
					state.issue[key] = value
				}
				if tc.lagWithNotModified {
					state.pullETag = state.lastPullETag()
				}
			})
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			charged := map[string]int{"issue": 1, "pull": 1}
			if tc.lagWithNotModified {
				charged = map[string]int{"issue": 1}
			}
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, charged)

			// The pull catches up with the issue and changes its ETag. The
			// disagreement left the snapshot expired, so the pull is requested.
			state.change(func() {
				state.pullETag = ""
				state.pull["updated_at"] = state.issue["updated_at"]
				state.pull["title"] = state.issue["title"]
				state.pull["state"] = state.issue["state"]
				state.pull["draft"] = state.issue["draft"]
				state.pull["user"] = state.issue["user"]
				state.pull["merged"] = tc.name == "Merged"
				state.pull["additions"] = 200
			})
			after, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			assert.Equal(t, int32(200), after.DiffStats.Additions)
			if tc.name == "AuthorLogin" {
				assert.Equal(t, "new-author", after.AuthorLogin)
			}
			if tc.name == "AuthorAvatarURL" {
				assert.Equal(t, "https://example.com/new.png", after.AuthorAvatarURL)
			}
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"pull": 1})

			// Once they agree, the issue gates the pull again.
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})
		})
	}
}

func TestGitHubStatusTokenIsolationAndEviction(t *testing.T) {
	t.Parallel()

	state, srv := newGitHubStatusServer(t)
	// Large descriptions and an embedded repository, which the snapshot must
	// not keep.
	state.change(func() {
		state.issue["body"] = strings.Repeat("description", 5_000)
		state.pull["body"] = state.issue["body"]
		state.pull["head"] = map[string]any{
			"sha": "abc123", "ref": "feature",
			"repo": map[string]any{"description": state.issue["body"]},
		}
	})
	provider, err := New("github", srv.URL+"/api/v3", srv.Client())
	require.NoError(t, err)
	github, ok := provider.(*githubProvider)
	require.True(t, ok)
	// One token's lookup stores two entries, the snapshot and the reviews, so
	// a lookup with another token evicts both.
	github.cache = newResponseCache(2)
	ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
	first, err := provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})
	github.cache.mu.Lock()
	assert.Len(t, github.cache.entries, 2)
	for _, element := range github.cache.entries {
		assert.Less(t, len(cachedEntry(element).body), 1_024, "descriptions and repositories must not be retained")
	}
	github.cache.mu.Unlock()

	_, err = provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})

	// token-b gets its own pull response, not token-a's snapshot.
	state.change(func() { state.pull["head"] = map[string]any{"sha": "private-sha", "ref": "private-branch"} })
	second, err := provider.FetchPullRequestStatus(t.Context(), "token-b", ref)
	require.NoError(t, err)
	assert.NotEqual(t, first.HeadSHA, second.HeadSHA)
	assert.Equal(t, "private-sha", second.HeadSHA)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	// token-b's lookup evicted token-a's entries, so every request is charged.
	state.change(func() { state.pull["head"] = map[string]any{"sha": "abc123", "ref": "feature"} })
	third, err := provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	assert.Equal(t, first.HeadSHA, third.HeadSHA)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})
}

func TestGitHubStatusUnchangedRequestCounts(t *testing.T) {
	t.Parallel()

	for _, busy := range []bool{false, true} {
		t.Run(fmt.Sprintf("BusyRepository=%t", busy), func(t *testing.T) {
			t.Parallel()
			state, srv := newGitHubStatusServer(t)
			if !busy {
				state.change(func() { state.pullETag = `"quiet-repository"` })
			}
			clock := quartz.NewMock(t)
			clock.Set(time.Date(2026, 11, 13, 12, 1, 0, 0, time.UTC))
			provider, err := New("github", srv.URL+"/api/v3", srv.Client(), WithClock(clock))
			require.NoError(t, err)
			ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

			// Two hours of lookups every two minutes. Each hour the snapshot
			// reaches githubPullRequestMaxAge once, and that pull request is
			// charged only when the pull ETag changed.
			for range 2 {
				for range 30 {
					clock.Advance(2 * time.Minute)
					status, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
					require.NoError(t, err)
					require.Equal(t, "Original title", status.Title)
				}
				charged := map[string]int{}
				if busy {
					charged["pull"] = 1
				}
				state.assertRequests(t, map[string]int{"issue": 30, "pull": 1, "reviews": 30}, charged)
			}
		})
	}
}

func TestGitHubStatusIssueRejectionFallback(t *testing.T) {
	t.Parallel()

	for _, statusCode := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusGone} {
		t.Run(fmt.Sprint(statusCode), func(t *testing.T) {
			t.Parallel()
			state, srv := newGitHubStatusServer(t)
			state.change(func() {
				state.issueStatus = statusCode
				state.pullETag = `"unchanged-pull"`
				state.reviews = []map[string]any{{"id": 1, "state": "APPROVED", "user": map[string]any{"login": "reviewer"}}}
			})
			clock := quartz.NewMock(t)
			clock.Set(time.Date(2026, 11, 13, 12, 1, 0, 0, time.UTC))
			provider, err := New("github", srv.URL+"/api/v3", srv.Client(), WithClock(clock))
			require.NoError(t, err)
			ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
			want := &PRStatus{
				Title:           "Original title",
				State:           PRStateOpen,
				HeadSHA:         "abc123",
				HeadBranch:      "feature",
				BaseBranch:      "main",
				PRNumber:        1,
				Commits:         2,
				AuthorLogin:     "author",
				AuthorAvatarURL: "https://example.com/author.png",
				DiffStats: DiffStats{
					Additions:    10,
					Deletions:    5,
					ChangedFiles: 3,
				},
				Approved:      true,
				ReviewerCount: 1,
				FetchedAt:     clock.Now(),
			}
			// The issue is rejected and the pull is readable, so the rejection
			// is remembered.
			first, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			require.Equal(t, want, first)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

			// While the rejection is remembered, the issue is not requested.
			clock.Advance(59 * time.Minute)
			second, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			want.FetchedAt = clock.Now()
			require.Equal(t, want, second)
			state.assertRequests(t, map[string]int{"pull": 1, "reviews": 1}, map[string]int{})

			// The pull is requested on every lookup, so its changes are visible
			// at once.
			state.change(func() {
				state.pull["head"] = map[string]any{"sha": "updated-head", "ref": "feature"}
				state.pull["additions"] = 20
				state.pullETag = `"updated-pull"`
				state.pull["updated_at"] = clock.Now().Format(time.RFC3339)
				state.issue["updated_at"] = state.pull["updated_at"]
				state.issueVersion++
			})
			changed, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			assert.Equal(t, "updated-head", changed.HeadSHA)
			assert.Equal(t, int32(20), changed.DiffStats.Additions)
			state.assertRequests(t, map[string]int{"pull": 1, "reviews": 1}, map[string]int{"pull": 1})

			// The rejection applies to the repository, not only this PR.
			otherPR, err := provider.FetchPullRequestStatus(t.Context(), "test-token", PRRef{Owner: "owner", Repo: "repo", Number: 2})
			require.NoError(t, err)
			assert.Equal(t, 2, otherPR.PRNumber)
			state.assertRequests(t, map[string]int{"pull": 1, "reviews": 1}, map[string]int{"pull": 1, "reviews": 1})

			// Another repository is not affected.
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", PRRef{Owner: "owner", Repo: "other", Number: 1})
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

			// The rejection expires after githubIssueRejectionTTL, and a
			// readable issue restores the gate.
			state.change(func() { state.issueStatus = 0 })
			clock.Advance(time.Minute)
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1})
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "reviews": 1}, map[string]int{})

			// A new rejection is remembered again.
			state.change(func() { state.issueStatus = statusCode })
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1})
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.assertRequests(t, map[string]int{"pull": 1, "reviews": 1}, map[string]int{})
		})
	}
}

func TestGitHubStatusIssueRejectionIsolationAndEviction(t *testing.T) {
	t.Parallel()

	state, srv := newGitHubStatusServer(t)
	state.change(func() {
		state.issueStatus = http.StatusForbidden
		state.pullETag = `"unchanged-pull"`
	})
	provider, err := New("github", srv.URL+"/api/v3", srv.Client())
	require.NoError(t, err)
	github, ok := provider.(*githubProvider)
	require.True(t, ok)
	github.cache = newResponseCache(6)
	ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
	_, err = provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	// token-a's rejection does not apply to token-b.
	state.change(func() { state.issueStatus = 0 })
	_, err = provider.FetchPullRequestStatus(t.Context(), "token-b", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	// token-a's rejection is still remembered.
	state.change(func() { state.issueStatus = http.StatusForbidden })
	_, err = provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"pull": 1, "reviews": 1}, map[string]int{})

	// Rejection markers compete with responses for the same entry limit. Two
	// more lookups that each store a marker, a snapshot, and reviews evict
	// token-a's entries, including its marker, so its issue is requested again.
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
	require.NoError(t, err)
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", PRRef{Owner: "owner", Repo: "other", Number: 1})
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 2, "pull": 2, "reviews": 2}, map[string]int{"issue": 2, "pull": 2, "reviews": 2})
	github.cache.mu.Lock()
	assert.Len(t, github.cache.entries, 6)
	github.cache.mu.Unlock()
	_, err = provider.FetchPullRequestStatus(t.Context(), "token-a", ref)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})
}

func TestGitHubStatusIssueErrors(t *testing.T) {
	t.Parallel()

	reset := time.Date(2026, 11, 13, 12, 2, 0, 0, time.UTC)
	// Unless headers replace them, issue responses carry the server's
	// default X-Ratelimit-Remaining of 14999 and a reset 30 minutes after the
	// real time, which is later than the test clock.
	for _, tc := range []struct {
		name           string
		status         int
		headers        http.Header
		rateLimited    bool
		transportError bool
		bodyError      bool
	}{
		{
			name:   "RetryAfter",
			status: http.StatusForbidden,
			headers: http.Header{
				"Retry-After": {"60"},
			},
			rateLimited: true,
		},
		{
			name:   "RemainingZero",
			status: http.StatusForbidden,
			headers: http.Header{
				"X-Ratelimit-Remaining": {"0"},
				"X-Ratelimit-Reset":     {fmt.Sprint(reset.Unix())},
			},
			rateLimited: true,
		},
		{
			name:   "TooManyRequests",
			status: http.StatusTooManyRequests,
			headers: http.Header{
				"Retry-After": {"60"},
			},
			rateLimited: true,
		},
		{
			name:        "TooManyRequestsWithReset",
			status:      http.StatusTooManyRequests,
			rateLimited: true,
		},
		{
			name:   "ExhaustedWithoutRetryTime",
			status: http.StatusForbidden,
			headers: http.Header{
				"X-Ratelimit-Remaining": {"0"},
				"X-Ratelimit-Reset":     {""},
			},
		},
		{
			name:   "InvalidRetryWithoutReset",
			status: http.StatusForbidden,
			headers: http.Header{
				"Retry-After":       {"invalid"},
				"X-Ratelimit-Reset": {""},
			},
		},
		{
			name:   "Unauthorized",
			status: http.StatusUnauthorized,
		},
		{
			name:           "Transport",
			transportError: true,
		},
		{
			name:      "ReadError",
			status:    http.StatusForbidden,
			bodyError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, srv := newGitHubStatusServer(t)
			state.change(func() {
				state.issueStatus = tc.status
				state.issueHeaders = tc.headers
			})
			clock := quartz.NewMock(t)
			clock.Set(reset.Add(-time.Minute))
			client := srv.Client()
			var transportRequests atomic.Int64
			if tc.transportError || tc.bodyError {
				transport := client.Transport
				if transport == nil {
					transport = http.DefaultTransport
				}
				client.Transport = testutil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "/issues/") {
						transportRequests.Add(1)
						if tc.transportError {
							return nil, xerrors.New("issue transport error")
						}
						readErr := xerrors.New("issue body error")
						body := iotest.ErrReader(readErr)
						return &http.Response{
							StatusCode: tc.status,
							Header:     make(http.Header),
							Body:       io.NopCloser(body),
						}, nil
					}
					return transport.RoundTrip(req)
				})
			}
			provider, err := New("github", srv.URL+"/api/v3", client, WithClock(clock))
			require.NoError(t, err)
			for range 2 {
				_, err := provider.FetchPullRequestStatus(t.Context(), "test-token", PRRef{Owner: "owner", Repo: "repo", Number: 1})
				require.Error(t, err)
				switch {
				case tc.rateLimited:
					var limit *RateLimitError
					require.ErrorAs(t, err, &limit)
				case tc.transportError:
					require.ErrorContains(t, err, "issue transport error")
				default:
					require.ErrorContains(t, err, fmt.Sprint(tc.status))
				}
			}
			if tc.transportError || tc.bodyError {
				assert.EqualValues(t, 2, transportRequests.Load())
				state.assertRequests(t, map[string]int{}, nil)
			} else {
				state.assertRequests(t, map[string]int{"issue": 2}, nil)
			}
		})
	}
}

func TestGitHubStatusOtherRequestsKeepRateClassification(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"pull", "reviews"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()

			state, srv := newGitHubStatusServer(t)
			// With the server's default future reset header, the shared
			// classifier treats a 403 as a rate limit.
			state.change(func() {
				if endpoint == "pull" {
					state.pullStatus = http.StatusForbidden
				} else {
					state.reviewStatus = http.StatusForbidden
				}
			})
			provider, err := New("github", srv.URL+"/api/v3", srv.Client())
			require.NoError(t, err)
			ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			var limit *RateLimitError
			require.ErrorAs(t, err, &limit)

			requests := map[string]int{"issue": 1, "pull": 1}
			if endpoint == "reviews" {
				requests["reviews"] = 1
			}
			state.assertRequests(t, requests, nil)
		})
	}
}

func TestGitHubStatusMissingPull(t *testing.T) {
	t.Parallel()

	state, srv := newGitHubStatusServer(t)
	provider, err := New("github", srv.URL+"/api/v3", srv.Client())
	require.NoError(t, err)

	healthyRef := PRRef{Owner: "owner", Repo: "repo", Number: 1}
	_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", healthyRef)
	require.NoError(t, err)
	state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})

	state.change(func() {
		state.issueStatus = http.StatusNotFound
		state.pullStatus = http.StatusNotFound
	})
	missingRef := PRRef{Owner: "owner", Repo: "repo", Number: 2}
	for range 2 {
		_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", missingRef)
		require.EqualError(t, err, `github request failed with status 404: {"message":"pull request error"}`)
		state.assertRequests(t, map[string]int{"issue": 1, "pull": 1}, map[string]int{"issue": 1, "pull": 1})
	}

	state.change(func() {
		state.issueStatus = 0
		state.pullStatus = 0
	})
	for range 3 {
		status, err := provider.FetchPullRequestStatus(t.Context(), "test-token", healthyRef)
		require.NoError(t, err)
		assert.Equal(t, 1, status.PRNumber)
	}
	state.assertRequests(t, map[string]int{"issue": 3, "reviews": 3}, map[string]int{})
}

func TestGitHubStatusPersistentDisagreement(t *testing.T) {
	t.Parallel()

	for _, busy := range []bool{false, true} {
		for _, warm := range []bool{false, true} {
			t.Run(fmt.Sprintf("Busy=%t/Warm=%t", busy, warm), func(t *testing.T) {
				t.Parallel()

				state, srv := newGitHubStatusServer(t)
				if !busy {
					state.change(func() { state.pullETag = `"stable-pull"` })
				}
				provider, err := New("github", srv.URL+"/api/v3", srv.Client())
				require.NoError(t, err)
				ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
				if warm {
					_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
					require.NoError(t, err)
					state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"issue": 1, "pull": 1, "reviews": 1})
				}

				// The issue avatar changes and the pull keeps the old one.
				state.change(func() {
					state.issueVersion++
					state.issue["user"] = map[string]any{
						"login":      "author",
						"avatar_url": "https://example.com/updated.png",
					}
				})
				_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
				require.NoError(t, err)
				// A cold lookup charges every endpoint. A warm one charges the
				// changed issue, and the pull only when its ETag changed.
				firstCharges := map[string]int{"issue": 1}
				if !warm {
					firstCharges["reviews"] = 1
				}
				if busy || !warm {
					firstCharges["pull"] = 1
				}
				state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, firstCharges)

				// While they disagree, every lookup checks the pull and returns
				// its fields. The unchanged issue returns 304.
				for range 3 {
					status, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
					require.NoError(t, err)
					assert.Equal(t, "https://example.com/author.png", status.AuthorAvatarURL)
				}
				charges := map[string]int{}
				if busy {
					charges["pull"] = 3
				}
				state.assertRequests(t, map[string]int{"issue": 3, "pull": 3, "reviews": 3}, charges)

				// A pull change is visible on the next lookup.
				state.change(func() {
					state.pull["additions"] = 40
					if !busy {
						state.pullETag = `"updated-pull"`
					}
				})
				latest, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
				require.NoError(t, err)
				assert.Equal(t, int32(40), latest.DiffStats.Additions)
				state.assertRequests(t, map[string]int{"issue": 1, "pull": 1, "reviews": 1}, map[string]int{"pull": 1})
			})
		}
	}
}

func TestGitHubStatusRequestFailure(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"issue", "pull", "reviews"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			state, srv := newGitHubStatusServer(t)
			provider, err := New("github", srv.URL+"/api/v3", srv.Client())
			require.NoError(t, err)
			ref := PRRef{Owner: "owner", Repo: "repo", Number: 1}
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			state.change(func() {
				state.issueVersion++
				state.issue["title"] = "Renamed"
				state.pull["title"] = "Renamed"
				switch endpoint {
				case "issue":
					state.issueStatus = http.StatusInternalServerError
				case "pull":
					state.pullStatus = http.StatusInternalServerError
				case "reviews":
					state.reviewStatus = http.StatusInternalServerError
				}
			})
			_, err = provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.ErrorContains(t, err, "500")
			state.change(func() {
				state.issueStatus = 0
				state.pullStatus = 0
				state.reviewStatus = 0
			})
			after, err := provider.FetchPullRequestStatus(t.Context(), "test-token", ref)
			require.NoError(t, err)
			assert.Equal(t, "Renamed", after.Title)
		})
	}
}
