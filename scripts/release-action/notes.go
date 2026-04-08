package main

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// generateReleaseNotes produces release notes markdown for the
// given version range and channel.
func generateReleaseNotes(newVersion, previousVersion version, channel string) (string, error) {
	commitRange := previousVersion.String() + "..." + newVersion.String()

	commits, err := commitLog(commitRange)
	if err != nil {
		// Fall back to previousVersion..HEAD when the new tag
		// does not exist yet locally (common in CI before the
		// tag is pushed).
		commitRange = previousVersion.String() + "..HEAD"
		commits, err = commitLog(commitRange)
		if err != nil {
			return "", xerrors.Errorf("reading commit log for %s: %w", commitRange, err)
		}
	}

	// Build PR metadata maps via gh CLI for label and author
	// lookups. A failure here is non-fatal; we just lose label
	// and author information.
	prMeta, err := ghBuildPRMetadataMap(commits)
	if err != nil {
		prMeta = &prMetadataMaps{
			bySHA:    make(map[string]prMetadata),
			byNumber: make(map[int]prMetadata),
		}
	}

	type section struct {
		Key   string
		Title string
	}
	sections := []section{
		{"breaking", "BREAKING CHANGES"},
		{"security", "SECURITY"},
		{"feat", "Features"},
		{"fix", "Bug fixes"},
		{"docs", "Documentation"},
		{"refactor", "Code refactoring"},
		{"perf", "Performance improvements"},
		{"test", "Tests"},
		{"build", "Builds"},
		{"ci", "Continuous integration"},
		{"chore", "Chores"},
		{"revert", "Reverts"},
		{"other", "Other changes"},
		{"experimental", "Experimental changes"},
	}
	sectionCommits := make(map[string][]string)

	for _, c := range commits {
		meta := prMeta.lookupCommit(c.FullSHA, c.PRCount)
		if meta.Author == "dependabot" || meta.Author == "app/dependabot" {
			continue
		}
		cat := categorizeCommit(c.Title, meta.Labels)
		humanTitle := humanizeTitle(c.Title)
		humanTitle = prNumRe.ReplaceAllString(humanTitle, "")
		humanTitle = strings.TrimSpace(humanTitle)

		var entry string
		if c.PRCount > 0 {
			entry = fmt.Sprintf("- %s (#%d, %s)", humanTitle, c.PRCount, c.SHA)
		} else {
			entry = fmt.Sprintf("- %s (%s)", humanTitle, c.SHA)
		}
		if meta.Author != "" {
			entry += fmt.Sprintf(" (@%s)", meta.Author)
		}
		sectionCommits[cat] = append(sectionCommits[cat], entry)
	}

	var notes strings.Builder

	if channel == "stable" {
		_, _ = fmt.Fprintf(&notes, "> ## Stable (since %s)\n\n", time.Now().Format("January 02, 2006"))
	}
	_, _ = fmt.Fprintln(&notes, "## Changelog")

	switch channel {
	case "rc":
		_, _ = fmt.Fprintln(&notes)
		_, _ = fmt.Fprintln(&notes, "> [!NOTE]")
		_, _ = fmt.Fprintln(&notes, "> This is a **release candidate** (RC) for testing purposes. It is not recommended for production use. Please report any issues you encounter. Learn more about our [Release Schedule](https://coder.com/docs/install/releases).")
	case "mainline":
		_, _ = fmt.Fprintln(&notes)
		_, _ = fmt.Fprintln(&notes, "> [!NOTE]")
		_, _ = fmt.Fprintln(&notes, "> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/install/releases).")
	}

	hasContent := false
	for _, s := range sections {
		entries, ok := sectionCommits[s.Key]
		if !ok || len(entries) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(&notes, "\n### %s\n\n", s.Title)
		if s.Key == "experimental" {
			_, _ = fmt.Fprintln(&notes, "These changes are feature-flagged and can be enabled with the `--experiments` server flag. They may change or be removed in future releases.")
			_, _ = fmt.Fprintln(&notes)
		}
		for _, e := range entries {
			_, _ = fmt.Fprintln(&notes, e)
		}
		hasContent = true
	}

	if !hasContent {
		_, _ = fmt.Fprintf(&notes, "\n_No changes since %s._\n", previousVersion.String())
	}

	// Compare link.
	_, _ = fmt.Fprintf(&notes, "\nCompare: [`%s...%s`](https://github.com/%s/%s/compare/%s...%s)\n",
		previousVersion, newVersion, owner, repo, previousVersion, newVersion)

	// Container image.
	imageTag := fmt.Sprintf("ghcr.io/coder/coder:%s", strings.TrimPrefix(newVersion.String(), "v"))
	_, _ = fmt.Fprintf(&notes, "\n## Container image\n\n- `docker pull %s`\n", imageTag)

	// Install/upgrade links.
	_, _ = fmt.Fprintln(&notes, "\n## Install/upgrade")
	_, _ = fmt.Fprintln(&notes, "\nRefer to our docs to [install](https://coder.com/docs/install) or [upgrade](https://coder.com/docs/install/upgrade) Coder, or use a release asset below.")

	return notes.String(), nil
}
