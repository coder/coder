package gitprovider_test

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
)

// newGitLabVCR creates a go-vcr recorder for GitLab integration tests.
// In replay mode (default), it serves responses from the cassette file.
// When GITLAB_UPDATE_GOLDEN=true, it records live responses to the cassette.
func newGitLabVCR(t *testing.T, cassetteName string) *recorder.Recorder {
	t.Helper()

	mode := recorder.ModeReplayOnly
	if update, _ := strconv.ParseBool(os.Getenv("GITLAB_UPDATE_GOLDEN")); update {
		mode = recorder.ModeRecordOnly
	}

	rec, err := recorder.New(
		"testdata/gitlab_cassettes/"+cassetteName,
		recorder.WithMode(mode),
		recorder.WithSkipRequestLatency(true),
		// Match only on method + URL; the default matcher is too strict
		// (compares proto, all headers, etc.) and breaks replay.
		// TODO: consider verifying that an Authorization header is present
		// during replay to catch auth-wiring regressions.
		recorder.WithMatcher(func(r *http.Request, i cassette.Request) bool {
			return r.Method == i.Method && r.URL.String() == i.URL
		}),
		// Strip headers down to an allowlist to reduce cassette noise.
		recorder.WithHook(func(i *cassette.Interaction) error {
			allowedRequestHeaders := map[string]struct{}{
				"Accept":       {},
				"Content-Type": {},
			}
			for h := range i.Request.Headers {
				if _, ok := allowedRequestHeaders[h]; !ok {
					i.Request.Headers[h] = []string{"stripped"}
				}
			}

			allowedResponseHeaders := map[string]struct{}{
				"Content-Type": {},
				"X-Total":      {},
			}
			for h := range i.Response.Headers {
				if _, ok := allowedResponseHeaders[h]; !ok {
					i.Response.Headers[h] = []string{"stripped"}
				}
			}
			return nil
		}, recorder.AfterCaptureHook),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	return rec
}

// TestGitLabIntegration exercises every gitprovider.Provider method
// against recorded GitLab API responses (go-vcr cassettes).
//
// To update cassettes from live GitLab:
//
//	GITLAB_UPDATE_GOLDEN=true GITLAB_TOKEN=<pat> go test ./coderd/externalauth/gitprovider/ -run TestGitLabIntegration -count=1
//
// Fixtures:
//
//  1. https://gitlab.com/test-group9945421/test-project/-/merge_requests/3
//     Simple namespace (single-level group).
//     State: open. Same-repo MR, 1 file, mergeable.
//
//  2. https://gitlab.com/test-group9945421/test-project/-/merge_requests/2
//     Simple namespace (single-level group).
//     State: open. Same-repo MR, 1 file, has conflicts.
//
//  3. https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1
//     Nested group (multi-level namespace: test-group9945421/test-subgroup).
//     State: merged. Same-repo MR, 1 file. Source branch deleted after merge.
//
//  4. https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/3
//     Nested group. State: closed (not merged). From a fork.
//     Source branch "forked" does not exist on the target project.
func TestGitLabIntegration(t *testing.T) {
	t.Parallel()

	apiURL := "https://gitlab.com"

	// Token is only used when recording (GITLAB_UPDATE_GOLDEN=true).
	token := os.Getenv("GITLAB_TOKEN")

	// URL parsing tests don't need VCR (no API calls).
	provider, err := gitprovider.New("gitlab", apiURL, http.DefaultClient)
	require.NoError(t, err)
	require.NotNil(t, provider, "gitprovider.New returned nil for \"gitlab\"")

	// --- URL parsing (no API calls) ---

	t.Run("ParseRepositoryOrigin", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name             string
			raw              string
			expectOK         bool
			expectOwner      string
			expectRepo       string
			expectNormalized string
		}{
			{
				name:             "HTTPS simple",
				raw:              "https://gitlab.com/test-group9945421/test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421",
				expectRepo:       "test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:             "HTTPS no .git",
				raw:              "https://gitlab.com/test-group9945421/test-project",
				expectOK:         true,
				expectOwner:      "test-group9945421",
				expectRepo:       "test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:             "HTTPS trailing slash",
				raw:              "https://gitlab.com/test-group9945421/test-project/",
				expectOK:         true,
				expectOwner:      "test-group9945421",
				expectRepo:       "test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:             "SSH",
				raw:              "git@gitlab.com:test-group9945421/test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421",
				expectRepo:       "test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:             "SSH prefix",
				raw:              "ssh://git@gitlab.com/test-group9945421/test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421",
				expectRepo:       "test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:             "Nested group HTTPS",
				raw:              "https://gitlab.com/test-group9945421/test-subgroup/another-test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421/test-subgroup",
				expectRepo:       "another-test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
			},
			{
				name:             "Nested group HTTPS no .git",
				raw:              "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
				expectOK:         true,
				expectOwner:      "test-group9945421/test-subgroup",
				expectRepo:       "another-test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
			},
			{
				name:             "Nested group SSH",
				raw:              "git@gitlab.com:test-group9945421/test-subgroup/another-test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421/test-subgroup",
				expectRepo:       "another-test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
			},
			{
				name:             "Nested group SSH prefix",
				raw:              "ssh://git@gitlab.com/test-group9945421/test-subgroup/another-test-project.git",
				expectOK:         true,
				expectOwner:      "test-group9945421/test-subgroup",
				expectRepo:       "another-test-project",
				expectNormalized: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
			},
			{
				name:     "GitHub does not match",
				raw:      "https://github.com/coder/coder",
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
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				owner, repo, normalized, ok := provider.ParseRepositoryOrigin(tt.raw)
				assert.Equal(t, tt.expectOK, ok)
				if tt.expectOK {
					assert.Equal(t, tt.expectOwner, owner)
					assert.Equal(t, tt.expectRepo, repo)
					assert.Equal(t, tt.expectNormalized, normalized)
				}
			})
		}
	})

	t.Run("ParsePullRequestURL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			raw          string
			expectOK     bool
			expectOwner  string
			expectRepo   string
			expectNumber int
		}{
			{
				name:         "Simple namespace",
				raw:          "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3",
				expectOK:     true,
				expectOwner:  "test-group9945421",
				expectRepo:   "test-project",
				expectNumber: 3,
			},
			{
				name:         "Nested group",
				raw:          "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1",
				expectOK:     true,
				expectOwner:  "test-group9945421/test-subgroup",
				expectRepo:   "another-test-project",
				expectNumber: 1,
			},
			{
				name:         "Nested group second MR",
				raw:          "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/3",
				expectOK:     true,
				expectOwner:  "test-group9945421/test-subgroup",
				expectRepo:   "another-test-project",
				expectNumber: 3,
			},
			{
				name:         "With query string",
				raw:          "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3?tab=diffs",
				expectOK:     true,
				expectOwner:  "test-group9945421",
				expectRepo:   "test-project",
				expectNumber: 3,
			},
			{
				name:         "With fragment",
				raw:          "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1#note_123",
				expectOK:     true,
				expectOwner:  "test-group9945421/test-subgroup",
				expectRepo:   "another-test-project",
				expectNumber: 1,
			},
			{
				name:         "With path suffix (diffs tab)",
				raw:          "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/3/diffs",
				expectOK:     true,
				expectOwner:  "test-group9945421/test-subgroup",
				expectRepo:   "another-test-project",
				expectNumber: 3,
			},
			{
				name:     "GitHub PR does not match",
				raw:      "https://github.com/coder/coder/pull/123",
				expectOK: false,
			},
			{
				name:     "Not a MR URL",
				raw:      "https://gitlab.com/test-group9945421/test-project/-/issues/1",
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
				ref, ok := provider.ParsePullRequestURL(tt.raw)
				assert.Equal(t, tt.expectOK, ok)
				if tt.expectOK {
					assert.Equal(t, tt.expectOwner, ref.Owner)
					assert.Equal(t, tt.expectRepo, ref.Repo)
					assert.Equal(t, tt.expectNumber, ref.Number)
				}
			})
		}
	})

	t.Run("NormalizePullRequestURL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			raw      string
			expected string
		}{
			{
				name:     "Simple, already normalized",
				raw:      "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3",
				expected: "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3",
			},
			{
				name:     "Simple with query and fragment",
				raw:      "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3?tab=diffs#note_123",
				expected: "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3",
			},
			{
				name:     "Nested group with query",
				raw:      "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1?diff_id=1234",
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1",
			},
			{
				name:     "Nested group with path suffix",
				raw:      "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/3/diffs",
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/3",
			},
			{
				name:     "Not a MR URL",
				raw:      "https://example.com/foo",
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
				got := provider.NormalizePullRequestURL(tt.raw)
				assert.Equal(t, tt.expected, got)
			})
		}
	})

	t.Run("BuildBranchURL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			owner    string
			repo     string
			branch   string
			expected string
		}{
			{
				name:     "Simple namespace",
				owner:    "test-group9945421",
				repo:     "test-project",
				branch:   "main",
				expected: "https://gitlab.com/test-group9945421/test-project/-/tree/main",
			},
			{
				name:     "Nested group",
				owner:    "test-group9945421/test-subgroup",
				repo:     "another-test-project",
				branch:   "main",
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/tree/main",
			},
			{
				name:     "Branch with special name",
				owner:    "test-group9945421/test-subgroup",
				repo:     "another-test-project",
				branch:   "johnstcn-main-patch-54711",
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/tree/johnstcn-main-patch-54711",
			},
			{
				name:     "Empty owner",
				owner:    "",
				repo:     "test-project",
				branch:   "main",
				expected: "",
			},
			{
				name:     "Empty repo",
				owner:    "test-group9945421",
				repo:     "",
				branch:   "main",
				expected: "",
			},
			{
				name:     "Empty branch",
				owner:    "test-group9945421",
				repo:     "test-project",
				branch:   "",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := provider.BuildBranchURL(tt.owner, tt.repo, tt.branch)
				assert.Equal(t, tt.expected, got)
			})
		}
	})

	t.Run("BuildRepositoryURL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			owner    string
			repo     string
			expected string
		}{
			{
				name:     "Simple namespace",
				owner:    "test-group9945421",
				repo:     "test-project",
				expected: "https://gitlab.com/test-group9945421/test-project",
			},
			{
				name:     "Nested group",
				owner:    "test-group9945421/test-subgroup",
				repo:     "another-test-project",
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project",
			},
			{
				name:     "Empty owner",
				owner:    "",
				repo:     "test-project",
				expected: "",
			},
			{
				name:     "Empty repo",
				owner:    "test-group9945421",
				repo:     "",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := provider.BuildRepositoryURL(tt.owner, tt.repo)
				assert.Equal(t, tt.expected, got)
			})
		}
	})

	t.Run("BuildPullRequestURL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			ref      gitprovider.PRRef
			expected string
		}{
			{
				name:     "Simple namespace",
				ref:      gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 3},
				expected: "https://gitlab.com/test-group9945421/test-project/-/merge_requests/3",
			},
			{
				name:     "Nested group",
				ref:      gitprovider.PRRef{Owner: "test-group9945421/test-subgroup", Repo: "another-test-project", Number: 1},
				expected: "https://gitlab.com/test-group9945421/test-subgroup/another-test-project/-/merge_requests/1",
			},
			{
				name:     "Empty owner",
				ref:      gitprovider.PRRef{Owner: "", Repo: "test-project", Number: 3},
				expected: "",
			},
			{
				name:     "Empty repo",
				ref:      gitprovider.PRRef{Owner: "test-group9945421", Repo: "", Number: 3},
				expected: "",
			},
			{
				name:     "Zero number",
				ref:      gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 0},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := provider.BuildPullRequestURL(tt.ref)
				assert.Equal(t, tt.expected, got)
			})
		}
	})

	// --- API calls (use VCR cassettes) ---

	t.Run("FetchPullRequestStatus", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                string
			ref                 gitprovider.PRRef
			expectState         gitprovider.PRState
			expectAuthor        string
			expectHead          string
			expectBase          string
			expectBranch        string
			expectTitle         string
			expectDraft         bool
			expectChanges       int32
			expectApproved      bool
			expectReviewerCount int32
			expectChangesReq    bool
		}{
			{
				name:                "open_mergeable",
				ref:                 gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 3},
				expectState:         gitprovider.PRStateOpen,
				expectAuthor:        "johnstcn",
				expectHead:          "da57fca657e02c1fbe131402f927d134a34b257b",
				expectBase:          "main",
				expectBranch:        "johnstcn-main-patch-98822",
				expectTitle:         "Open mergeable",
				expectDraft:         false,
				expectChanges:       1,
				expectApproved:      true,
				expectReviewerCount: 0,
				expectChangesReq:    false,
			},
			{
				name:                "open_with_conflicts",
				ref:                 gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 2},
				expectState:         gitprovider.PRStateOpen,
				expectAuthor:        "johnstcn",
				expectHead:          "642379758fa148ff24cba5f676226a3f8e560d73",
				expectBase:          "main",
				expectBranch:        "johnstcn-main-patch-84369",
				expectTitle:         "Open with conflicts",
				expectDraft:         false,
				expectChanges:       1,
				expectApproved:      true,
				expectReviewerCount: 0,
				expectChangesReq:    false,
			},
			{
				name:                "nested_merged",
				ref:                 gitprovider.PRRef{Owner: "test-group9945421/test-subgroup", Repo: "another-test-project", Number: 1},
				expectState:         gitprovider.PRStateMerged,
				expectAuthor:        "johnstcn",
				expectHead:          "ff919f3dc418e4fbffb6fbded7b4c9ae60a4531b",
				expectBase:          "main",
				expectBranch:        "johnstcn-main-patch-54711",
				expectTitle:         "Nested merged",
				expectDraft:         false,
				expectChanges:       1,
				expectApproved:      true,
				expectReviewerCount: 0,
				expectChangesReq:    false,
			},
			{
				name:                "nested_closed_from_fork",
				ref:                 gitprovider.PRRef{Owner: "test-group9945421/test-subgroup", Repo: "another-test-project", Number: 3},
				expectState:         gitprovider.PRStateClosed,
				expectAuthor:        "johnstcn",
				expectHead:          "6b743c6728fa248e3654657e0e576eafcf472953",
				expectBase:          "main",
				expectBranch:        "forked",
				expectTitle:         "Nested closed from fork",
				expectDraft:         false,
				expectChanges:       1,
				expectApproved:      true,
				expectReviewerCount: 0,
				expectChangesReq:    false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				rec := newGitLabVCR(t, "FetchPullRequestStatus/"+tt.name)
				vcrProvider, err := gitprovider.New("gitlab", apiURL, rec.GetDefaultClient())
				require.NoError(t, err)
				require.NotNil(t, vcrProvider)

				status, err := vcrProvider.FetchPullRequestStatus(ctx, token, tt.ref)
				require.NoError(t, err)
				require.NotNil(t, status)

				assert.Equal(t, tt.expectState, status.State)
				assert.Equal(t, tt.expectDraft, status.Draft)
				assert.Equal(t, tt.ref.Number, status.PRNumber)
				assert.False(t, status.FetchedAt.IsZero())
				assert.WithinDuration(t, time.Now(), status.FetchedAt, 10*time.Second)

				// Fields that are always populated.
				assert.NotEmpty(t, status.Title)
				assert.NotEmpty(t, status.HeadSHA)
				assert.NotEmpty(t, status.HeadBranch)
				assert.NotEmpty(t, status.BaseBranch)
				assert.NotEmpty(t, status.AuthorLogin)

				// Exact assertions for publicly-verifiable fixtures.
				if tt.expectAuthor != "" {
					assert.Equal(t, tt.expectAuthor, status.AuthorLogin)
				}
				if tt.expectHead != "" {
					assert.Equal(t, tt.expectHead, status.HeadSHA)
				}
				if tt.expectBase != "" {
					assert.Equal(t, tt.expectBase, status.BaseBranch)
				}
				if tt.expectBranch != "" {
					assert.Equal(t, tt.expectBranch, status.HeadBranch)
				}
				if tt.expectTitle != "" {
					assert.Equal(t, tt.expectTitle, status.Title)
				}
				if tt.expectChanges > 0 {
					assert.Equal(t, tt.expectChanges, status.DiffStats.ChangedFiles)
				}

				// Approval-related fields populated from GitLab approvals endpoint.
				assert.Equal(t, tt.expectApproved, status.Approved)
				assert.Equal(t, tt.expectReviewerCount, status.ReviewerCount)
				assert.Equal(t, tt.expectChangesReq, status.ChangesRequested)
			})
		}
	})

	t.Run("FetchPullRequestDiff", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			ref  gitprovider.PRRef
		}{
			{
				name: "open_mergeable",
				ref:  gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 3},
			},
			{
				name: "open_with_conflicts",
				ref:  gitprovider.PRRef{Owner: "test-group9945421", Repo: "test-project", Number: 2},
			},
			{
				name: "nested_merged",
				ref:  gitprovider.PRRef{Owner: "test-group9945421/test-subgroup", Repo: "another-test-project", Number: 1},
			},
			{
				name: "nested_closed_from_fork",
				ref:  gitprovider.PRRef{Owner: "test-group9945421/test-subgroup", Repo: "another-test-project", Number: 3},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				rec := newGitLabVCR(t, "FetchPullRequestDiff/"+tt.name)
				vcrProvider, err := gitprovider.New("gitlab", apiURL, rec.GetDefaultClient())
				require.NoError(t, err)
				require.NotNil(t, vcrProvider)

				diff, err := vcrProvider.FetchPullRequestDiff(ctx, token, tt.ref)
				require.NoError(t, err)
				assert.NotEmpty(t, diff)
				assert.Contains(t, diff, "diff --git")
			})
		}
	})

	t.Run("ResolveBranchPullRequest", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			ref       gitprovider.BranchRef
			expectNil bool // true if branch is known-deleted or from a fork
		}{
			{
				name: "open_mr_branch",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421",
					Repo:   "test-project",
					Branch: "johnstcn-main-patch-98822",
				},
				expectNil: false,
			},
			{
				name: "nested_branch_deleted_after_merge",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421/test-subgroup",
					Repo:   "another-test-project",
					Branch: "johnstcn-main-patch-54711",
				},
				expectNil: true,
			},
			{
				name: "nested_fork_branch_not_on_target",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421/test-subgroup",
					Repo:   "another-test-project",
					Branch: "forked",
				},
				expectNil: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				rec := newGitLabVCR(t, "ResolveBranchPullRequest/"+tt.name)
				vcrProvider, err := gitprovider.New("gitlab", apiURL, rec.GetDefaultClient())
				require.NoError(t, err)
				require.NotNil(t, vcrProvider)

				ref, err := vcrProvider.ResolveBranchPullRequest(ctx, token, tt.ref)
				require.NoError(t, err)
				if tt.expectNil {
					assert.Nil(t, ref)
				} else {
					require.NotNil(t, ref)
					assert.Equal(t, tt.ref.Owner, ref.Owner)
					assert.Equal(t, tt.ref.Repo, ref.Repo)
					assert.Greater(t, ref.Number, 0)
				}
			})
		}
	})

	t.Run("FetchBranchDiff", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			ref       gitprovider.BranchRef
			expectErr bool // true if branch no longer exists
		}{
			{
				name: "open_mr_branch",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421",
					Repo:   "test-project",
					Branch: "johnstcn-main-patch-98822",
				},
			},
			{
				name: "nested_branch_deleted_after_merge",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421/test-subgroup",
					Repo:   "another-test-project",
					Branch: "johnstcn-main-patch-54711",
				},
				// Branch was removed after merge.
				expectErr: true,
			},
			{
				name: "nested_fork_branch_not_on_target",
				ref: gitprovider.BranchRef{
					Owner:  "test-group9945421/test-subgroup",
					Repo:   "another-test-project",
					Branch: "forked",
				},
				// Branch only existed in the fork, not on the target repo.
				expectErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				rec := newGitLabVCR(t, "FetchBranchDiff/"+tt.name)
				vcrProvider, err := gitprovider.New("gitlab", apiURL, rec.GetDefaultClient())
				require.NoError(t, err)
				require.NotNil(t, vcrProvider)

				diff, err := vcrProvider.FetchBranchDiff(ctx, token, tt.ref)
				if tt.expectErr {
					// TODO: assert on error content (not just presence) to
					// distinguish real API errors from stale-cassette mismatches.
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				assert.NotEmpty(t, diff)
			})
		}
	})
}
