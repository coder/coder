package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

// prMetadata holds PR labels and author for release notes.
type prMetadata struct {
	labels []string
	author string
}

// prMetadataMaps holds PR metadata indexed by PR number.
type prMetadataMaps map[int]prMetadata

// ghBuildPRMetadataMap builds a map of PR number to metadata by
// querying the GitHub API via the gh CLI for all PRs referenced in
// the commit list.
func ghBuildPRMetadataMap(commits []commitEntry) prMetadataMaps {
	m := make(prMetadataMaps)

	// Collect unique PR numbers.
	prNums := make(map[int]bool)
	for _, c := range commits {
		if c.PRCount > 0 {
			prNums[c.PRCount] = true
		}
	}

	for prNum := range prNums {
		out, err := ghOutput("pr", "view", fmt.Sprintf("%d", prNum),
			"--repo", fmt.Sprintf("%s/%s", owner, repo),
			"--json", "number,labels,author")
		if err != nil {
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
			continue
		}

		var labels []string
		for _, l := range result.Labels {
			labels = append(labels, l.Name)
		}

		m[result.Number] = prMetadata{
			labels: labels,
			author: result.Author.Login,
		}
	}

	return m
}
