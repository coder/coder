package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

const (
	defaultGiteaAPIBaseURL = "https://gitea.com/api/v1"

	// giteaPageSize is the page size for list requests. Gitea and
	// Forgejo cap list responses at the server's MAX_RESPONSE_ITEMS
	// setting, which defaults to 50.
	giteaPageSize = 50

	// giteaMaxBranchPages bounds how many pages of open pull
	// requests ResolveBranchPullRequest scans on servers that do
	// not filter by head branch.
	giteaMaxBranchPages = 10
)

// giteaProvider implements Provider for Gitea and Forgejo. Forgejo is a
// fork of Gitea and serves the same /api/v1 REST API for every call
// used here, so one implementation covers both. Where the two servers
// differ, the provider relies on behavior that works on both instead
// of detecting the server type or version.
type giteaProvider struct {
	apiBaseURL string
	// webBaseURL includes any subpath the instance is served under,
	// e.g. https://example.com/gitea.
	webBaseURL string
	httpClient *http.Client
	clock      quartz.Clock
}

func newGitea(apiBaseURL string, httpClient *http.Client, clock quartz.Clock) (Provider, error) {
	if apiBaseURL == "" {
		apiBaseURL = defaultGiteaAPIBaseURL
	}
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	u, err := url.Parse(apiBaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, xerrors.Errorf("invalid gitea api base url %q", apiBaseURL)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	web := *u
	web.Path = strings.TrimSuffix(strings.TrimSuffix(web.Path, "/api/v1"), "/")
	web.RawQuery = ""
	web.Fragment = ""

	return &giteaProvider{
		apiBaseURL: apiBaseURL,
		webBaseURL: web.String(),
		httpClient: httpClient,
		clock:      clock,
	}, nil
}

var _ Provider = (*giteaProvider)(nil)

// webURL returns the parsed web base URL. It never fails because the
// constructor validated it.
func (g *giteaProvider) webURL() *url.URL {
	u, _ := url.Parse(g.webBaseURL)
	return u
}

// repoPath extracts "owner/repo" from a web URL path, stripping the
// instance subpath. Gitea and Forgejo have no nested groups, so the
// result must be exactly two segments, optionally followed by rest.
func (g *giteaProvider) repoPath(path string) (owner, repo, rest string, ok bool) {
	base := strings.TrimSuffix(g.webURL().Path, "/")
	path, ok = strings.CutPrefix(path, base+"/")
	if !ok {
		return "", "", "", false
	}
	owner, after, found := strings.Cut(path, "/")
	if !found {
		return "", "", "", false
	}
	repo, rest, _ = strings.Cut(after, "/")
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return "", "", "", false
	}
	return owner, repo, rest, true
}

func (g *giteaProvider) ParseRepositoryOrigin(raw string) (owner, repo, normalizedOrigin string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}
	web := g.webURL()

	// SSH remotes never include the HTTP subpath.
	if path, matched := parseSSHOrigin(raw, web.Host); matched {
		owner, repo = splitOwnerRepo(path)
		if owner == "" || repo == "" || strings.Contains(owner, "/") {
			return "", "", "", false
		}
		return owner, repo, g.BuildRepositoryURL(owner, repo), true
	}

	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.EqualFold(u.Host, web.Host) {
		return "", "", "", false
	}
	owner, repo, rest, ok := g.repoPath(strings.TrimSuffix(u.Path, "/"))
	if !ok || rest != "" {
		return "", "", "", false
	}
	return owner, repo, g.BuildRepositoryURL(owner, repo), true
}

func (g *giteaProvider) ParsePullRequestURL(raw string) (PRRef, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.EqualFold(u.Host, g.webURL().Host) {
		return PRRef{}, false
	}
	owner, repo, rest, ok := g.repoPath(u.Path)
	if !ok {
		return PRRef{}, false
	}
	// Pull request URLs are /{owner}/{repo}/pulls/{index}, optionally
	// followed by a tab such as /files.
	numStr, ok := strings.CutPrefix(rest, "pulls/")
	if !ok {
		return PRRef{}, false
	}
	numStr, _, _ = strings.Cut(numStr, "/")
	number, err := strconv.Atoi(numStr)
	if err != nil || number <= 0 {
		return PRRef{}, false
	}
	return PRRef{Owner: owner, Repo: repo, Number: number}, true
}

func (g *giteaProvider) NormalizePullRequestURL(raw string) string {
	ref, ok := g.ParsePullRequestURL(strings.TrimRight(
		strings.TrimSpace(raw),
		trailingPunctuation,
	))
	if !ok {
		return ""
	}
	return g.BuildPullRequestURL(ref)
}

func (g *giteaProvider) BuildBranchURL(owner, repo, branch string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return ""
	}
	return fmt.Sprintf(
		"%s/%s/%s/src/branch/%s",
		g.webBaseURL,
		url.PathEscape(owner),
		url.PathEscape(repo),
		escapePathPreserveSlashes(branch),
	)
}

