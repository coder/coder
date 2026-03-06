package gitprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const defaultGitHubAPIBaseURL = "https://api.github.com"

type githubProvider struct {
	apiBaseURL string
	webBaseURL string
	httpClient *http.Client

	// Compiled per-instance to support GitHub Enterprise hosts.
	pullRequestPathPattern   *regexp.Regexp
	repositoryHTTPSPattern   *regexp.Regexp
	repositorySSHPathPattern *regexp.Regexp
}

func newGitHub(apiBaseURL string, httpClient *http.Client) *githubProvider {
	if apiBaseURL == "" {
		apiBaseURL = defaultGitHubAPIBaseURL
	}
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// Derive the web base URL from the API base URL.
	// github.com: api.github.com → github.com
	// GHE: ghes.corp.com/api/v3 → ghes.corp.com
	webBaseURL := deriveWebBaseURL(apiBaseURL)

	// Parse the host for regex construction.
	host := extractHost(webBaseURL)

	// Escape the host for use in regex patterns.
	escapedHost := regexp.QuoteMeta(host)

	return &githubProvider{
		apiBaseURL: apiBaseURL,
		webBaseURL: webBaseURL,
		httpClient: httpClient,
		pullRequestPathPattern: regexp.MustCompile(
			`^https://` + escapedHost + `/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([0-9]+)(?:[/?#].*)?$`,
		),
		repositoryHTTPSPattern: regexp.MustCompile(
			`^https://` + escapedHost + `/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
		),
		repositorySSHPathPattern: regexp.MustCompile(
			`^(?:ssh://)?git@` + escapedHost + `[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
		),
	}
}

// deriveWebBaseURL converts a GitHub API base URL to the
// corresponding web base URL.
//
// github.com:  https://api.github.com       → https://github.com
// GHE:         https://ghes.corp.com/api/v3 → https://ghes.corp.com
func deriveWebBaseURL(apiBaseURL string) string {
	u, err := url.Parse(apiBaseURL)
	if err != nil {
		return "https://github.com"
	}

	// Standard github.com: API host is api.github.com.
	if strings.EqualFold(u.Host, "api.github.com") {
		return "https://github.com"
	}

	// GHE: strip /api/v3 path suffix.
	u.Path = strings.TrimSuffix(u.Path, "/api/v3")
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}

// extractHost returns the host portion of a URL.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "github.com"
	}
	return u.Host
}

func (g *githubProvider) ParseRepositoryOrigin(raw string) (owner string, repo string, normalizedOrigin string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}

	matches := g.repositoryHTTPSPattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		matches = g.repositorySSHPathPattern.FindStringSubmatch(raw)
	}
	if len(matches) != 3 {
		return "", "", "", false
	}

	owner = strings.TrimSpace(matches[1])
	repo = strings.TrimSpace(matches[2])
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return "", "", "", false
	}

	return owner, repo, fmt.Sprintf("%s/%s/%s", g.webBaseURL, owner, repo), true
}

func (g *githubProvider) ParsePullRequestURL(raw string) (PRRef, bool) {
	matches := g.pullRequestPathPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 4 {
		return PRRef{}, false
	}

	number, err := strconv.Atoi(matches[3])
	if err != nil {
		return PRRef{}, false
	}

	return PRRef{
		Owner:  matches[1],
		Repo:   matches[2],
		Number: number,
	}, true
}

func (g *githubProvider) NormalizePullRequestURL(raw string) string {
	ref, ok := g.ParsePullRequestURL(strings.TrimRight(
		strings.TrimSpace(raw),
		"),.;",
	))
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d", g.webBaseURL, ref.Owner, ref.Repo, ref.Number)
}

func (g *githubProvider) BuildBranchURL(owner string, repo string, branch string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return ""
	}

	return fmt.Sprintf(
		"%s/%s/%s/tree/%s",
		g.webBaseURL,
		owner,
		repo,
		url.PathEscape(branch),
	)
}

func (g *githubProvider) BuildRepositoryURL(owner string, repo string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", g.webBaseURL, owner, repo)
}

func (g *githubProvider) BuildPullRequestURL(ref PRRef) string {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d", g.webBaseURL, ref.Owner, ref.Repo, ref.Number)
}

func (g *githubProvider) ResolveBranchPullRequest(
	ctx context.Context,
	token string,
	ref BranchRef,
) (*PRRef, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return nil, nil
	}

	query := url.Values{}
	query.Set("state", "open")
	query.Set("head", fmt.Sprintf("%s:%s", ref.Owner, ref.Branch))
	query.Set("sort", "updated")
	query.Set("direction", "desc")
	query.Set("per_page", "1")

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/pulls?%s",
		g.apiBaseURL,
		ref.Owner,
		ref.Repo,
		query.Encode(),
	)

	var pulls []struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}

	if err := g.decodeJSON(ctx, requestURL, token, &pulls); err != nil {
		return nil, err
	}
	if len(pulls) == 0 {
		return nil, nil
	}

	prRef, ok := g.ParsePullRequestURL(pulls[0].HTMLURL)
	if !ok {
		return nil, nil
	}
	return &prRef, nil
}

