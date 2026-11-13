package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/promoauth"
)

// githubPullRequestMaxAge is how long a snapshot that agrees with the issue is
// reused without requesting the pull. The issue ETag does not cover pull
// fields such as the head SHA, diff stats, and commit count.
const githubPullRequestMaxAge = time.Hour

// githubIssueRejectionTTL is how long a rejected issue request disables the
// issue gate for a token and repository. LRU eviction can end it earlier.
const githubIssueRejectionTTL = time.Hour

type githubPullRequest struct {
	Title        string    `json:"title"`
	State        string    `json:"state"`
	Merged       bool      `json:"merged"`
	Draft        bool      `json:"draft"`
	Additions    int32     `json:"additions"`
	Deletions    int32     `json:"deletions"`
	ChangedFiles int32     `json:"changed_files"`
	Number       int       `json:"number"`
	Commits      int32     `json:"commits"`
	UpdatedAt    time.Time `json:"updated_at"`
	Head         struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	User struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type githubIssueState struct {
	Title       string    `json:"title"`
	State       string    `json:"state"`
	Draft       bool      `json:"draft"`
	UpdatedAt   time.Time `json:"updated_at"`
	PullRequest struct {
		MergedAt *time.Time `json:"merged_at"`
	} `json:"pull_request"`
	User struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// githubPullRequestSnapshot is the JSON projection stored under the issue's
// cache key. The entry's etag is the issue ETag, and PullETag validates Pull.
// Descriptions and embedded repositories are not decoded, so they are not
// stored.
type githubPullRequestSnapshot struct {
	Pull     githubPullRequest `json:"pull"`
	Issue    githubIssueState  `json:"issue"`
	PullETag string            `json:"pull_etag"`
}

// githubPullRequestLookup holds the endpoints and cache keys for one pull
// request lookup.
type githubPullRequestLookup struct {
	token         string
	issueEndpoint string
	pullEndpoint  string
	// snapshotKey is the issue endpoint's cache key. Its entry holds the
	// githubPullRequestSnapshot.
	snapshotKey string
	// rejectionKey is the key of the repository's rejection marker. The
	// repository endpoint is lowercased because GitHub matches owner and
	// repository names case-insensitively.
	rejectionKey string
}

// githubIssueAccess says whether a lookup could use the issue endpoint.
type githubIssueAccess int

const (
	// githubIssueReadable means the issue request returned 200 or 304.
	githubIssueReadable githubIssueAccess = iota
	// githubIssueRejected means the issue request was rejected and no
	// rejection marker exists yet.
	githubIssueRejected
	// githubIssueRejectedByMarker means an active rejection marker skipped
	// the issue request.
	githubIssueRejectedByMarker
)

// githubIssueResult is the outcome of the issue step of a lookup.
type githubIssueResult struct {
	access githubIssueAccess
	// state and etag are zero unless access is githubIssueReadable.
	state githubIssueState
	etag  string
	// unchanged means the issue request returned 304.
	unchanged bool
	// rejectionExpiresAt is when a new rejection marker expires. It is set
	// only when access is githubIssueRejected.
	rejectionExpiresAt time.Time
}

// fetchPullRequest returns the pull request's fields, using the issue
// endpoint as a gate. A push to any branch of the repository changes the pull
// ETag through the embedded repositories, without changing the pull request.
// Those pushes do not change the issue ETag. While the issue returns 304 and
// the snapshot has not expired, the cached pull is reused. When the token
// cannot read the issue, the pull is requested every time.
func (g *githubProvider) fetchPullRequest(ctx context.Context, token string, ref PRRef, pullEndpoint string) (githubPullRequest, error) {
	lookup := g.pullRequestLookup(token, ref, pullEndpoint)

	entry, haveCached := g.cache.loadEntry(lookup.snapshotKey)
	var snapshot githubPullRequestSnapshot
	if haveCached {
		if err := json.Unmarshal(entry.body, &snapshot); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode cached github pull request: %w", err)
		}
	}

	if g.cache.active(lookup.rejectionKey, g.clock.Now()) {
		issue := githubIssueResult{access: githubIssueRejectedByMarker}
		return g.fetchAndStorePull(ctx, lookup, snapshot, issue)
	}

	issue, err := g.checkIssue(ctx, lookup, entry.etag, snapshot.Issue)
	if err != nil {
		return githubPullRequest{}, err
	}
	if issue.unchanged && g.clock.Now().Before(entry.expiresAt) {
		return snapshot.Pull, nil
	}

	return g.fetchAndStorePull(ctx, lookup, snapshot, issue)
}

func (g *githubProvider) pullRequestLookup(token string, ref PRRef, pullEndpoint string) githubPullRequestLookup {
	owner := url.PathEscape(ref.Owner)
	repo := url.PathEscape(ref.Repo)
	repositoryEndpoint := fmt.Sprintf("%s/repos/%s/%s", g.apiBaseURL, owner, repo)
	issueEndpoint := fmt.Sprintf("%s/issues/%d", repositoryEndpoint, ref.Number)
	normalizedRepository := strings.ToLower(repositoryEndpoint)

	return githubPullRequestLookup{
		token:         token,
		issueEndpoint: issueEndpoint,
		pullEndpoint:  pullEndpoint,
		snapshotKey:   responseCacheKey(issueEndpoint, token),
		rejectionKey:  "issue-rejection:" + responseCacheKey(normalizedRepository, token),
	}
}

// checkIssue requests the issue, conditional on etag. On 304 the result
// carries previous as its state.
func (g *githubProvider) checkIssue(
	ctx context.Context,
	lookup githubPullRequestLookup,
	etag string,
	previous githubIssueState,
) (githubIssueResult, error) {
	resp, err := g.requestJSON(ctx, githubJSONRequest{
		requestURL:    lookup.issueEndpoint,
		token:         lookup.token,
		etag:          etag,
		source:        promoauth.SourceGitProviderIssue,
		isRateLimited: issueRateLimited,
	})
	if err != nil {
		if isIssueRejection(err) {
			return githubIssueResult{
				access:             githubIssueRejected,
				rejectionExpiresAt: g.clock.Now().Add(githubIssueRejectionTTL),
			}, nil
		}
		return githubIssueResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if etag == "" {
			return githubIssueResult{}, xerrors.New("github issue returned 304 without a cached issue validator")
		}
		return githubIssueResult{
			access:    githubIssueReadable,
			state:     previous,
			etag:      etag,
			unchanged: true,
		}, nil
	}

	var state githubIssueState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return githubIssueResult{}, xerrors.Errorf("decode github issue: %w", err)
	}
	return githubIssueResult{
		access: githubIssueReadable,
		state:  state,
		etag:   resp.Header.Get("ETag"),
	}, nil
}

