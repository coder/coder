package gitprovider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	// Title is the PR's title/subject line.
	Title string
	// State is the PR's lifecycle state.
	State PRState
	// Draft indicates the PR is marked as draft/WIP.
	Draft bool
	// HeadSHA is the SHA of the head commit.
	HeadSHA string
	// HeadBranch is the name of the branch containing the PR changes.
	HeadBranch string
	// DiffStats summarizes additions/deletions/files changed.
	DiffStats DiffStats
	// ChangesRequested is a convenience boolean: true if any
	// reviewer's current state is "changes_requested".
	ChangesRequested bool
	// AuthorLogin is the login/username of the PR author.
	AuthorLogin string
	// AuthorAvatarURL is the avatar URL of the PR author.
	AuthorAvatarURL string
	// BaseBranch is the target branch the PR will merge into.
	BaseBranch string
	// PRNumber is the PR number (e.g. 1347).
	PRNumber int
	// Commits is the number of commits in the PR.
	Commits int32
	// Approved is true when at least one reviewer has approved
	// and no reviewer has outstanding changes requested.
	Approved bool
	// ReviewerCount is the number of distinct reviewers who
	// have left a decisive review (approved, changes_requested,
	// or dismissed).
	ReviewerCount int32
	// FetchedAt is when this status was fetched.
	FetchedAt time.Time
}

// trailingPunctuation is the set of characters stripped from the right
// of a raw URL before parsing it as a pull request URL.
const trailingPunctuation = "),;."

// MaxDiffSize is the maximum number of bytes read from a diff
// response. Diffs exceeding this limit are rejected with
// ErrDiffTooLarge.
const MaxDiffSize = 4 << 20 // 4 MiB

// RateLimitPadding is added to rate-limit retry times to guard
// against over-consumption of request quotas.
const RateLimitPadding = 5 * time.Minute

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
// URL. Returns (nil, nil) for unsupported provider types and a
// non-nil error if construction fails.
func New(providerType string, apiBaseURL string, httpClient *http.Client, opts ...Option) (Provider, error) {
	o := providerOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.clock == nil {
		o.clock = quartz.NewReal()
	}

	switch providerType {
	case "github":
		return newGitHub(apiBaseURL, httpClient, o.clock), nil
	case "gitlab":
		return newGitLab(apiBaseURL, httpClient, o.clock)
	default:
		// Other providers (bitbucket-cloud, etc.) will be
		// added here as they are implemented.
		return nil, nil //nolint:nilnil // nil provider means unsupported type, not an error
	}
}

// parseRetryAfter extracts a retry duration from rate-limit response
// headers. It checks Retry-After (seconds) first, then the named
// resetHeader (unix timestamp). Returns zero if no recognizable header
// is present.
func parseRetryAfter(h http.Header, resetHeader string, clk quartz.Clock) time.Duration {
	if clk == nil {
		clk = quartz.NewReal()
	}
	// Retry-After header: seconds until retry.
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	// Reset header: unix timestamp. We compute the duration from now
	// according to the caller's clock.
	if reset := h.Get(resetHeader); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			return time.Unix(ts, 0).Sub(clk.Now())
		}
	}
	return 0
}

// checkRateLimitError returns a *RateLimitError when resp indicates a
// rate limit (HTTP 403 or 429) with recognizable retry headers;
// otherwise nil. A nil resp returns nil.
func checkRateLimitError(resp *http.Response, clk quartz.Clock, resetHeader string) error {
	if resp == nil {
		return nil
	}
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if clk == nil {
		clk = quartz.NewReal()
	}
	retryAfter := parseRetryAfter(resp.Header, resetHeader, clk)
	if retryAfter <= 0 {
		return nil
	}
	return &RateLimitError{RetryAfter: clk.Now().Add(retryAfter + RateLimitPadding)}
}

// countDiffLines counts added and deleted lines in a unified diff. It excludes
// file header lines such as +++ b/file and --- a/file.
func countDiffLines(diff string) (additions, deletions int32) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return additions, deletions
}

// RateLimitError indicates the git provider's API rate limit was hit.
type RateLimitError struct {
	RetryAfter time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited until %s", e.RetryAfter.Format(time.RFC3339))
}