func (g *giteaProvider) BuildRepositoryURL(owner, repo string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", g.webBaseURL, url.PathEscape(owner), url.PathEscape(repo))
}

func (g *giteaProvider) BuildPullRequestURL(ref PRRef) string {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/pulls/%d", g.webBaseURL, url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), ref.Number)
}

// giteaPullRequest is the subset of the pull request object used by the
// provider.
type giteaPullRequest struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	State        string `json:"state"`
	Merged       bool   `json:"merged"`
	Draft        bool   `json:"draft"`
	Additions    int64  `json:"additions"`
	Deletions    int64  `json:"deletions"`
	ChangedFiles int64  `json:"changed_files"`
	Head         struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	User *struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// normalizedReview is the review shape accepted by summarizeReviews.
type normalizedReview = struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (g *giteaProvider) repoAPIURL(owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s", g.apiBaseURL, url.PathEscape(owner), url.PathEscape(repo))
}

// ResolveBranchPullRequest finds the most recently updated open pull
// request whose head is ref.Branch in the same repository. Forgejo
// filters by head branch server-side with the head query parameter.
// Gitea ignores that parameter, so results are always filtered here as
// well, and up to giteaMaxBranchPages pages of open pull requests are
// scanned.
func (g *giteaProvider) ResolveBranchPullRequest(
	ctx context.Context,
	token string,
	ref BranchRef,
) (*PRRef, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return nil, nil //nolint:nilnil // Provider contract: nil, nil means no open PR.
	}
	fullName := ref.Owner + "/" + ref.Repo

	for page := 1; page <= giteaMaxBranchPages; page++ {
		query := url.Values{}
		query.Set("state", "open")
		query.Set("head", ref.Branch)
		query.Set("sort", "recentupdate")
		query.Set("limit", strconv.Itoa(giteaPageSize))
		query.Set("page", strconv.Itoa(page))

		var pulls []giteaPullRequest
		if _, err := g.decodeJSON(ctx, g.repoAPIURL(ref.Owner, ref.Repo)+"/pulls?"+query.Encode(), token, &pulls); err != nil {
			return nil, err
		}
		for _, pull := range pulls {
			if pull.Head.Ref != ref.Branch {
				continue
			}
			// Exclude pull requests opened from a fork that happens to
			// use the same branch name.
			if pull.Head.Repo != nil && !strings.EqualFold(pull.Head.Repo.FullName, fullName) {
				continue
			}
			return &PRRef{Owner: ref.Owner, Repo: ref.Repo, Number: pull.Number}, nil
		}
		if len(pulls) < giteaPageSize {
			break
		}
	}
	return nil, nil //nolint:nilnil // Provider contract: nil, nil means no open PR.
}

