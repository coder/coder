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

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/quartz"
)

const (
	defaultGitHubAPIBaseURL = "https://api.github.com"
)

type githubProvider struct {
	apiBaseURL     string
	webBaseURL     string
	httpClient     *http.Client
	clock          quartz.Clock
	requestMetrics promoauth.InstrumentedOAuth2Config

	// cache retains validators, projections, and expiry metadata.
	cache *responseCache

	// Compiled per-instance to support GitHub Enterprise hosts.
	pullRequestPathPattern   *regexp.Regexp
	repositoryHTTPSPattern   *regexp.Regexp
	repositorySSHPathPattern *regexp.Regexp
}

func newGitHub(apiBaseURL string, httpClient *http.Client, clock quartz.Clock, requestMetrics promoauth.InstrumentedOAuth2Config) (Provider, error) {
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
		apiBaseURL:     apiBaseURL,
		webBaseURL:     webBaseURL,
		httpClient:     httpClient,
		clock:          clock,
		requestMetrics: requestMetrics,
		cache:          newResponseCache(defaultResponseCacheEntries),
		pullRequestPathPattern: regexp.MustCompile(
			`^https://` + escapedHost + `/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([0-9]+)(?:[/?#].*)?$`,
		),
		repositoryHTTPSPattern: regexp.MustCompile(
			`^https://` + escapedHost + `/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
		),
		repositorySSHPathPattern: regexp.MustCompile(
			`^(?:ssh://)?git@` + escapedHost + `[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`,
		),
	}, nil
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

	return owner, repo, fmt.Sprintf("%s/%s/%s", g.webBaseURL, url.PathEscape(owner), url.PathEscape(repo)), true
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
		trailingPunctuation,
	))
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d", g.webBaseURL, url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), ref.Number)
}

// escapePathPreserveSlashes escapes each segment of a path
// individually, preserving `/` separators. This is needed for
// web URLs where GitHub expects literal slashes (e.g.
// /tree/feat/new-thing).
func escapePathPreserveSlashes(s string) string {
	segments := strings.Split(s, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
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
		url.PathEscape(owner),
		url.PathEscape(repo),
		escapePathPreserveSlashes(branch),
	)
}

func (g *githubProvider) BuildRepositoryURL(owner string, repo string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", g.webBaseURL, url.PathEscape(owner), url.PathEscape(repo))
}

func (g *githubProvider) BuildPullRequestURL(ref PRRef) string {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pull/%d", g.webBaseURL, url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), ref.Number)
}

func (g *githubProvider) ResolveBranchPullRequest(
	ctx context.Context,
	token string,
	ref BranchRef,
) (*PRRef, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return nil, nil //nolint:nilnil // Provider contract: nil, nil means no open PR.
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
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
		query.Encode(),
	)

	var pulls []struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}

	if err := g.decodeJSON(ctx, requestURL, token, &pulls, promoauth.SourceGitProviderListPullRequests); err != nil {
		return nil, err
	}
	if len(pulls) == 0 {
		return nil, nil //nolint:nilnil // Provider contract: nil, nil means no open PR.
	}

	prRef, ok := g.ParsePullRequestURL(pulls[0].HTMLURL)
	if !ok {
		return nil, nil //nolint:nilnil // Provider contract: nil, nil means no open PR.
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
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
		ref.Number,
	)

	pull, err := g.fetchPullRequest(ctx, token, ref, pullEndpoint)
	if err != nil {
		return nil, err
	}

	var reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	// GitHub returns at most 100 reviews per page. We do not
	// paginate because PRs with >100 reviews are extremely rare,
	// and the cost of multiple API calls per refresh is not
	// justified. If needed, pagination can be added later.
	if err := g.decodeJSON(
		ctx,
		pullEndpoint+"/reviews?per_page=100",
		token,
		&reviews,
		promoauth.SourceGitProviderReviews,
	); err != nil {
		return nil, err
	}

	state := PRState(strings.ToLower(strings.TrimSpace(pull.State)))
	if pull.Merged {
		state = PRStateMerged
	}

	reviewInfo := summarizeReviews(reviews)

	return &PRStatus{
		Title:      pull.Title,
		State:      state,
		Draft:      pull.Draft,
		HeadSHA:    pull.Head.SHA,
		HeadBranch: pull.Head.Ref,
		DiffStats: DiffStats{
			Additions:    pull.Additions,
			Deletions:    pull.Deletions,
			ChangedFiles: pull.ChangedFiles,
		},
		ChangesRequested: reviewInfo.changesRequested,
		Approved:         reviewInfo.approved,
		ReviewerCount:    reviewInfo.reviewerCount,
		AuthorLogin:      pull.User.Login,
		AuthorAvatarURL:  pull.User.AvatarURL,
		BaseBranch:       pull.Base.Ref,
		PRNumber:         pull.Number,
		Commits:          pull.Commits,
		FetchedAt:        g.clock.Now().UTC(),
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
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
		ref.Number,
	)
	return g.fetchDiff(ctx, requestURL, token, promoauth.SourceGitProviderPullRequestDiff)
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
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
	)
	if err := g.decodeJSON(ctx, repositoryURL, token, &repository, promoauth.SourceGitProviderRepository); err != nil {
		return "", err
	}
	defaultBranch := strings.TrimSpace(repository.DefaultBranch)
	if defaultBranch == "" {
		return "", xerrors.New("github repository default branch is empty")
	}

	requestURL := fmt.Sprintf(
		"%s/repos/%s/%s/compare/%s...%s",
		g.apiBaseURL,
		url.PathEscape(ref.Owner),
		url.PathEscape(ref.Repo),
		url.PathEscape(defaultBranch),
		url.PathEscape(ref.Branch),
	)

	return g.fetchDiff(ctx, requestURL, token, promoauth.SourceGitProviderBranchDiff)
}

