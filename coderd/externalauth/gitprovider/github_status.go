package gitprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/promoauth"
)

// Bound reuse of pull fields not covered by the issue ETag to one hour.
// A base force push may change diff stats without an observed issue change.
const githubPullRequestMaxAge = time.Hour

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

// githubPullRequestSnapshot shares one response-cache entry with the issue's
// ETag. Neither the PR description nor the embedded repositories are retained.
type githubPullRequestSnapshot struct {
	Pull      githubPullRequest `json:"pull"`
	Issue     githubIssueState  `json:"issue"`
	PullETag  string            `json:"pull_etag"`
	CheckedAt time.Time         `json:"checked_at"`
}

func (g *githubProvider) fetchPullRequest(ctx context.Context, token string, ref PRRef, pullEndpoint string) (githubPullRequest, error) {
	issueEndpoint := fmt.Sprintf("%s/repos/%s/%s/issues/%d", g.apiBaseURL, url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), ref.Number)
	cacheKey := responseCacheKey(issueEndpoint, token)
	issueETag, body, haveCached := g.cache.load(cacheKey)
	var snapshot githubPullRequestSnapshot
	if haveCached {
		if err := json.Unmarshal(body, &snapshot); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode cached github pull request: %w", err)
		}
	}

	issueResponse, err := g.requestJSON(ctx, issueEndpoint, token, issueETag, promoauth.SourceGitProviderIssue)
	if err != nil {
		return githubPullRequest{}, err
	}
	defer issueResponse.Body.Close()

	if issueResponse.StatusCode == http.StatusNotModified {
		if !haveCached {
			return githubPullRequest{}, xerrors.New("github issue returned 304 without a cached pull request")
		}
		if g.clock.Now().Sub(snapshot.CheckedAt) < githubPullRequestMaxAge {
			return snapshot.Pull, nil
		}
	} else {
		var issue githubIssueState
		if err := json.NewDecoder(issueResponse.Body).Decode(&issue); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode github issue: %w", err)
		}
		snapshot.Issue = issue
		issueETag = issueResponse.Header.Get("ETag")
	}

	checkedAt := g.clock.Now()
	pullResponse, err := g.requestJSON(ctx, pullEndpoint, token, snapshot.PullETag, promoauth.SourceGitProviderPullRequest)
	if err != nil {
		return githubPullRequest{}, err
	}
	defer pullResponse.Body.Close()

	if pullResponse.StatusCode == http.StatusNotModified {
		if !haveCached {
			return githubPullRequest{}, xerrors.New("github pull request returned 304 without a cached response")
		}
	} else {
		var pull githubPullRequest
		if err := json.NewDecoder(pullResponse.Body).Decode(&pull); err != nil {
			return githubPullRequest{}, xerrors.Errorf("decode github pull request: %w", err)
		}
		snapshot.Pull = pull
		snapshot.PullETag = pullResponse.Header.Get("ETag")
	}

	// The endpoints can briefly disagree after a PR change. Only associate
	// the issue ETag with a pull response that has caught up to that issue;
	// otherwise the next diff-status refresh must fetch the pull again.
	pull := snapshot.Pull
	issue := snapshot.Issue
	matchesIssue := !issue.UpdatedAt.IsZero() && !pull.UpdatedAt.Before(issue.UpdatedAt) &&
		pull.Title == issue.Title && pull.State == issue.State && pull.Draft == issue.Draft &&
		pull.Merged == (issue.PullRequest.MergedAt != nil) && pull.User == issue.User
	if matchesIssue {
		snapshot.CheckedAt = checkedAt
		projection, err := json.Marshal(snapshot)
		if err != nil {
			return githubPullRequest{}, xerrors.Errorf("encode github pull request snapshot: %w", err)
		}
		g.cache.store(cacheKey, issueETag, projection)
	}
	return pull, nil
}
