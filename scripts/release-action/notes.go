package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// generateReleaseNotes produces markdown release notes for the given
// version range by examining the commit log and PR metadata.
func generateReleaseNotes(exec CommandExecutor, newVersion, previousVersion version) (string, error) {
	// Build commit range. If the new tag doesn't exist locally yet,
	// fall back to ..HEAD.
	newTag := newVersion.String()
	commitRange := fmt.Sprintf("%s...%s", previousVersion.String(), newTag)
	if err := gitRun(exec, "rev-parse", "--verify", newTag); err != nil {
		commitRange = fmt.Sprintf("%s..HEAD", previousVersion.String())
	}

	commits, err := commitLog(exec, commitRange)
	if err != nil {
		return "", xerrors.Errorf("commit log: %w", err)
	}

	// Extract PR numbers from commit titles and fetch metadata.
	prMeta := ghBuildPullRequestMap(exec, extractPRNumbers(commits))

	// Section definitions in display order.
	type section struct {
		key   string
		title string
	}
	sections := []section{
		{"breaking", "BREAKING CHANGES"},
		{"security", "Security"},
		{"feat", "Features"},
		{"fix", "Bug fixes"},
		{"docs", "Documentation"},
		{"refactor", "Code refactoring"},
		{"perf", "Performance"},
		{"test", "Tests"},
		{"build", "Build"},
		{"ci", "CI"},
		{"chore", "Chores"},
		{"revert", "Reverts"},
		{"other", "Other changes"},
		{"experimental", "Experimental"},
	}

	// Categorize commits into sections.
	buckets := make(map[string][]commitEntry)
	for _, c := range commits {
		// Skip dependabot commits.
		if isDependabot(c.Title) {
			continue
		}

		var labels []string
		for _, prNum := range parsePRNumbers(c.Title) {
			if meta, ok := prMeta[prNum]; ok {
				labels = append(labels, meta.Labels...)
			}
		}
		cat := categorizeCommit(c.Title, labels)
		buckets[cat] = append(buckets[cat], c)
	}

	var b strings.Builder

	// RC note based on version.
	if newVersion.IsRC() {
		_, _ = b.WriteString("> [!NOTE]\n")
		_, _ = b.WriteString("> This is a **release candidate** build of Coder. Release candidate builds are not intended for production use. Learn more about our [Release Schedule](https://coder.com/docs/install/releases).\n\n")
	}

	_, _ = b.WriteString("## Changelog\n\n")

	for _, sec := range sections {
		entries, ok := buckets[sec.key]
		if !ok || len(entries) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(&b, "### %s\n\n", sec.title)
		for _, e := range entries {
			title := humanizeTitle(e.Title)
			if prNums := parsePRNumbers(e.Title); len(prNums) > 0 {
				// Strip the trailing PR reference from the title since
				// we add it as a link.
				title = stripPRRef(title)
				_, _ = fmt.Fprintf(&b, "- %s (#%d)\n", title, prNums[0])
			} else {
				_, _ = fmt.Fprintf(&b, "- %s\n", title)
			}
		}
		_, _ = b.WriteString("\n")
	}

	// Compare link.
	_, _ = fmt.Fprintf(&b, "Compare: [`%s...%s`](https://github.com/%s/%s/compare/%s...%s)\n\n",
		previousVersion.String(), newVersion.String(),
		owner, repo,
		previousVersion.String(), newVersion.String())

	// Container image.
	_, _ = b.WriteString("## Container image\n\n")
	_, _ = fmt.Fprintf(&b, "- `docker pull ghcr.io/%s/%s:%s`\n\n", owner, repo, newVersion.String())

	// Install/upgrade links.
	_, _ = b.WriteString("## Install/upgrade\n\n")
	_, _ = b.WriteString("Refer to our docs to [install](https://coder.com/docs/install) or [upgrade](https://coder.com/docs/admin/upgrade) Coder, or use a release asset below.\n")

	return b.String(), nil
}

// isDependabot returns true if the commit title looks like it came
// from dependabot.
func isDependabot(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "dependabot") ||
		strings.HasPrefix(lower, "chore(deps):")
}

// prNumRe matches GitHub's "(#NNN)" PR reference convention.
var prNumRe = regexp.MustCompile(`\(#(\d+)\)`)

// parsePRNumbers extracts all PR numbers from a commit title.
func parsePRNumbers(title string) []int {
	var nums []int
	for _, m := range prNumRe.FindAllStringSubmatch(title, -1) {
		num, _ := strconv.Atoi(m[1])
		nums = append(nums, num)
	}
	return nums
}

// extractPRNumbers collects all unique PR numbers from a list of commits.
func extractPRNumbers(commits []commitEntry) []int {
	seen := make(map[int]bool)
	var nums []int
	for _, c := range commits {
		for _, num := range parsePRNumbers(c.Title) {
			if !seen[num] {
				seen[num] = true
				nums = append(nums, num)
			}
		}
	}
	return nums
}

// stripPRRef removes a trailing (#NNN) from a title.
func stripPRRef(title string) string {
	if idx := strings.LastIndex(title, "(#"); idx >= 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
}
