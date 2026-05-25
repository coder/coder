package gitprovider

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

type gitlabProvider struct {
	webBaseURL string
	client     *gitlab.Client
	clock      quartz.Clock
}

func newGitLab(baseURL string, httpClient *http.Client, clock quartz.Clock) (*gitlabProvider, error) {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/v4")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	client, err := gitlab.NewClient("",
		gitlab.WithBaseURL(baseURL),
		gitlab.WithHTTPClient(httpClient),
		gitlab.WithoutRetries(),
	)
	if err != nil {
		return nil, xerrors.Errorf("create gitlab client: %w", err)
	}

	return &gitlabProvider{
		webBaseURL: baseURL,
		client:     client,
		clock:      clock,
	}, nil
}

var _ Provider = (*gitlabProvider)(nil)

// webHost returns the hostname (with port if present) of the GitLab web URL.
func (g *gitlabProvider) webHost() string {
	u, err := url.Parse(g.webBaseURL)
	if err != nil {
		return "gitlab.com"
	}
	return u.Host
}

// reqOpts returns per-request options for authentication and context.
func reqOpts(ctx context.Context, token string) []gitlab.RequestOptionFunc {
	opts := []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)}
	if token != "" {
		opts = append(opts, gitlab.WithToken(gitlab.OAuthToken, token))
	}
	return opts
}

// gitLabPID returns the full project path (owner/repo) for use as a pid.
// The library handles URL encoding internally.
func gitLabPID(owner, repo string) string {
	return owner + "/" + repo
}

func (g *gitlabProvider) FetchPullRequestStatus(
	ctx context.Context,
	token string,
	ref PRRef,
) (*PRStatus, error) {
	pid := gitLabPID(ref.Owner, ref.Repo)
	opts := reqOpts(ctx, token)

	// Fetch merge request details.
	mr, _, err := g.client.MergeRequests.GetMergeRequest(pid, int64(ref.Number), nil, opts...)
	if err != nil {
		return nil, g.wrapError(err, "get merge request")
	}

	// Fetch approvals.
	approvals, _, err := g.client.MergeRequests.GetMergeRequestApprovals(pid, int64(ref.Number), opts...)
	if err != nil {
		return nil, g.wrapError(err, "get merge request approvals")
	}

	// Fetch commits to get the commit count.
	var totalCommits int32
	commits, resp, err := g.client.MergeRequests.GetMergeRequestCommits(
		pid, int64(ref.Number),
		&gitlab.GetMergeRequestCommitsOptions{ListOptions: gitlab.ListOptions{PerPage: 100}},
		opts...,
	)
	if err != nil {
		return nil, g.wrapError(err, "get merge request commits")
	}
	if resp.TotalItems > 0 {
		totalCommits = int32(resp.TotalItems)
	} else {
		totalCommits = int32(len(commits))
	}

	// Fetch MR diffs to compute additions/deletions.
	// The commits endpoint does not return per-commit stats, so we
	// count +/- lines from the unified diff returned by this endpoint.
	var additions, deletions int32
	diffs, _, err := g.client.MergeRequests.ListMergeRequestDiffs(
		pid, int64(ref.Number),
		// NOTE: fetches a single page of up to 100 diffs. MRs with more than
		// 100 changed files will have correct ChangedFiles (from MR metadata)
		// but undercounted Additions/Deletions. Pagination is omitted because
		// the gitsync worker only uses ChangedFiles for its heuristics today.
		&gitlab.ListMergeRequestDiffsOptions{ListOptions: gitlab.ListOptions{PerPage: 100}},
		opts...,
	)
	if err != nil {
		return nil, g.wrapError(err, "list merge request diffs")
	}
	for _, d := range diffs {
		diffAdditions, diffDeletions := countDiffLines(d.Diff)
		additions += diffAdditions
		deletions += diffDeletions
	}

	// Map GitLab state to normalized state.
	state := mapGitLabState(mr.State)

	// Use diff_refs.head_sha if available, fall back to top-level sha.
	headSHA := cmp.Or(mr.DiffRefs.HeadSha, mr.SHA)

	// Parse changes_count (it's a string, possibly "1000+").
	var changedFiles int32
	if mr.ChangesCount != "" {
		trimmed := strings.TrimSuffix(mr.ChangesCount, "+")
		if n, err := strconv.Atoi(trimmed); err == nil {
			changedFiles = int32(n)
		}
	}

	// TODO(CODAGT-440): These fields have semantic gaps vs the GitHub
	// provider. GitLab's "Approved" is threshold-based (not "at least one
	// approval and no changes requested"), ChangesRequested has no GitLab
	// equivalent, and ReviewerCount only counts approvers.
	reviewerCount := int32(len(approvals.ApprovedBy))

	var authorLogin, authorAvatarURL string
	if mr.Author != nil {
		authorLogin = mr.Author.Username
		authorAvatarURL = mr.Author.AvatarURL
	}

	return &PRStatus{
		Title:      mr.Title,
		State:      state,
		Draft:      mr.Draft,
		HeadSHA:    headSHA,
		HeadBranch: mr.SourceBranch,
		DiffStats: DiffStats{
			Additions:    additions,
			Deletions:    deletions,
			ChangedFiles: changedFiles,
		},
		ChangesRequested: false,
		Approved:         approvals.Approved,
		ReviewerCount:    reviewerCount,
		AuthorLogin:      authorLogin,
		AuthorAvatarURL:  authorAvatarURL,
		BaseBranch:       mr.TargetBranch,
		PRNumber:         int(mr.IID),
		Commits:          totalCommits,
		FetchedAt:        g.clock.Now().UTC(),
	}, nil
}

