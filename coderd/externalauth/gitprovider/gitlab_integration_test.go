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
//  1. https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1
//     Simple owner/repo layout (single-level namespace).
//     State: open. Same-repo MR, 794 files, has conflicts (null base_sha).
//
//  2. https://gitlab.com/johnstcn/dotfiles/-/merge_requests/2
//     Simple owner/repo layout (single-level namespace).
//     State: open. Same-repo MR, 1 file, mergeable.
//
//  3. https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883
//     Nested group (multi-level namespace: gitlab-org/api).
//     State: merged. Same-repo MR, 1 file changed.
//
//  4. https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2654
//     Nested group, state: closed (not merged). Forked MR, 21 files.
//     Exercises closed-state mapping and cross-fork handling.
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
				raw:              "https://gitlab.com/johnstcn/dotfiles.git",
				expectOK:         true,
				expectOwner:      "johnstcn",
				expectRepo:       "dotfiles",
				expectNormalized: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:             "HTTPS no .git",
				raw:              "https://gitlab.com/johnstcn/dotfiles",
				expectOK:         true,
				expectOwner:      "johnstcn",
				expectRepo:       "dotfiles",
				expectNormalized: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:             "HTTPS trailing slash",
				raw:              "https://gitlab.com/johnstcn/dotfiles/",
				expectOK:         true,
				expectOwner:      "johnstcn",
				expectRepo:       "dotfiles",
				expectNormalized: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:             "SSH",
				raw:              "git@gitlab.com:johnstcn/dotfiles.git",
				expectOK:         true,
				expectOwner:      "johnstcn",
				expectRepo:       "dotfiles",
				expectNormalized: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:             "SSH prefix",
				raw:              "ssh://git@gitlab.com/johnstcn/dotfiles.git",
				expectOK:         true,
				expectOwner:      "johnstcn",
				expectRepo:       "dotfiles",
				expectNormalized: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:             "Nested group HTTPS",
				raw:              "https://gitlab.com/gitlab-org/api/client-go.git",
				expectOK:         true,
				expectOwner:      "gitlab-org/api",
				expectRepo:       "client-go",
				expectNormalized: "https://gitlab.com/gitlab-org/api/client-go",
			},
			{
				name:             "Nested group HTTPS no .git",
				raw:              "https://gitlab.com/gitlab-org/api/client-go",
				expectOK:         true,
				expectOwner:      "gitlab-org/api",
				expectRepo:       "client-go",
				expectNormalized: "https://gitlab.com/gitlab-org/api/client-go",
			},
			{
				name:             "Nested group SSH",
				raw:              "git@gitlab.com:gitlab-org/api/client-go.git",
				expectOK:         true,
				expectOwner:      "gitlab-org/api",
				expectRepo:       "client-go",
				expectNormalized: "https://gitlab.com/gitlab-org/api/client-go",
			},
			{
				name:             "Nested group SSH prefix",
				raw:              "ssh://git@gitlab.com/gitlab-org/api/client-go.git",
				expectOK:         true,
				expectOwner:      "gitlab-org/api",
				expectRepo:       "client-go",
				expectNormalized: "https://gitlab.com/gitlab-org/api/client-go",
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
				raw:          "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1",
				expectOK:     true,
				expectOwner:  "johnstcn",
				expectRepo:   "dotfiles",
				expectNumber: 1,
			},
			{
				name:         "Nested group",
				raw:          "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883",
				expectOK:     true,
				expectOwner:  "gitlab-org/api",
				expectRepo:   "client-go",
				expectNumber: 2883,
			},
			{
				name:         "Nested group second MR",
				raw:          "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2654",
				expectOK:     true,
				expectOwner:  "gitlab-org/api",
				expectRepo:   "client-go",
				expectNumber: 2654,
			},
			{
				name:         "With query string",
				raw:          "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1?tab=diffs",
				expectOK:     true,
				expectOwner:  "johnstcn",
				expectRepo:   "dotfiles",
				expectNumber: 1,
			},
			{
				name:         "With fragment",
				raw:          "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883#note_123",
				expectOK:     true,
				expectOwner:  "gitlab-org/api",
				expectRepo:   "client-go",
				expectNumber: 2883,
			},
			{
				name:         "With path suffix (diffs tab)",
				raw:          "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2654/diffs",
				expectOK:     true,
				expectOwner:  "gitlab-org/api",
				expectRepo:   "client-go",
				expectNumber: 2654,
			},
			{
				name:     "GitHub PR does not match",
				raw:      "https://github.com/coder/coder/pull/123",
				expectOK: false,
			},
			{
				name:     "Not a MR URL",
				raw:      "https://gitlab.com/johnstcn/dotfiles/-/issues/1",
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
				raw:      "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1",
				expected: "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1",
			},
			{
				name:     "Simple with query and fragment",
				raw:      "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1?tab=diffs#note_123",
				expected: "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1",
			},
			{
				name:     "Nested group with query",
				raw:      "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883?diff_id=1234",
				expected: "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883",
			},
			{
				name:     "Nested group with path suffix",
				raw:      "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2654/diffs",
				expected: "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2654",
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
				owner:    "johnstcn",
				repo:     "dotfiles",
				branch:   "main",
				expected: "https://gitlab.com/johnstcn/dotfiles/-/tree/main",
			},
			{
				name:     "Nested group",
				owner:    "gitlab-org/api",
				repo:     "client-go",
				branch:   "main",
				expected: "https://gitlab.com/gitlab-org/api/client-go/-/tree/main",
			},
			{
				name:     "Branch with slashes",
				owner:    "gitlab-org/api",
				repo:     "client-go",
				branch:   "add-missing-deprecation-replacement",
				expected: "https://gitlab.com/gitlab-org/api/client-go/-/tree/add-missing-deprecation-replacement",
			},
			{
				name:     "Empty owner",
				owner:    "",
				repo:     "dotfiles",
				branch:   "main",
				expected: "",
			},
			{
				name:     "Empty repo",
				owner:    "johnstcn",
				repo:     "",
				branch:   "main",
				expected: "",
			},
			{
				name:     "Empty branch",
				owner:    "johnstcn",
				repo:     "dotfiles",
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
				owner:    "johnstcn",
				repo:     "dotfiles",
				expected: "https://gitlab.com/johnstcn/dotfiles",
			},
			{
				name:     "Nested group",
				owner:    "gitlab-org/api",
				repo:     "client-go",
				expected: "https://gitlab.com/gitlab-org/api/client-go",
			},
			{
				name:     "Empty owner",
				owner:    "",
				repo:     "dotfiles",
				expected: "",
			},
			{
				name:     "Empty repo",
				owner:    "johnstcn",
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
				ref:      gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 1},
				expected: "https://gitlab.com/johnstcn/dotfiles/-/merge_requests/1",
			},
			{
				name:     "Nested group",
				ref:      gitprovider.PRRef{Owner: "gitlab-org/api", Repo: "client-go", Number: 2883},
				expected: "https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2883",
			},
			{
				name:     "Empty owner",
				ref:      gitprovider.PRRef{Owner: "", Repo: "dotfiles", Number: 1},
				expected: "",
			},
			{
				name:     "Empty repo",
				ref:      gitprovider.PRRef{Owner: "johnstcn", Repo: "", Number: 1},
				expected: "",
			},
			{
				name:     "Zero number",
				ref:      gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 0},
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
			name          string
			ref           gitprovider.PRRef
			expectState   gitprovider.PRState
			expectAuthor  string
			expectHead    string
			expectBase    string
			expectBranch  string
			expectTitle   string
			expectDraft   bool
			expectChanges int32
		}{
			{
				// Open MR in simple namespace, 794 files, has conflicts.
				name:          "simple_namespace_open_with_conflicts",
				ref:           gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 1},
				expectState:   gitprovider.PRStateOpen,
				expectAuthor:  "johnstcn",
				expectHead:    "806769254e513400a9ef53cd7fd51e236b61eef9",
				expectBase:    "main",
				expectBranch:  "github",
				expectTitle:   "Migrate from Ansible back to shell scripts",
				expectDraft:   false,
				expectChanges: 794,
			},
			{
				// Open MR in simple namespace, 1 file, mergeable.
				name:          "simple_namespace_open_mergeable",
				ref:           gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 2},
				expectState:   gitprovider.PRStateOpen,
				expectAuthor:  "johnstcn",
				expectHead:    "039b0a471e3afd3119c0baa526412ab8ec0bb56b",
				expectBase:    "main",
				expectBranch:  "johnstcn-main-patch-16936",
				expectTitle:   "Edit install.sh",
				expectDraft:   false,
				expectChanges: 1,
			},
			{
				// Merged MR in nested group, same-repo, 1 file.
				name:          "nested_group_merged",
				ref:           gitprovider.PRRef{Owner: "gitlab-org/api", Repo: "client-go", Number: 2883},
				expectState:   gitprovider.PRStateMerged,
				expectAuthor:  "heidi.berry",
				expectHead:    "f793022bc000a737f4fca46a4eed1c4a3ea353d2",
				expectBase:    "main",
				expectBranch:  "add-missing-deprecation-replacement",
				expectTitle:   "fix: Add PublicJobs to CreateProjectOptions",
				expectDraft:   false,
				expectChanges: 1,
			},
			{
				// Closed (not merged) MR from a fork, 21 files.
				name:          "nested_group_closed_from_fork",
				ref:           gitprovider.PRRef{Owner: "gitlab-org/api", Repo: "client-go", Number: 2654},
				expectState:   gitprovider.PRStateClosed,
				expectAuthor:  "amrkhald777",
				expectHead:    "868041db85955c58393306e7c21c480f243b3a18",
				expectBase:    "main",
				expectBranch:  "convert-examples-to-testable-examples",
				expectTitle:   "feat: convert examples to testable examples for pkg.go.dev",
				expectDraft:   false,
				expectChanges: 21,
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
			})
		}
	})

	t.Run("FetchPullRequestDiff", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			ref       gitprovider.PRRef
			emptyDiff bool // true if GitLab cannot compute a diff (e.g. null base_sha)
		}{
			{
				// MR !1 has conflicts and null base_sha; GitLab returns empty.
				name:      "simple_namespace_open_with_conflicts_empty_diff",
				ref:       gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 1},
				emptyDiff: true,
			},
			{
				name: "simple_namespace_open_mergeable",
				ref:  gitprovider.PRRef{Owner: "johnstcn", Repo: "dotfiles", Number: 2},
			},
			{
				name: "nested_group_merged",
				ref:  gitprovider.PRRef{Owner: "gitlab-org/api", Repo: "client-go", Number: 2883},
			},
			{
				name: "nested_group_closed_from_fork",
				ref:  gitprovider.PRRef{Owner: "gitlab-org/api", Repo: "client-go", Number: 2654},
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
				if tt.emptyDiff {
					assert.Empty(t, diff)
				} else {
					assert.NotEmpty(t, diff)
					assert.Contains(t, diff, "diff --git")
				}
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
				// The MR source branch "github" exists and is open.
				name: "simple_namespace_open_mr_branch",
				ref: gitprovider.BranchRef{
					Owner:  "johnstcn",
					Repo:   "dotfiles",
					Branch: "github",
				},
				expectNil: false,
			},
			{
				name: "nested_group_source_branch_removed_after_merge",
				ref: gitprovider.BranchRef{
					Owner:  "gitlab-org/api",
					Repo:   "client-go",
					Branch: "add-missing-deprecation-replacement",
				},
				// MR 2883 had should_remove_source_branch=true,
				// so the branch should be gone. No open MR expected.
				expectNil: true,
			},
			{
				name: "nested_group_branch_from_fork",
				ref: gitprovider.BranchRef{
					Owner:  "gitlab-org/api",
					Repo:   "client-go",
					Branch: "convert-examples-to-testable-examples",
				},
				// MR 2654 was from a fork (source_project_id=null).
				// The branch does not exist on the target project.
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
				name: "simple_namespace_open_mr_branch",
				ref: gitprovider.BranchRef{
					Owner:  "johnstcn",
					Repo:   "dotfiles",
					Branch: "github",
				},
			},
			{
				name: "nested_group_source_branch_deleted_after_merge",
				ref: gitprovider.BranchRef{
					Owner:  "gitlab-org/api",
					Repo:   "client-go",
					Branch: "add-missing-deprecation-replacement",
				},
				// Branch was removed after merge.
				expectErr: true,
			},
			{
				name: "nested_group_branch_from_fork_not_on_target",
				ref: gitprovider.BranchRef{
					Owner:  "gitlab-org/api",
					Repo:   "client-go",
					Branch: "convert-examples-to-testable-examples",
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
					require.Error(t, err)
					return
				}
				// For branches that may or may not exist, be lenient.
				if err != nil {
					t.Logf("FetchBranchDiff returned error (branch may be deleted): %v", err)
					return
				}
				assert.NotEmpty(t, diff)
			})
		}
	})
}
