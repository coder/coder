package gitprovider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// providerOptions holds optional configuration for provider
// construction.
type providerOptions struct {
	clock quartz.Clock
}

// Option configures optional behavior for a Provider.
type Option func(*providerOptions)

// WithClock sets the clock used by the provider. Defaults to
// quartz.NewReal() if not provided.
func WithClock(c quartz.Clock) Option {
	return func(o *providerOptions) {
		o.clock = c
	}
}

// PRState is the normalized state of a pull/merge request across
// all providers.
type PRState string

const (
	PRStateOpen   PRState = "open"
	PRStateClosed PRState = "closed"
	PRStateMerged PRState = "merged"
)

// PRRef identifies a pull request on any provider.
type PRRef struct {
	// Owner is the repository owner / project / workspace.
	Owner string
	// Repo is the repository name or slug.
	Repo string
	// Number is the PR number / IID / index.
	Number int
}

// BranchRef identifies a branch in a repository, used for
// branch-to-PR resolution.
type BranchRef struct {
	Owner  string
	Repo   string
	Branch string
}

// DiffStats summarizes the size of a PR's changes.
type DiffStats struct {
	Additions    int32
	Deletions    int32
	ChangedFiles int32
}

// PRStatus is the complete status of a pull/merge request.
// This is the universal return type that all providers populate.
type PRStatus struct {
	// State is the PR's lifecycle state.
	State PRState
	// Draft indicates the PR is marked as draft/WIP.
	Draft bool
	// HeadSHA is the SHA of the head commit.
	HeadSHA string
	// DiffStats summarizes additions/deletions/files changed.
	DiffStats DiffStats
	// ChangesRequested is a convenience boolean: true if any
	// reviewer's current state is "changes_requested".
	ChangesRequested bool
	// FetchedAt is when this status was fetched.
	FetchedAt time.Time
}

// MaxDiffSize is the maximum number of bytes read from a diff
// response. Diffs exceeding this limit are rejected with
// ErrDiffTooLarge.
const MaxDiffSize = 4 << 20 // 4 MiB

// ErrDiffTooLarge is returned when a diff exceeds MaxDiffSize.
var ErrDiffTooLarge = xerrors.Errorf("diff exceeds maximum size of %d bytes", MaxDiffSize)

// Provider defines the interface that all Git hosting providers
// implement. Each method is designed to minimize API round-trips
// for the specific provider.
type Provider interface {
	// FetchPullRequestStatus retrieves the complete status of a
	// pull request in the minimum number of API calls for this
	// provider.
	FetchPullRequestStatus(ctx context.Context, token string, ref PRRef) (*PRStatus, error)

	// ResolveBranchPullRequest finds the open PR (if any) for
	// the given branch. Returns nil, nil if no open PR exists.
	ResolveBranchPullRequest(ctx context.Context, token string, ref BranchRef) (*PRRef, error)

	// FetchPullRequestDiff returns the raw unified diff for a
	// pull request. This uses the PR's actual base branch (which
	// may differ from the repo default branch, e.g. a PR
	// targeting "staging" instead of "main"), so it matches what
	// the provider shows on the PR's "Files changed" tab.
	// Returns ErrDiffTooLarge if the diff exceeds MaxDiffSize.
	FetchPullRequestDiff(ctx context.Context, token string, ref PRRef) (string, error)

	// FetchBranchDiff returns the diff of a branch compared
	// against the repository's default branch. This is the
	// fallback when no pull request exists yet (e.g. the agent
	// pushed a branch but hasn't opened a PR). Returns
	// ErrDiffTooLarge if the diff exceeds MaxDiffSize.
	FetchBranchDiff(ctx context.Context, token string, ref BranchRef) (string, error)

	// ParseRepositoryOrigin parses a remote origin URL (HTTPS
	// or SSH) into owner and repo components, returning the
	// normalized HTTPS URL. Returns false if the URL does not
	// match this provider.
	ParseRepositoryOrigin(raw string) (owner, repo, normalizedOrigin string, ok bool)

	// ParsePullRequestURL parses a pull request URL into a
	// PRRef. Returns false if the URL does not match this
	// provider.
	ParsePullRequestURL(raw string) (PRRef, bool)

	// NormalizePullRequestURL normalizes a pull request URL,
	// stripping trailing punctuation, query strings, and
	// fragments. Returns empty string if the URL does not
	// match this provider.
	NormalizePullRequestURL(raw string) string

	// BuildBranchURL constructs a URL to view a branch on
	// the provider's web UI.
	BuildBranchURL(owner, repo, branch string) string

	// BuildRepositoryURL constructs a URL to view a repository
	// on the provider's web UI.
	BuildRepositoryURL(owner, repo string) string

	// BuildPullRequestURL constructs a URL to view a pull
	// request on the provider's web UI.
	BuildPullRequestURL(ref PRRef) string
}

// New creates a Provider for the given provider type and API base
// URL. Returns nil if the provider type is not a supported git
// provider.
func New(providerType string, apiBaseURL string, httpClient *http.Client, opts ...Option) Provider {
	o := providerOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.clock == nil {
		o.clock = quartz.NewReal()
	}

	switch providerType {
	case "github":
		return newGitHub(apiBaseURL, httpClient, o.clock)
	default:
		// Other providers (gitlab, bitbucket-cloud, etc.) will be
		// added here as they are implemented.
		return nil
	}
}

// RateLimitError indicates the git provider's API rate limit was hit.
type RateLimitError struct {
	RetryAfter time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited until %s", e.RetryAfter.Format(time.RFC3339))
}