func (g *gitlabProvider) ResolveBranchPullRequest(
	ctx context.Context,
	token string,
	ref BranchRef,
) (*PRRef, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return nil, nil
	}

	pid := gitLabPID(ref.Owner, ref.Repo)
	opts := reqOpts(ctx, token)

	mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(pid, &gitlab.ListProjectMergeRequestsOptions{
		ListOptions:  gitlab.ListOptions{PerPage: 1},
		SourceBranch: gitlab.Ptr(ref.Branch),
		State:        gitlab.Ptr("opened"),
		OrderBy:      gitlab.Ptr("updated_at"),
		Sort:         gitlab.Ptr("desc"),
	}, opts...)
	if err != nil {
		return nil, g.wrapError(err, "list merge requests by branch")
	}
	if len(mrs) == 0 {
		return nil, nil
	}

	prRef, ok := g.ParsePullRequestURL(mrs[0].WebURL)
	if !ok {
		// Fallback: construct from known owner/repo and returned IID.
		return &PRRef{
			Owner:  ref.Owner,
			Repo:   ref.Repo,
			Number: int(mrs[0].IID),
		}, nil
	}
	return &prRef, nil
}

func (g *gitlabProvider) FetchPullRequestDiff(
	ctx context.Context,
	token string,
	ref PRRef,
) (string, error) {
	pid := gitLabPID(ref.Owner, ref.Repo)

	// Make a direct HTTP request instead of using the library's
	// ShowMergeRequestRawDiffs, which reads the entire response
	// into memory before returning. We use io.LimitReader to
	// bound memory and reject diffs exceeding MaxDiffSize.
	rawURL := fmt.Sprintf("%sprojects/%s/merge_requests/%d/raw_diffs",
		g.client.BaseURL().String(), url.PathEscape(pid), ref.Number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", g.wrapError(err, "create raw diffs request")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.client.HTTPClient().Do(req)
	if err != nil {
		return "", g.wrapError(err, "get merge request raw diffs")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if rlErr := checkRateLimitError(resp, g.clock, "RateLimit-Reset"); rlErr != nil {
			return "", rlErr
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return "", g.wrapError(
				xerrors.Errorf("unexpected status %d", resp.StatusCode),
				"get merge request raw diffs",
			)
		}
		return "", g.wrapError(
			xerrors.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
			"get merge request raw diffs",
		)
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, MaxDiffSize+1))
	if err != nil {
		return "", g.wrapError(err, "read merge request raw diffs")
	}
	if len(buf) > MaxDiffSize {
		return "", ErrDiffTooLarge
	}
	return string(buf), nil
}

