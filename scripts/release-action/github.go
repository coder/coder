package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"
)

// ghOutput runs a gh CLI command and returns trimmed stdout.
func ghOutput(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pullRequest holds metadata about a GitHub pull request.
type pullRequest struct {
	Number int
	Labels []string
	Author string
}

// pullRequestMap holds PR metadata indexed by PR number.
type pullRequestMap map[int]pullRequest

// ghBuildPullRequestMap builds a map of PR number to metadata by
// querying the GitHub API via the gh CLI for all PRs referenced in
// the commit list.
func ghBuildPullRequestMap(commits []commitEntry) pullRequestMap {
	m := make(pullRequestMap)

	// Collect unique PR numbers.
	prNums := make(map[int]bool)
	for _, c := range commits {
		for _, num := range c.PRNumbers {
			prNums[num] = true
		}
	}

	for prNum := range prNums {
		out, err := ghOutput("pr", "view", fmt.Sprintf("%d", prNum),
			"--repo", fmt.Sprintf("%s/%s", owner, repo),
			"--json", "number,labels,author")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to fetch PR #%d metadata: %v\n", prNum, err)
			continue
		}

		var result struct {
			Number int `json:"number"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to parse PR #%d metadata: %v\n", prNum, err)
			continue
		}

		var labels []string
		for _, l := range result.Labels {
			labels = append(labels, l.Name)
		}

		m[result.Number] = pullRequest{
			Number: result.Number,
			Labels: labels,
			Author: result.Author.Login,
		}
	}

	return m
}

// openPR represents an open pull request targeting a branch.
type openPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	URL string `json:"url"`
}

// checkOpenPRs verifies that no pull requests are open against the
// given branch. If any are found, it returns an error listing them
// with instructions to merge or close before releasing.
func checkOpenPRs(branch string) error {
	out, err := ghOutput("pr", "list",
		"--repo", fmt.Sprintf("%s/%s", owner, repo),
		"--base", branch,
		"--state", "open",
		"--json", "number,title,author,url",
		"--limit", "100")
	if err != nil {
		return xerrors.Errorf("failed to list open PRs for branch %s: %w", branch, err)
	}

	var prs []openPR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return xerrors.Errorf("failed to parse open PRs response: %w", err)
	}

	if len(prs) == 0 {
		return nil
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "found %d open pull request(s) targeting %s that must be merged or closed before releasing:\n\n", len(prs), branch)
	for _, pr := range prs {
		_, _ = fmt.Fprintf(&b, "  - #%d: %s (by @%s)\n    %s\n", pr.Number, pr.Title, pr.Author.Login, pr.URL)
	}
	_, _ = fmt.Fprintf(&b, "\nMerge or close these pull requests, then re-run the release workflow.")
	return xerrors.New(b.String())
}
