package gitprovider

import (
	"context"
	"net/http"
	"time"
)

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

// Provider defines the interface that all Git hosting providers
// implement. Each method is designed to minimize API round-trips
// for the specific provider.
type Provider interface {
	// FetchPRStatus retrieves the complete status of a pull
	// request in the minimum number of API calls for this
	// provider.
	FetchPRStatus(ctx context.Context, token string, ref PRRef) (*PRStatus, error)

	// ResolveBranchPR finds the open PR (if any) for the given
	// branch. Returns nil, nil if no open PR exists.
	ResolveBranchPR(ctx context.Context, token string, ref BranchRef) (*PRRef, error)

	// FetchPRDiff returns the raw unified diff for a PR.
	// Separate from FetchPRStatus because diffs can be large
	// and are not always needed.
	FetchPRDiff(ctx context.Context, token string, ref PRRef) (string, error)

	// FetchBranchDiff returns the diff of a branch compared
	// against the repository's default branch.
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

	// BuildPullRequestURL constructs a URL to view a pull
	// request on the provider's web UI.
	BuildPullRequestURL(ref PRRef) string
}

// GitProvider wraps an external auth config and provides
// Git hosting operations. Use externalauth.Config.Git() to
// obtain an instance.
type GitProvider struct {
	provider Provider
}

// New creates a GitProvider for the given provider type and
// API base URL. Returns nil if the provider type is not a
// supported git provider.
func New(providerType string, apiBaseURL string, httpClient HTTPClient) *GitProvider {
	var p Provider
	switch providerType {
	case "github":
		p = newGitHub(apiBaseURL, httpClient)
	default:
		// Other providers (gitlab, bitbucket-cloud, etc.) will be
		// added here as they are implemented.
		return nil
	}
	return &GitProvider{provider: p}
}

// HTTPClient is the interface for making HTTP requests. This
// allows injecting instrumented clients or test mocks.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// FetchPRStatus delegates to the underlying provider.
func (g *GitProvider) FetchPRStatus(ctx context.Context, token string, ref PRRef) (*PRStatus, error) {
	return g.provider.FetchPRStatus(ctx, token, ref)
}

// ResolveBranchPR delegates to the underlying provider.
func (g *GitProvider) ResolveBranchPR(ctx context.Context, token string, ref BranchRef) (*PRRef, error) {
	return g.provider.ResolveBranchPR(ctx, token, ref)
}

// FetchPRDiff delegates to the underlying provider.
func (g *GitProvider) FetchPRDiff(ctx context.Context, token string, ref PRRef) (string, error) {
	return g.provider.FetchPRDiff(ctx, token, ref)
}

// FetchBranchDiff delegates to the underlying provider.
func (g *GitProvider) FetchBranchDiff(ctx context.Context, token string, ref BranchRef) (string, error) {
	return g.provider.FetchBranchDiff(ctx, token, ref)
}

// ParseRepositoryOrigin delegates to the underlying provider.
func (g *GitProvider) ParseRepositoryOrigin(raw string) (owner, repo, normalizedOrigin string, ok bool) {
	return g.provider.ParseRepositoryOrigin(raw)
}

// ParsePullRequestURL delegates to the underlying provider.
func (g *GitProvider) ParsePullRequestURL(raw string) (PRRef, bool) {
	return g.provider.ParsePullRequestURL(raw)
}

// NormalizePullRequestURL delegates to the underlying provider.
func (g *GitProvider) NormalizePullRequestURL(raw string) string {
	return g.provider.NormalizePullRequestURL(raw)
}

// BuildBranchURL delegates to the underlying provider.
func (g *GitProvider) BuildBranchURL(owner, repo, branch string) string {
	return g.provider.BuildBranchURL(owner, repo, branch)
}

// BuildPullRequestURL delegates to the underlying provider.
func (g *GitProvider) BuildPullRequestURL(ref PRRef) string {
	return g.provider.BuildPullRequestURL(ref)
}