// compareResponse is the subset of GitLab's compare endpoint response
// that we need. We decode manually (instead of using the library) so
// we can bound memory with io.LimitReader before JSON parsing.
type compareResponse struct {
	Diffs []struct {
		Diff        string `json:"diff"`
		OldPath     string `json:"old_path"`
		NewPath     string `json:"new_path"`
		NewFile     bool   `json:"new_file"`
		DeletedFile bool   `json:"deleted_file"`
		RenamedFile bool   `json:"renamed_file"`
		Collapsed   bool   `json:"collapsed"`
		TooLarge    bool   `json:"too_large"`
	} `json:"diffs"`
	CompareTimeout bool `json:"compare_timeout"`
}

func (g *gitlabProvider) FetchBranchDiff(
	ctx context.Context,
	token string,
	ref BranchRef,
) (string, error) {
	if ref.Owner == "" || ref.Repo == "" || ref.Branch == "" {
		return "", nil
	}

	pid := gitLabPID(ref.Owner, ref.Repo)
	opts := reqOpts(ctx, token)

	// Get the default branch from the project.
	project, _, err := g.client.Projects.GetProject(pid, nil, opts...)
	if err != nil {
		return "", g.wrapError(err, "get project")
	}
	defaultBranch := strings.TrimSpace(project.DefaultBranch)
	if defaultBranch == "" {
		return "", xerrors.New("gitlab project default branch is empty")
	}

	// Use raw HTTP with io.LimitReader to bound memory. The library's
	// Compare() decodes the full response before returning, which
	// would allow a maliciously large diff to OOM the process.
	compareURL := fmt.Sprintf("%sprojects/%s/repository/compare?from=%s&to=%s&unidiff=true",
		g.client.BaseURL().String(),
		url.PathEscape(pid),
		url.QueryEscape(defaultBranch),
		url.QueryEscape(ref.Branch),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, compareURL, nil)
	if err != nil {
		return "", g.wrapError(err, "create compare request")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.client.HTTPClient().Do(req)
	if err != nil {
		return "", g.wrapError(err, "compare branches")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if rlErr := checkRateLimitError(resp, g.clock, "RateLimit-Reset"); rlErr != nil {
			return "", rlErr
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if readErr != nil {
			return "", g.wrapError(
				xerrors.Errorf("unexpected status %d", resp.StatusCode),
				"compare branches",
			)
		}
		return "", g.wrapError(
			xerrors.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
			"compare branches",
		)
	}

	// Bound the read to MaxDiffSize + overhead for JSON structure.
	// The JSON envelope (commits, metadata) adds some overhead beyond
	// the raw diff content, so we allow ~10% extra for framing.
	maxRead := int64(MaxDiffSize) + int64(MaxDiffSize/10) + 4096
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRead+1))
	if err != nil {
		return "", g.wrapError(err, "read compare response")
	}
	if int64(len(body)) > maxRead {
		return "", ErrDiffTooLarge
	}

	var compare compareResponse
	if err := json.Unmarshal(body, &compare); err != nil {
		return "", g.wrapError(err, "decode compare response")
	}
	if compare.CompareTimeout {
		return "", xerrors.New("gitlab compare timed out; diff may be incomplete")
	}

	// Reconstruct unified diff from individual file diffs.
	var sb strings.Builder
	var estimated int
	for _, d := range compare.Diffs {
		estimated += len(d.Diff) + len(d.OldPath) + len(d.NewPath) + 20
	}
	if estimated > MaxDiffSize {
		return "", ErrDiffTooLarge
	}
	sb.Grow(estimated)
	for _, d := range compare.Diffs {
		if d.Collapsed || d.TooLarge {
			slog.WarnContext(ctx, "gitlab compare: file diff truncated",
				slog.String("path", d.NewPath),
				slog.Bool("collapsed", d.Collapsed),
				slog.Bool("too_large", d.TooLarge),
			)
		}
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", d.OldPath, d.NewPath)
		// Add standard unified diff file headers.
		switch {
		case d.NewFile:
			sb.WriteString("--- /dev/null\n")
			fmt.Fprintf(&sb, "+++ b/%s\n", d.NewPath)
		case d.DeletedFile:
			fmt.Fprintf(&sb, "--- a/%s\n", d.OldPath)
			sb.WriteString("+++ /dev/null\n")
		default:
			fmt.Fprintf(&sb, "--- a/%s\n", d.OldPath)
			fmt.Fprintf(&sb, "+++ b/%s\n", d.NewPath)
		}
		sb.WriteString(d.Diff)
		// Ensure each file diff ends with a newline.
		if len(d.Diff) > 0 && d.Diff[len(d.Diff)-1] != '\n' {
			sb.WriteByte('\n')
		}
	}

	result := sb.String()
	if len(result) > MaxDiffSize {
		return "", ErrDiffTooLarge
	}
	return result, nil
}

