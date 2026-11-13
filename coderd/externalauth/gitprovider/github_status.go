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

// Bound reuse of pull fields not covered by the issue ETag to one hour.
const githubPullRequestMaxAge = time.Hour

// Retry rejected issue requests after one hour, unless their LRU entry is evicted.
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

// githubPullRequestSnapshot retains validators and consumed fields, without
// descriptions or embedded repositories. Its cache entry holds the issue ETag.
type githubPullRequestSnapshot struct {
	Pull     githubPullRequest `json:"pull"`
	Issue    githubIssueState  `json:"issue"`
	PullETag string            `json:"pull_etag"`
}

type githubIssueResult struct {
	state     githubIssueState
	etag      string
	unchanged bool
	rejected  bool
	retryAt   time.Time
}

func (g *githubProvider) fetchPullRequest(ctx context.Context, token string, ref PRRef, pullEndpoint string) (githubPullRequest, error) {
	owner := url.PathEscape(ref.Owner)
	repo := url.PathEscape(ref.Repo)
	repositoryEndpoint := fmt.Sprintf("%s/repos/%s/%s", g.apiBaseURL, owner, repo)
	issueEndpoint := fmt.Sprintf("%s/issues/%d", repositoryEndpoint, ref.Number)
	cacheKey := responseCacheKey(issueEndpoint, token)
	normalizedRepository := strings.ToLower(repositoryEndpoint)
	rejectionKey := "issue-rejection:" + responseCacheKey(normalizedRepository, token)

	entry, haveCached := g.cache.loadEntry(cacheKey)
	var snapshot githubPullRequestSnapshot
	if haveCached {
		if err := json.Unmarshal(entry.body, &snapshot); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode cached github pull request: %w", err)
		}
	}

	rejected := g.cache.active(rejectionKey, g.clock.Now())
	issue := githubIssueResult{rejected: rejected}
	if !rejected {
		var err error
		issue, err = g.checkIssue(ctx, token, issueEndpoint, entry.etag, snapshot.Issue)
		if err != nil {
			return githubPullRequest{}, err
		}
		if issue.unchanged && g.clock.Now().Before(entry.expiresAt) {
			return snapshot.Pull, nil
		}
	}

	return g.fetchAndStorePull(ctx, token, pullEndpoint, cacheKey, rejectionKey, snapshot, issue)
}

func (g *githubProvider) checkIssue(
	ctx context.Context,
	token string,
	endpoint string,
	etag string,
	previous githubIssueState,
) (githubIssueResult, error) {
	resp, err := g.requestJSON(ctx, endpoint, token, etag, promoauth.SourceGitProviderIssue)
	if err != nil {
		requestErr, ok := errors.AsType[*githubRequestError](err)
		if !ok || requestErr.rateLimited {
			return githubIssueResult{}, err
		}

		switch requestErr.statusCode {
		case http.StatusForbidden, http.StatusNotFound, http.StatusGone:
			return githubIssueResult{
				rejected: true,
				retryAt:  g.clock.Now().Add(githubIssueRejectionTTL),
			}, nil
		default:
			return githubIssueResult{}, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if etag == "" {
			return githubIssueResult{}, xerrors.New("github issue returned 304 without a cached issue validator")
		}
		return githubIssueResult{
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
		state: state,
		etag:  resp.Header.Get("ETag"),
	}, nil
}

func (g *githubProvider) fetchAndStorePull(
	ctx context.Context,
	token string,
	endpoint string,
	cacheKey string,
	rejectionKey string,
	snapshot githubPullRequestSnapshot,
	issue githubIssueResult,
) (githubPullRequest, error) {
	checkedAt := g.clock.Now()
	resp, err := g.requestJSON(ctx, endpoint, token, snapshot.PullETag, promoauth.SourceGitProviderPullRequest)
	if err != nil {
		return githubPullRequest{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if snapshot.PullETag == "" {
			return githubPullRequest{}, xerrors.New("github pull request returned 304 without a cached pull validator")
		}
	} else {
		var pull githubPullRequest
		if err := json.NewDecoder(resp.Body).Decode(&pull); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode github pull request: %w", err)
		}
		snapshot.Pull = pull
		snapshot.PullETag = resp.Header.Get("ETag")
	}

	snapshot.Issue = issue.state
	var expiresAt time.Time
	if !issue.rejected && pullMatchesIssue(snapshot.Pull, issue.state) {
		expiresAt = checkedAt.Add(githubPullRequestMaxAge)
	}

	projection, err := json.Marshal(snapshot)
	if err != nil {
		return githubPullRequest{}, xerrors.Errorf("encode github pull request snapshot: %w", err)
	}

	// Only a readable pull confirms that the rejection is specific to issues.
	if !issue.retryAt.IsZero() {
		g.cache.storeEntry(cachedResponse{
			key:       rejectionKey,
			expiresAt: issue.retryAt,
		})
	}
	g.cache.storeEntry(cachedResponse{
		key:       cacheKey,
		etag:      issue.etag,
		body:      projection,
		expiresAt: expiresAt,
	})
	return snapshot.Pull, nil
}

// A newer issue can be visible before the pull response catches up.
func pullMatchesIssue(pull githubPullRequest, issue githubIssueState) bool {
	return !issue.UpdatedAt.IsZero() &&
		!pull.UpdatedAt.Before(issue.UpdatedAt) &&
		pull.Title == issue.Title &&
		pull.State == issue.State &&
		pull.Draft == issue.Draft &&
		pull.Merged == (issue.PullRequest.MergedAt != nil) &&
		pull.User == issue.User
}

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
