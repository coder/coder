package main

import (
	"errors"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ghOutput runs a gh CLI command and returns trimmed stdout.
func ghOutput(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", exitErr
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// checkGHAuth verifies that the gh CLI is installed and
// authenticated. Returns true if gh is available.
func checkGHAuth() bool {
	cmd := exec.Command("gh", "auth", "status")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// ghPR is a minimal pull request representation parsed from gh CLI
// JSON output.
type ghPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Labels []string
}

// ghListOpenPRs returns open PRs targeting the given branch via
// the gh CLI.
func ghListOpenPRs(branch string) ([]ghPR, error) {
	out, err := ghOutput("pr", "list",
		"--repo", owner+"/"+repo,
		"--base", branch,
		"--state", "open",
		"--json", "number,title,author",
		"--jq", `.[] | "\(.number)\t\(.title)\t\(.author.login)"`,
	)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var prs []ghPR
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		num, _ := strconv.Atoi(parts[0])
		prs = append(prs, ghPR{
			Number: num,
			Title:  parts[1],
			Author: parts[2],
		})
	}
	return prs, nil
}

// ghListPRsWithLabel returns merged PRs targeting the given branch
// that have a specific label.
func ghListPRsWithLabel(branch, label string) ([]ghPR, error) {
	out, err := ghOutput("pr", "list",
		"--repo", owner+"/"+repo,
		"--base", branch,
		"--state", "merged",
		"--label", label,
		"--json", "number,title",
		"--jq", `.[] | "\(.number)\t\(.title)"`,
	)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var prs []ghPR
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		num, _ := strconv.Atoi(parts[0])
		prs = append(prs, ghPR{Number: num, Title: parts[1]})
	}
	return prs, nil
}

// prMetadata holds labels and author for a merged PR.
type prMetadata struct {
	Labels []string
	Author string
}

// ghBuildPRMetadataMap returns a map of full merge-commit SHA →
// metadata (labels and author) for merged PRs targeting main. This
// matches the bash script's approach of querying --base main with a
// date filter based on the oldest commit in the range.
func ghBuildPRMetadataMap(commits []commitEntry) (map[string]prMetadata, error) {
	if len(commits) == 0 {
		return make(map[string]prMetadata), nil
	}
	// Find the earliest commit timestamp to scope the PR query.
	earliest := commits[0].Timestamp
	for _, c := range commits[1:] {
		if c.Timestamp < earliest {
			earliest = c.Timestamp
		}
	}
	lookbackDate := time.Unix(earliest, 0).Format("2006-01-02")

	out, err := ghOutput("pr", "list",
		"--repo", owner+"/"+repo,
		"--base", "main",
		"--state", "merged",
		"--limit", "10000",
		"--search", "merged:>="+lookbackDate,
		"--json", "mergeCommit,labels,author",
		"--jq", `.[] | "\(.mergeCommit.oid)\t\(.author.login)\t\([.labels[].name] | join(","))"`,
	)
	if err != nil {
		return nil, err
	}
	result := make(map[string]prMetadata)
	if out == "" {
		return result, nil
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		sha := parts[0]
		author := parts[1]
		var labels []string
		if parts[2] != "" {
			labels = strings.Split(parts[2], ",")
			sort.Strings(labels)
		}
		result[sha] = prMetadata{
			Labels: labels,
			Author: author,
		}
	}
	return result, nil
}