// ParseRepositoryOrigin preserves slashes in owner because GitLab supports
// subgroup paths such as group/subgroup/repo.
//
// TODO: this does not handle GitLab instances installed under a relative URL
// prefix (e.g. https://example.com/gitlab/). See
// https://docs.gitlab.com/install/relative_url/ for details.
func (g *gitlabProvider) ParseRepositoryOrigin(raw string) (owner, repo, normalizedOrigin string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}

	host := g.webHost()

	// Try SSH format: git@HOST:path.git or ssh://git@HOST/path.git
	if path, matched := g.parseSSHOrigin(raw, host); matched {
		owner, repo = splitOwnerRepo(path)
		if owner == "" || repo == "" {
			return "", "", "", false
		}
		normalized := fmt.Sprintf("%s/%s/%s", g.webBaseURL, owner, repo)
		return owner, repo, normalized, true
	}

	// Try HTTPS format.
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", false
	}
	if !strings.EqualFold(u.Host, host) {
		return "", "", "", false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", "", "", false
	}

	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if path == "" {
		return "", "", "", false
	}

	owner, repo = splitOwnerRepo(path)
	if owner == "" || repo == "" {
		return "", "", "", false
	}

	normalized := fmt.Sprintf("%s/%s/%s", g.webBaseURL, owner, repo)
	return owner, repo, normalized, true
}

func (g *gitlabProvider) ParsePullRequestURL(raw string) (PRRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PRRef{}, false
	}

	u, err := url.Parse(raw)
	if err != nil {
		return PRRef{}, false
	}

	host := g.webHost()
	if !strings.EqualFold(u.Host, host) {
		return PRRef{}, false
	}

	// GitLab MR URLs: /owner/repo/-/merge_requests/123
	// or /group/subgroup/repo/-/merge_requests/123
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, "/")

	// Find "-/merge_requests/NUMBER" in the path.
	const mrMarker = "-/merge_requests/"
	idx := strings.Index(path, mrMarker)
	if idx < 0 {
		return PRRef{}, false
	}

	// Everything before the marker (minus trailing slash) is the project path.
	projPath := path[:idx]
	projPath = strings.TrimSuffix(projPath, "/")
	if projPath == "" {
		return PRRef{}, false
	}

	// The number comes after the marker.
	afterMR := path[idx+len(mrMarker):]
	// Strip any trailing path segments.
	if slashIdx := strings.Index(afterMR, "/"); slashIdx >= 0 {
		afterMR = afterMR[:slashIdx]
	}

	number, err := strconv.Atoi(afterMR)
	if err != nil || number <= 0 {
		return PRRef{}, false
	}

	owner, repo := splitOwnerRepo(projPath)
	if owner == "" || repo == "" {
		return PRRef{}, false
	}

	return PRRef{
		Owner:  owner,
		Repo:   repo,
		Number: number,
	}, true
}

