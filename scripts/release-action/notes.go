package main

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"
)

// generateReleaseNotes produces markdown release notes for the given
// version range by examining the commit log and PR metadata.
func generateReleaseNotes(newVersion, previousVersion version, channel string) (string, error) {
	// Build commit range. If the new tag doesn't exist locally yet,
	// fall back to ..HEAD.
	newTag := newVersion.String()
	commitRange := fmt.Sprintf("%s...%s", previousVersion.String(), newTag)
	if err := gitRun("rev-parse", "--verify", newTag); err != nil {
		commitRange = fmt.Sprintf("%s..HEAD", previousVersion.String())
	}

	commits, err := commitLog(commitRange)
	if err != nil {
		return "", xerrors.Errorf("commit log: %w", err)
	}

	// Build PR metadata map for label lookup.
	prMeta := ghBuildPullRequestMap(commits)

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
		for _, prNum := range c.PRNumbers {
			if meta, ok := prMeta[prNum]; ok {
				labels = append(labels, meta.Labels...)
			}
		}
		cat := categorizeCommit(c.Title, labels)
		buckets[cat] = append(buckets[cat], c)
	}

	var b strings.Builder

	// RC channel note.
	if channel == "rc" {
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
			if len(e.PRNumbers) > 0 {
				// Strip the trailing PR reference from the title since
				// we add it as a link.
				title = stripPRRef(title)
				_, _ = fmt.Fprintf(&b, "- %s (#%d)\n", title, e.PRNumbers[0])
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

// stripPRRef removes a trailing (#NNN) from a title.
func stripPRRef(title string) string {
	if idx := strings.LastIndex(title, "(#"); idx >= 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
}