func (g *githubProvider) decodeJSON(
	ctx context.Context,
	requestURL string,
	token string,
	dest any,
	source promoauth.Oauth2Source,
) error {
	cacheKey := responseCacheKey(requestURL, token)
	etag, cachedBody, haveCached := g.cache.load(cacheKey)
	resp, err := g.requestJSON(ctx, githubJSONRequest{
		requestURL: requestURL,
		token:      token,
		etag:       etag,
		source:     source,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Nothing changed since the cached response: reuse the stored body.
	if resp.StatusCode == http.StatusNotModified && haveCached {
		if err := json.Unmarshal(cachedBody, dest); err != nil {
			return xerrors.Errorf("decode cached github response: %w", err)
		}
		return nil
	}

	if resp.StatusCode == http.StatusNotModified {
		return xerrors.New("github response returned 304 without a cached response")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return xerrors.Errorf("read github response: %w", err)
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return xerrors.Errorf("decode github response: %w", err)
	}

	// Only cache bodies we could successfully decode, so a malformed
	// response does not poison the cache.
	g.cache.store(cacheKey, resp.Header.Get("ETag"), body)
	return nil
}

// githubJSONRequest is a GET request for a JSON resource of the GitHub REST
// API.
type githubJSONRequest struct {
	requestURL string
	token      string
	// etag, when set, makes the request conditional.
	etag   string
	source promoauth.Oauth2Source
	// isRateLimited selects the failed responses that are checked for a
	// retry time. When nil, every failed response is checked.
	isRateLimited func(*http.Response) bool
}

// requestJSON returns the response for a 200 or a 304; the caller closes its
// body. For any other response it returns a *RateLimitError when the response
// is checked and has a usable retry time. Otherwise it returns a
// *githubRequestError, or a plain error when the body cannot be read.
func (g *githubProvider) requestJSON(ctx context.Context, request githubJSONRequest) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.requestURL, nil)
	if err != nil {
		return nil, xerrors.Errorf("create github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "coder-chat-diff-status")
	if request.token != "" {
		req.Header.Set("Authorization", "Bearer "+request.token)
	}
	if request.etag != "" {
		req.Header.Set("If-None-Match", request.etag)
	}

	resp, err := g.do(ctx, req, request.source)
	if err != nil {
		return nil, xerrors.Errorf("execute github request: %w", err)
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	defer resp.Body.Close()

	rateLimited := request.isRateLimited != nil && request.isRateLimited(resp)
	checkRetryTime := request.isRateLimited == nil || rateLimited
	if checkRetryTime {
		if rlErr := checkRateLimitError(resp, g.clock, "X-Ratelimit-Reset"); rlErr != nil {
			return nil, rlErr
		}
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if readErr != nil {
		return nil, xerrors.Errorf("github request failed with status %d", resp.StatusCode)
	}
	message := fmt.Sprintf(
		"github request failed with status %d: %s",
		resp.StatusCode,
		strings.TrimSpace(string(body)),
	)
	return nil, &githubRequestError{
		statusCode:  resp.StatusCode,
		message:     message,
		rateLimited: rateLimited,
	}
}

// githubRequestError is a failed GitHub API response without a usable retry
// time.
type githubRequestError struct {
	statusCode int
	message    string
	// rateLimited is true when the request's isRateLimited predicate matched
	// the response. It is false for requests without a predicate.
	rateLimited bool
}

func (e *githubRequestError) Error() string {
	return e.message
}

func (g *githubProvider) do(ctx context.Context, req *http.Request, source promoauth.Oauth2Source) (*http.Response, error) {
	if g.requestMetrics == nil {
		return g.httpClient.Do(req)
	}
	// promoauth.Config.Do reads oauth2.HTTPClient from ctx to preserve this client.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, g.httpClient)
	return g.requestMetrics.Do(ctx, source, req)
}

func (g *githubProvider) fetchDiff(
	ctx context.Context,
	requestURL string,
	token string,
	source promoauth.Oauth2Source,
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

	resp, err := g.do(ctx, req, source)
	if err != nil {
		return "", xerrors.Errorf("execute github diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if rlErr := checkRateLimitError(resp, g.clock, "X-Ratelimit-Reset"); rlErr != nil {
			return "", rlErr
		}
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

	// Read one extra byte beyond MaxDiffSize so we can detect
	// whether the diff exceeds the limit. LimitReader stops us
	// allocating an arbitrarily large buffer by accident.
	buf, err := io.ReadAll(io.LimitReader(resp.Body, MaxDiffSize+1))
	if err != nil {
		return "", xerrors.Errorf("read github diff response: %w", err)
	}
	if len(buf) > MaxDiffSize {
		return "", ErrDiffTooLarge
	}
	return string(buf), nil
}

// reviewStats holds aggregated review statistics for a PR.
type reviewStats struct {
	changesRequested bool
	approved         bool
	reviewerCount    int32
}

// summarizeReviews extracts review statistics from a list of
// reviews. For each reviewer, only the latest decisive review
// (by ID) is considered. "Decisive" means APPROVED,
// CHANGES_REQUESTED, or DISMISSED.
func summarizeReviews(
	reviews []struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	},
) reviewStats {
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

	var result reviewStats
	result.reviewerCount = clampToInt32(int64(len(statesByReviewer)))

	hasApproval := false
	for _, state := range statesByReviewer {
		if state.state == "CHANGES_REQUESTED" {
			result.changesRequested = true
		}
		if state.state == "APPROVED" {
			hasApproval = true
		}
	}
	// Approved is true only when at least one reviewer approved
	// and no reviewer has outstanding changes requested.
	result.approved = hasApproval && !result.changesRequested

	return result
}