func (g *giteaProvider) FetchPullRequestStatus(
	ctx context.Context,
	token string,
	ref PRRef,
) (*PRStatus, error) {
	pullEndpoint := fmt.Sprintf("%s/pulls/%d", g.repoAPIURL(ref.Owner, ref.Repo), ref.Number)

	var pull giteaPullRequest
	if _, err := g.decodeJSON(ctx, pullEndpoint, token, &pull); err != nil {
		return nil, err
	}

	// The pull request object has no commit count. Request a single
	// commit and read the total from X-Total-Count. The stat, files,
	// and verification parameters skip work the count does not need.
	var commits []json.RawMessage
	header, err := g.decodeJSON(ctx, pullEndpoint+"/commits?limit=1&stat=false&files=false&verification=false", token, &commits)
	if err != nil {
		return nil, err
	}
	commitCount := int64(len(commits))
	if total, err := strconv.ParseInt(header.Get("X-Total-Count"), 10, 64); err == nil {
		commitCount = total
	}

	// Pull requests with more than one page of reviews are rare, so
	// only the first page is read, as the GitHub provider does.
	var reviews []struct {
		ID        int64  `json:"id"`
		State     string `json:"state"`
		Dismissed bool   `json:"dismissed"`
		User      *struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if _, err := g.decodeJSON(ctx, fmt.Sprintf("%s/reviews?limit=%d", pullEndpoint, giteaPageSize), token, &reviews); err != nil {
		return nil, err
	}

	// Map Gitea review states onto the GitHub states that
	// summarizeReviews understands. COMMENT, PENDING, and
	// REQUEST_REVIEW are not decisive and are skipped there.
	normalized := make([]normalizedReview, 0, len(reviews))
	for _, review := range reviews {
		if review.User == nil {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(review.State))
		switch {
		case review.Dismissed:
			state = "DISMISSED"
		case state == "REQUEST_CHANGES":
			state = "CHANGES_REQUESTED"
		}
		var entry normalizedReview
		entry.ID = review.ID
		entry.State = state
		entry.User.Login = review.User.Login
		normalized = append(normalized, entry)
	}
	reviewInfo := summarizeReviews(normalized)

	state := PRState(strings.ToLower(strings.TrimSpace(pull.State)))
	if pull.Merged {
		state = PRStateMerged
	}

	status := &PRStatus{
		Title:      pull.Title,
		State:      state,
		Draft:      pull.Draft,
		HeadSHA:    pull.Head.SHA,
		HeadBranch: pull.Head.Ref,
		DiffStats: DiffStats{
			Additions:    clampToInt32(pull.Additions),
			Deletions:    clampToInt32(pull.Deletions),
			ChangedFiles: clampToInt32(pull.ChangedFiles),
		},
		ChangesRequested: reviewInfo.changesRequested,
		Approved:         reviewInfo.approved,
		ReviewerCount:    reviewInfo.reviewerCount,
		BaseBranch:       pull.Base.Ref,
		PRNumber:         pull.Number,
		Commits:          clampToInt32(commitCount),
		FetchedAt:        g.clock.Now().UTC(),
	}
	if pull.User != nil {
		status.AuthorLogin = pull.User.Login
		status.AuthorAvatarURL = pull.User.AvatarURL
	}
	return status, nil
}

func (g *giteaProvider) FetchPullRequestDiff(
	ctx context.Context,
	token string,
	ref PRRef,
) (string, error) {
	diff, _, err := g.fetchDiff(ctx, fmt.Sprintf("%s/pulls/%d.diff", g.repoAPIURL(ref.Owner, ref.Repo), ref.Number), token)
	return diff, err
}

// FetchBranchDiff compares the branch with the repository's default
// branch. Gitea 1.27 and later return a raw diff from the compare API
// when output=diff is set. Older Gitea releases and Forgejo ignore the
// parameter and return JSON, and offer no other token-authenticated way
// to get the diff, so the provider reports errors.ErrUnsupported until
// a pull request exists.
func (g *giteaProvider) FetchBranchDiff(
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
	if _, err := g.decodeJSON(ctx, g.repoAPIURL(ref.Owner, ref.Repo), token, &repository); err != nil {
		return "", err
	}
	defaultBranch := strings.TrimSpace(repository.DefaultBranch)
	if defaultBranch == "" {
		return "", xerrors.New("gitea repository default branch is empty")
	}

	compare := escapePathPreserveSlashes(defaultBranch) + "..." + escapePathPreserveSlashes(ref.Branch)
	diff, isDiff, err := g.fetchDiff(ctx, g.repoAPIURL(ref.Owner, ref.Repo)+"/compare/"+compare+"?output=diff", token)
	if err != nil {
		return "", err
	}
	if !isDiff {
		return "", xerrors.Errorf("gitea server cannot diff a branch without a pull request: %w", errors.ErrUnsupported)
	}
	return diff, nil
}

func newGiteaRequest(ctx context.Context, requestURL, token, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, xerrors.Errorf("create gitea request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "coder-chat-diff-status")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// responseError builds the error for a non-200 response, preferring a
// RateLimitError when the response carries retry headers.
func (g *giteaProvider) responseError(resp *http.Response, what string) error {
	if rlErr := checkRateLimitError(resp, g.clock, "X-RateLimit-Reset"); rlErr != nil {
		return rlErr
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if readErr != nil {
		return xerrors.Errorf("gitea %s failed with status %d", what, resp.StatusCode)
	}
	return xerrors.Errorf(
		"gitea %s failed with status %d: %s",
		what,
		resp.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

// decodeJSON issues a GET request and decodes the JSON response into
// dest. It returns the response headers so callers can read pagination
// metadata such as X-Total-Count.
func (g *giteaProvider) decodeJSON(ctx context.Context, requestURL, token string, dest any) (http.Header, error) {
	req, err := newGiteaRequest(ctx, requestURL, token, "application/json")
	if err != nil {
		return nil, err
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("execute gitea request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.responseError(resp, "request")
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return nil, xerrors.Errorf("decode gitea response: %w", err)
	}
	return resp.Header, nil
}

// fetchDiff issues a GET request for a raw diff. It reports whether the
// response was a diff, as opposed to a JSON body from a server that
// ignored a request for raw output.
func (g *giteaProvider) fetchDiff(ctx context.Context, requestURL, token string) (diff string, isDiff bool, err error) {
	req, err := newGiteaRequest(ctx, requestURL, token, "text/plain")
	if err != nil {
		return "", false, err
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", false, xerrors.Errorf("execute gitea diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, g.responseError(resp, "diff request")
	}
	if mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil && mediaType == "application/json" {
		return "", false, nil
	}

	// Read one extra byte beyond MaxDiffSize so we can detect
	// whether the diff exceeds the limit.
	buf, err := io.ReadAll(io.LimitReader(resp.Body, MaxDiffSize+1))
	if err != nil {
		return "", false, xerrors.Errorf("read gitea diff response: %w", err)
	}
	if len(buf) > MaxDiffSize {
		return "", false, ErrDiffTooLarge
	}
	return string(buf), true, nil
}