// NormalizePullRequestURL normalizes a GitLab merge request URL.
func (g *gitlabProvider) NormalizePullRequestURL(raw string) string {
	ref, ok := g.ParsePullRequestURL(strings.TrimRight(
		strings.TrimSpace(raw),
		trailingPunctuation,
	))
	if !ok {
		return ""
	}
	return g.BuildPullRequestURL(ref)
}

// BuildBranchURL keeps owner and repo unescaped because GitLab owners can
// include subgroup paths with slashes.
func (g *gitlabProvider) BuildBranchURL(owner, repo, branch string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return ""
	}

	return fmt.Sprintf(
		"%s/%s/%s/-/tree/%s",
		g.webBaseURL,
		owner,
		repo,
		escapePathPreserveSlashes(branch),
	)
}

// BuildRepositoryURL keeps owner and repo unescaped because GitLab owners can
// include subgroup paths with slashes.
func (g *gitlabProvider) BuildRepositoryURL(owner, repo string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", g.webBaseURL, owner, repo)
}

func (g *gitlabProvider) BuildPullRequestURL(ref PRRef) string {
	if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/-/merge_requests/%d", g.webBaseURL, ref.Owner, ref.Repo, ref.Number)
}

// wrapError converts library errors to our domain errors (e.g. rate limits).
func (g *gitlabProvider) wrapError(err error, action string) error {
	if errResp, ok := errors.AsType[*gitlab.ErrorResponse](err); ok {
		if rlErr := checkRateLimitError(errResp.Response, g.clock, "RateLimit-Reset"); rlErr != nil {
			return rlErr
		}
	}
	return xerrors.Errorf("gitlab %s: %w", action, err)
}

// mapGitLabState maps a GitLab merge request state string to a normalized PRState.
func mapGitLabState(state string) PRState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "opened":
		return PRStateOpen
	case "merged":
		return PRStateMerged
	case "closed", "locked":
		return PRStateClosed
	default:
		return PRStateClosed
	}
}

// splitOwnerRepo splits a path like "group/subgroup/repo" into
// owner="group/subgroup" and repo="repo". The last segment is always
// the repo name, and everything before it is the owner.
func splitOwnerRepo(path string) (owner, repo string) {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "", ""
	}

	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		// No slash means no owner/repo split possible.
		return "", ""
	}

	owner = path[:lastSlash]
	repo = path[lastSlash+1:]
	if owner == "" || repo == "" {
		return "", ""
	}
	return owner, repo
}

// parseSSHOrigin attempts to parse an SSH git remote URL for the given host.
// Returns the path (without .git suffix) and true if it matched.
func (g *gitlabProvider) parseSSHOrigin(raw string, host string) (string, bool) {
	// Handle ssh://git@HOST/path.git format.
	if strings.HasPrefix(raw, "ssh://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false
		}
		// The host in SSH URLs may include a port, so compare case-insensitively.
		if !strings.EqualFold(u.Host, host) && !strings.EqualFold(u.Hostname(), hostWithoutPort(host)) {
			return "", false
		}
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			return "", false
		}
		return path, true
	}

	// Handle git@HOST:path.git format (SCP-like syntax).
	prefix := "git@" + host + ":"
	// Also try matching without port for host comparison.
	prefixNoPort := "git@" + hostWithoutPort(host) + ":"

	path, ok := strings.CutPrefix(raw, prefix)
	if !ok {
		path, ok = strings.CutPrefix(raw, prefixNoPort)
	}
	if !ok {
		return "", false
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "", false
	}
	return path, true
}

// hostWithoutPort strips the port from a host:port string.
func hostWithoutPort(host string) string {
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}