func (g *githubProvider) FetchPullRequestStatus(
	ctx context.Context,
	token string,
	ref PRRef,
) (*PRStatus, error) {
	pullEndpoint := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d",
		g.apiBaseURL,
		ref.Owner,
		ref.Repo,
		ref.Number,
	)

	var pull struct {
		State        string `json:"state"`
		Merged       bool   `json:"merged"`
		Draft        bool   `json:"draft"`
		Additions    int32  `json:"additions"`
		Deletions    int32  `json:"deletions"`
		ChangedFiles int32  `json:"changed_files"`
		Head         struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := g.decodeJSON(ctx, pullEndpoint, token, &pull); err != nil {
		return nil, err
	}

	var reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := g.decodeJSON(
		ctx,
		pullEndpoint+"/reviews?per_page=100",
		token,
		&reviews,
	); err != nil {
		return nil, err
	}

	state := PRState(strings.ToLower(strings.TrimSpace(pull.State)))
	if pull.Merged {
		state = PRStateMerged
	}

	return &PRStatus{
		State:   state,
		Draft:   pull.Draft,
		HeadSHA: pull.Head.SHA,
		DiffStats: DiffStats{
			Additions:    pull.Additions,
			Deletions:    pull.Deletions,
			ChangedFiles: pull.ChangedFiles,
		},
		ChangesRequested: hasOutstandingChangesRequested(reviews),
		FetchedAt:        time.Now().UTC(),
	}, nil
}

func (g *githubProvider) FetchPullRequestDiff(
	ctx context.Context,
	token string,
	ref PRRef,
) (string, error) {
	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/pulls/%d",
		g.apiBaseURL,
		ref.Owner,
		ref.Repo,
		ref.Number,
	)
	return g.fetchDiff(ctx, requestURL, token)
}

func (g *githubProvider) FetchBranchDiff(
	ctx context.Context,
	token string,
	ref BranchRef,
) (string, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return "", nil
	}

	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}

	repositoryURL := fmt.Sprintf(
		"%s/repos/%s/%s",
		g.apiBaseURL,
		ref.Owner,
		ref.Repo,
	)
	if err := g.decodeJSON(ctx, repositoryURL, token, &repository); err != nil {
		return "", err
	}
	defaultBranch := strings.TrimSpace(repository.DefaultBranch)
	if defaultBranch == "" {
		return "", xerrors.New("github repository default branch is empty")
	}

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/compare/%s...%s",
		g.apiBaseURL,
		ref.Owner,
		ref.Repo,
		url.PathEscape(defaultBranch),
		url.PathEscape(ref.Branch),
	)

	return g.fetchDiff(ctx, requestURL, token)
}

func (g *githubProvider) decodeJSON(
	ctx context.Context,
	requestURL string,
	token string,
	dest any,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return xerrors.Errorf("create github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "coder-chat-diff-status")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return xerrors.Errorf("execute github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return xerrors.Errorf(
				"github request failed with status %d",
				resp.StatusCode,
			)
		}
		return xerrors.Errorf(
			"github request failed with status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return xerrors.Errorf("decode github response: %w", err)
	}
	return nil
}

func (g *githubProvider) fetchDiff(
	ctx context.Context,
	requestURL string,
	token string,
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", xerrors.Errorf("create github diff request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.diff")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "coder-chat-diff")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", xerrors.Errorf("execute github diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return "", xerrors.Errorf("github diff request failed with status %d", resp.StatusCode)
		}
		return "", xerrors.Errorf(
			"github diff request failed with status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	diff, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", xerrors.Errorf("read github diff response: %w", err)
	}
	return string(diff), nil
}

func hasOutstandingChangesRequested(
	reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	},
) bool {
	type reviewerState struct {
		reviewID int64
		state    string
	}

	statesByReviewer := make(map[string]reviewerState)
	for _, review := range reviews {
		login := strings.ToLower(strings.TrimSpace(review.User.Login))
		if login == "" {
			continue
		}

		state := strings.ToUpper(strings.TrimSpace(review.State))
		switch state {
		case "CHANGES_REQUESTED", "APPROVED", "DISMISSED":
		default:
			continue
		}

		current, exists := statesByReviewer[login]
		if exists && current.reviewID > review.ID {
			continue
		}
		statesByReviewer[login] = reviewerState{
			reviewID: review.ID,
			state:    state,
		}
	}

	for _, state := range statesByReviewer {
		if state.state == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}