// isIssueRejection reports whether err is a 403, 404, or 410 response from
// the issue endpoint that issueRateLimited did not match.
func isIssueRejection(err error) bool {
	requestErr, ok := errors.AsType[*githubRequestError](err)
	if !ok || requestErr.rateLimited {
		return false
	}

	switch requestErr.statusCode {
	case http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return true
	default:
		return false
	}
}

// fetchAndStorePull requests the pull and stores the updated snapshot, and
// the rejection marker when the issue request was rejected.
func (g *githubProvider) fetchAndStorePull(
	ctx context.Context,
	lookup githubPullRequestLookup,
	snapshot githubPullRequestSnapshot,
	issue githubIssueResult,
) (githubPullRequest, error) {
	checkedAt := g.clock.Now()
	pull, pullETag, err := g.requestPull(ctx, lookup, snapshot)
	if err != nil {
		return githubPullRequest{}, err
	}

	snapshot.Pull = pull
	snapshot.PullETag = pullETag
	snapshot.Issue = issue.state

	// A snapshot that disagrees with the issue keeps a zero expiry, so the
	// next lookup requests the pull even when the issue returns 304.
	var expiresAt time.Time
	if issue.access == githubIssueReadable && pullMatchesIssue(pull, issue.state) {
		expiresAt = checkedAt.Add(githubPullRequestMaxAge)
	}

	projection, err := json.Marshal(snapshot)
	if err != nil {
		return githubPullRequest{}, xerrors.Errorf("encode github pull request snapshot: %w", err)
	}

	// Only a readable pull confirms that the rejection is specific to issues.
	if issue.access == githubIssueRejected {
		g.cache.storeEntry(cachedResponse{
			key:       lookup.rejectionKey,
			expiresAt: issue.rejectionExpiresAt,
		})
	}
	g.cache.storeEntry(cachedResponse{
		key:       lookup.snapshotKey,
		etag:      issue.etag,
		body:      projection,
		expiresAt: expiresAt,
	})
	return pull, nil
}

// requestPull requests the pull, conditional on cached.PullETag, and returns
// it with its ETag. On 304 it returns cached.Pull.
func (g *githubProvider) requestPull(
	ctx context.Context,
	lookup githubPullRequestLookup,
	cached githubPullRequestSnapshot,
) (githubPullRequest, string, error) {
	resp, err := g.requestJSON(ctx, githubJSONRequest{
		requestURL: lookup.pullEndpoint,
		token:      lookup.token,
		etag:       cached.PullETag,
		source:     promoauth.SourceGitProviderPullRequest,
	})
	if err != nil {
		return githubPullRequest{}, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if cached.PullETag == "" {
			return githubPullRequest{}, "", xerrors.New("github pull request returned 304 without a cached pull validator")
		}
		return cached.Pull, cached.PullETag, nil
	}

	var pull githubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pull); err != nil {
		return githubPullRequest{}, "", xerrors.Errorf("decode github pull request: %w", err)
	}
	return pull, resp.Header.Get("ETag"), nil
}

// pullMatchesIssue reports whether the pull response reflects the issue
// response, so the issue ETag can stand in for the pull until
// githubPullRequestMaxAge. They disagree when a newer issue is visible before
// the pull response catches up.
func pullMatchesIssue(pull githubPullRequest, issue githubIssueState) bool {
	return !issue.UpdatedAt.IsZero() &&
		!pull.UpdatedAt.Before(issue.UpdatedAt) &&
		pull.Title == issue.Title &&
		pull.State == issue.State &&
		pull.Draft == issue.Draft &&
		pull.Merged == (issue.PullRequest.MergedAt != nil) &&
		pull.User == issue.User
}

// issueRateLimited reports whether a failed issue response is a rate limit.
// Every 429 is. A 403 is only with Retry-After or X-RateLimit-Remaining: 0,
// because permission 403s also carry X-RateLimit-Reset and a positive
// remaining count.
func issueRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	retryAfter := resp.Header.Get("Retry-After")
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	return retryAfter != "" || remaining == "0"
}
