package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

//nolint:revive // Long function is fine for a sequential release flow.
func runRelease(ctx context.Context, inv *serpent.Invocation, executor ReleaseExecutor, ghAvailable, gpgConfigured, dryRun bool) error {
	w := inv.Stderr

	// --- Release landscape ---
	infof(w, "Checking current releases...")
	allTags, err := allSemverTags()
	if err != nil {
		return xerrors.Errorf("listing tags: %w", err)
	}

	var latestMainline *version
	for _, t := range allTags {
		if t.Pre == "" {
			latestMainline = &t
			break
		}
	}

	stableMinor := -1
	latestStableStr := "(unknown)"
	if latestMainline != nil {
		stableMinor = latestMainline.Minor - 1
		// Find highest tag in the stable minor series.
		for _, t := range allTags {
			if t.Major == latestMainline.Major && t.Minor == stableMinor && t.Pre == "" {
				latestStableStr = t.String()
				break
			}
		}
		if latestStableStr == "(unknown)" {
			latestStableStr = fmt.Sprintf("(none found for v%d.%d.x)", latestMainline.Major, stableMinor)
		}
	}

	fmt.Fprintln(w)
	mainlineStr := "(none)"
	if latestMainline != nil {
		mainlineStr = latestMainline.String()
	}
	fmt.Fprintf(w, "  Latest mainline release: %s\n", pretty.Sprint(cliui.BoldFmt(), mainlineStr))
	fmt.Fprintf(w, "  Latest stable release:   %s\n", pretty.Sprint(cliui.BoldFmt(), latestStableStr))
	fmt.Fprintln(w)

	// --- Branch detection ---
	currentBranch, err := gitOutput("branch", "--show-current")
	if err != nil {
		return xerrors.Errorf("detecting branch: %w", err)
	}

	// Two modes:
	//   1. On "main" — for tagging release candidates (RCs).
	//   2. On "release/X.Y" — for releases and patches.
	// RCs are tagged directly on main to avoid the toil of
	// cherry-picking hundreds of commits onto a release branch.
	// The release/X.Y branch is only cut when the release is
	// ready.
	//
	// Detached HEAD is common: the release manager checks out a
	// specific commit on main before running the tool. We detect
	// this by checking whether HEAD is an ancestor of origin/main.
	branchRe := regexp.MustCompile(`^release/(\d+)\.(\d+)$`)
	onMain := currentBranch == "main"
	var branchMajor, branchMinor int

	// Detached HEAD: currentBranch is empty. Check if HEAD is
	// reachable from origin/main.
	if currentBranch == "" {
		if err := gitRun("merge-base", "--is-ancestor", "HEAD", "origin/main"); err == nil {
			onMain = true
			currentBranch = "main"
			successf(w, "Detached HEAD is an ancestor of main — RC tagging mode.")
		}
	}

	switch {
	case onMain:
		successf(w, "On main branch — RC tagging mode.")
	case branchRe.MatchString(currentBranch):
		m := branchRe.FindStringSubmatch(currentBranch)
		branchMajor, _ = strconv.Atoi(m[1])
		branchMinor, _ = strconv.Atoi(m[2])
		successf(w, "Using release branch: %s", currentBranch)
	default:
		if currentBranch == "" {
			warnf(w, "Detached HEAD is not reachable from origin/main.")
		} else {
			warnf(w, "Current branch %q is not 'main' or a release branch (release/X.Y).", currentBranch)
		}
		branchInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text: "Enter the branch to use (e.g. main, release/2.21)",
			Validate: func(s string) error {
				if s == "main" || branchRe.MatchString(s) {
					return nil
				}
				return xerrors.New("must be 'main' or release/X.Y (e.g. release/2.21)")
			},
		})
		if err != nil {
			return err
		}
		currentBranch = branchInput
		if currentBranch == "main" {
			onMain = true
			successf(w, "On main branch — RC tagging mode.")
		} else {
			m := branchRe.FindStringSubmatch(currentBranch)
			branchMajor, _ = strconv.Atoi(m[1])
			branchMinor, _ = strconv.Atoi(m[2])
			successf(w, "Using release branch: %s", currentBranch)
		}
	}

	// --- Commit selection (RC mode) ---
	// RCs are always tagged at a specific commit. Show the current
	// HEAD and let the user confirm or provide a different SHA.
	// We always checkout the commit so the rest of the flow
	// operates in detached HEAD at the exact commit being tagged.
	if onMain {
		headSHA, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			return xerrors.Errorf("resolving HEAD: %w", err)
		}
		headShort := headSHA[:12]
		headTitle, _ := gitOutput("log", "-1", "--format=%s", "HEAD")
		fmt.Fprintf(w, "  Current commit: %s %s\n", headShort, headTitle)
		fmt.Fprintln(w)

		commitInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text:    "Commit SHA to tag (press Enter to use current)",
			Default: headShort,
		})
		if err != nil {
			return err
		}
		commitInput = strings.TrimSpace(commitInput)

		// Resolve the input to a full SHA.
		targetSHA, err := gitOutput("rev-parse", commitInput)
		if err != nil {
			return xerrors.Errorf("resolving %q: %w", commitInput, err)
		}

		// Always checkout so we're in detached HEAD at the
		// target commit for the rest of the flow.
		if err := gitRun("checkout", "--quiet", targetSHA); err != nil {
			return xerrors.Errorf("checking out %s: %w", commitInput, err)
		}
		if targetSHA != headSHA {
			newTitle, _ := gitOutput("log", "-1", "--format=%s", "HEAD")
			successf(w, "Checked out %s %s", targetSHA[:12], newTitle)
		}
		fmt.Fprintln(w)
	}

	// --- Fetch & sync check ---
	infof(w, "Fetching latest from origin...")
	if err := gitRun("fetch", "--quiet", "--tags", "origin", currentBranch); err != nil {
		return xerrors.Errorf("fetching: %w", err)
	}

	// Skip the local-vs-remote sync check in RC mode because
	// we always checkout a specific commit (detached HEAD).
	if !onMain {
		localHead, err := gitOutput("rev-parse", "HEAD")
		if err != nil {
			return xerrors.Errorf("resolving HEAD: %w", err)
		}
		remoteHead, _ := gitOutput("rev-parse", "origin/"+currentBranch)

		if remoteHead != "" && localHead != remoteHead {
			warnf(w, "Your local branch is not up to date with origin/%s.", currentBranch)
			fmt.Fprintf(w, "  Local:  %s\n", localHead[:12])
			fmt.Fprintf(w, "  Remote: %s\n", remoteHead[:12])
			if err := confirmWithDefault(inv, "Continue anyway?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}
	}

	// --- Find previous version & suggest next ---
	mergedTags, err := mergedSemverTags()
	if err != nil {
		return xerrors.Errorf("listing merged tags: %w", err)
	}

	var prevVersion *version
	var suggested version
	var changelogBaseRef string

	if onMain { //nolint:nestif // Sequential release flow with two distinct modes is inherently nested.
		// On main, suggest the next RC. Find the latest RC tag
		// across all tags, then suggest the next one. If no RC
		// tags exist, suggest rc.0 for the next minor after the
		// latest mainline release.
		var latestRC *version
		for _, t := range allTags {
			if t.IsRC() {
				v := t
				latestRC = &v
				break
			}
		}

		switch {
		case latestRC != nil:
			prevVersion = latestRC
			infof(w, "Latest RC tag: %s", latestRC.String())

			// Check if a final release already exists for this
			// RC's minor series. If so, the series is complete
			// and we should start the next minor's RC cycle.
			seriesComplete := false
			for _, t := range allTags {
				if t.Major == latestRC.Major && t.Minor == latestRC.Minor && t.Pre == "" {
					infof(w, "Final release %s already exists for this series, moving to next minor.", t.String())
					seriesComplete = true
					break
				}
			}

			if seriesComplete {
				suggested = version{
					Major: latestRC.Major,
					Minor: latestRC.Minor + 1,
					Patch: 0,
					Pre:   "rc.0",
				}
			} else {
				suggested = version{
					Major: latestRC.Major,
					Minor: latestRC.Minor,
					Patch: latestRC.Patch,
					Pre:   fmt.Sprintf("rc.%d", latestRC.rcNumber()+1),
				}
			}
		case latestMainline != nil:
			infof(w, "No RC tags found. Latest mainline: %s", latestMainline.String())
			suggested = version{
				Major: latestMainline.Major,
				Minor: latestMainline.Minor + 1,
				Patch: 0,
				Pre:   "rc.0",
			}
		default:
			infof(w, "No previous tags found.")
			suggested = version{Major: 2, Minor: 0, Patch: 0, Pre: "rc.0"}
		}
	} else {
		// On a release branch, find the latest tag matching this
		// branch's major.minor. Without this filter, tags from
		// newer branches reachable via merge history would be
		// picked up incorrectly.
		for _, t := range mergedTags {
			if t.Major == branchMajor && t.Minor == branchMinor {
				v := t
				prevVersion = &v
				break
			}
		}

		// changelogBaseRef is the git ref used as the starting
		// point for release notes. When a tag exists in this
		// minor series we use it directly. For the first release
		// on a new minor no matching tag exists, so we compute
		// the merge-base with the previous minor's release branch
		// instead. This works even when that branch has no tags
		// yet. As a last resort we fall back to the latest
		// reachable tag from a previous minor.
		if prevVersion == nil {
			prevReleaseBranch := fmt.Sprintf("release/%d.%d", branchMajor, branchMinor-1)
			if err := gitRun("fetch", "--quiet", "origin", prevReleaseBranch); err != nil {
				warnf(w, "Could not fetch %s: %v", prevReleaseBranch, err)
			}
			if mb, mbErr := gitOutput("merge-base", "HEAD", "origin/"+prevReleaseBranch); mbErr == nil && mb != "" {
				changelogBaseRef = mb
				infof(w, "Using merge-base with %s as changelog base: %s", prevReleaseBranch, mb[:12])
			} else {
				// No previous release branch; fall back to the
				// latest reachable tag from a previous minor.
				for _, t := range mergedTags {
					if t.Major == branchMajor && t.Minor < branchMinor {
						changelogBaseRef = t.String()
						break
					}
				}
			}
		}

		if prevVersion == nil {
			infof(w, "No previous release tag found on this branch.")
			suggested = version{Major: branchMajor, Minor: branchMinor, Patch: 0}
		} else {
			infof(w, "Previous release tag: %s", prevVersion.String())
			if prevVersion.IsRC() {
				// Branch has only RC tags; suggest the
				// release (same base, no pre-release suffix).
				suggested = version{
					Major: prevVersion.Major,
					Minor: prevVersion.Minor,
					Patch: prevVersion.Patch,
				}
			} else {
				suggested = version{
					Major: prevVersion.Major,
					Minor: prevVersion.Minor,
					Patch: prevVersion.Patch + 1,
				}
			}
		}
	}

	fmt.Fprintln(w)

	// --- Version prompt ---
	versionInput, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:    "Version to release",
		Default: suggested.String(),
		Validate: func(s string) error {
			if _, ok := parseVersion(s); !ok {
				return xerrors.New("must be in format vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N (e.g. v2.31.1 or v2.31.0-rc.0)")
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	newVersion, _ := parseVersion(versionInput)

	// Validate version against branch context.
	switch {
	case onMain && !newVersion.IsRC():
		return xerrors.Errorf("cannot tag a non-RC version (%s) on main; switch to a release/X.Y branch", newVersion)
	case !onMain && newVersion.IsRC():
		return xerrors.Errorf("cannot tag an RC (%s) on a release branch; switch to main", newVersion)
	case !onMain && (newVersion.Major != branchMajor || newVersion.Minor != branchMinor):
		warnf(w, "Version %s does not match branch %s (expected v%d.%d.X).",
			newVersion, currentBranch, branchMajor, branchMinor)
		if err := confirmWithDefault(inv, "Continue anyway?", cliui.ConfirmNo); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w)
	infof(w, "=== Coder Release: %s ===", newVersion)
	fmt.Fprintln(w)

	// --- Check if tag already exists ---
	tagExists := false
	existingTag, _ := gitOutput("tag", "-l", newVersion.String())
	if existingTag != "" {
		tagExists = true
		warnf(w, "Tag '%s' already exists!", newVersion)
		if err := confirmWithDefault(inv, "This will skip tagging. Continue?", cliui.ConfirmNo); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	// --- Check open PRs ---
	// This runs before breaking changes so any last-minute merges
	// are caught by the subsequent checks. Skipped on main since
	// there are always open PRs targeting main.
	if !onMain {
		infof(w, "Checking for open PRs against %s...", currentBranch)
		var openPRs []ghPR
		if ghAvailable {
			openPRs, err = ghListOpenPRs(currentBranch)
			if err != nil {
				warnf(w, "Failed to check open PRs: %v", err)
			}
		} else {
			infof(w, "Skipping (no gh CLI).")
		}

		if len(openPRs) > 0 {
			fmt.Fprintln(w)
			warnf(w, "There are open PRs targeting %s that may need merging first:", currentBranch)
			fmt.Fprintln(w)
			for _, pr := range openPRs {
				fmt.Fprintf(w, "  #%d %s (@%s)\n", pr.Number, pr.Title, pr.Author)
			}
			fmt.Fprintln(w)
			if err := confirmWithDefault(inv, "Continue without merging these?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		} else {
			successf(w, "No open PRs against %s.", currentBranch)
		}
		fmt.Fprintln(w)
	}

	// --- Semver sanity checks ---
	if prevVersion != nil { //nolint:nestif // Sequential release checks are inherently nested.
		// Downgrade check.
		if prevVersion.GreaterThan(newVersion) {
			warnf(w, "Version DOWNGRADE detected: %s → %s.", prevVersion, newVersion)
			if err := confirmWithDefault(inv, "Continue?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}

		// Duplicate check.
		if prevVersion.Equal(newVersion) {
			warnf(w, "Version %s is the SAME as the previous tag %s.", newVersion, prevVersion)
			if err := confirmWithDefault(inv, "Continue?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}

		// Skipped patch check.
		if newVersion.Major == prevVersion.Major && newVersion.Minor == prevVersion.Minor {
			expectedPatch := prevVersion.Patch + 1
			if newVersion.Patch > expectedPatch {
				warnf(w, "Skipping patch version(s): expected v%d.%d.%d, got %s.",
					newVersion.Major, newVersion.Minor, expectedPatch, newVersion)
				if err := confirmWithDefault(inv, "Continue?", cliui.ConfirmNo); err != nil {
					return err
				}
				fmt.Fprintln(w)
			}
		}

		// Breaking changes in patch release check.
		if newVersion.Major == prevVersion.Major && newVersion.Minor == prevVersion.Minor && newVersion.Patch > prevVersion.Patch {
			infof(w, "Checking for breaking changes in patch release...")

			commitRange := prevVersion.String() + "..HEAD"
			commits, err := commitLog(commitRange)
			if err != nil {
				return xerrors.Errorf("reading commit log: %w", err)
			}

			var breakingCommits []commitEntry
			for _, c := range commits {
				if breakingCommitRe.MatchString(c.Title) {
					breakingCommits = append(breakingCommits, c)
				}
			}

			// Check PR labels for release/breaking.
			var breakingPRLabeled []ghPR
			if ghAvailable {
				breakingPRLabeled, err = ghListPRsWithLabel(currentBranch, "release/breaking")
				if err != nil {
					warnf(w, "Failed to check PR labels: %v", err)
				}
			}

			if len(breakingCommits) > 0 || len(breakingPRLabeled) > 0 {
				fmt.Fprintln(w)
				warnf(w, "BREAKING CHANGES detected in a PATCH release — this violates semver!")
				fmt.Fprintln(w)
				if len(breakingCommits) > 0 {
					fmt.Fprintln(w, "  Breaking commits (by conventional commit prefix):")
					for _, c := range breakingCommits {
						fmt.Fprintf(w, "    - %s %s\n", c.SHA, c.Title)
					}
				}
				if len(breakingPRLabeled) > 0 {
					fmt.Fprintln(w, "  PRs labeled release/breaking:")
					for _, pr := range breakingPRLabeled {
						fmt.Fprintf(w, "    - #%d %s\n", pr.Number, pr.Title)
					}
				}
				fmt.Fprintln(w)
				if err := confirmWithDefault(inv, "Continue with patch release despite breaking changes?", cliui.ConfirmNo); err != nil {
					return err
				}
				fmt.Fprintln(w)
			} else {
				successf(w, "No breaking changes detected.")
			}
		}
	}

	// --- Channel selection ---
	// This is done before release notes generation because the
	// notes format differs between mainline and stable channels.
	// RC releases are always on the "rc" channel and skip the
	// stable/mainline prompt.
	channel := "mainline"
	if newVersion.IsRC() {
		channel = "rc"
		infof(w, "Channel: rc (release candidate, will be marked as prerelease on GitHub).")
	} else {
		channelDefault := cliui.ConfirmNo
		channelHint := ""
		if newVersion.Minor == stableMinor {
			channelDefault = cliui.ConfirmYes
			channelHint = " (this looks like a stable release)"
		}

		_, err = cliui.Prompt(inv, cliui.PromptOptions{
			Text:      fmt.Sprintf("Mark this as the latest stable release on GitHub?%s", channelHint),
			Default:   channelDefault,
			IsConfirm: true,
		})
		if err == nil {
			channel = "stable"
		} else if !errors.Is(err, cliui.ErrCanceled) {
			return err
		}

		if channel == "stable" {
			infof(w, "Channel: stable (will be marked as GitHub Latest).")
		} else {
			infof(w, "Channel: mainline (will be marked as prerelease).")
		}
	}
	fmt.Fprintln(w)

	// --- Generate release notes ---
	infof(w, "Generating release notes...")

	var commitRange string
	switch {
	case prevVersion != nil:
		commitRange = prevVersion.String() + "..HEAD"
	case changelogBaseRef != "":
		commitRange = changelogBaseRef + "..HEAD"
	default:
		commitRange = "HEAD"
	}

	commits, err := commitLog(commitRange)
	if err != nil {
		return xerrors.Errorf("reading commit log: %w", err)
	}

	// Build PR metadata maps (by SHA and PR number) via gh CLI.
	var prMeta *prMetadataMaps
	if ghAvailable {
		prMeta, err = ghBuildPRMetadataMap(commits)
		if err != nil {
			warnf(w, "Failed to fetch PR metadata: %v", err)
		}
	}
	if prMeta == nil {
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
		// Skip dependabot commits.
		if meta.Author == "dependabot" || meta.Author == "app/dependabot" {
			continue
		}
		cat := categorizeCommit(c.Title, meta.Labels)
		humanTitle := humanizeTitle(c.Title)
		// Strip trailing PR ref from humanized title if present,
		// so we can rebuild it with the SHA appended.
		humanTitle = prNumRe.ReplaceAllString(humanTitle, "")
		humanTitle = strings.TrimSpace(humanTitle)
		// Build entry: - Title (#PR, SHA) (@author)
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

	// Build release notes markdown matching the format from
	// scripts/release/generate_release_notes.sh.
	var notes strings.Builder

	// Stable since header, mainline blurb, or RC advisory.
	if channel == "stable" {
		fmt.Fprintf(&notes, "> ## Stable (since %s)\n\n", time.Now().Format("January 02, 2006"))
	}
	fmt.Fprintln(&notes, "## Changelog")
	switch channel {
	case "rc":
		fmt.Fprintln(&notes)
		fmt.Fprintln(&notes, "> [!NOTE]")
		fmt.Fprintln(&notes, "> This is a **release candidate** (RC) for testing purposes. It is not recommended for production use. Please report any issues you encounter. Learn more about our [Release Schedule](https://coder.com/docs/install/releases).")
	case "mainline":
		fmt.Fprintln(&notes)
		fmt.Fprintln(&notes, "> [!NOTE]")
		fmt.Fprintln(&notes, "> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/install/releases).")
	}

	hasContent := false
	for _, s := range sections {
		if entries, ok := sectionCommits[s.Key]; ok && len(entries) > 0 {
			fmt.Fprintf(&notes, "\n### %s\n\n", s.Title)
			if s.Key == "experimental" {
				fmt.Fprintln(&notes, "These changes are feature-flagged and can be enabled with the `--experiments` server flag. They may change or be removed in future releases.")
				fmt.Fprintln(&notes)
			}
			for _, e := range entries {
				fmt.Fprintln(&notes, e)
			}
			hasContent = true
		}
	}
	if !hasContent {
		prevStr := "the beginning of time"
		if prevVersion != nil {
			prevStr = prevVersion.String()
		}
		fmt.Fprintf(&notes, "\n_No changes since %s._\n", prevStr)
	}

	// Compare link.
	compareBase := changelogBaseRef
	if prevVersion != nil {
		compareBase = prevVersion.String()
	}
	if compareBase != "" {
		fmt.Fprintf(&notes, "\nCompare: [`%s...%s`](https://github.com/%s/%s/compare/%s...%s)\n",
			compareBase, newVersion, owner, repo, compareBase, newVersion)
	}

	// Container image.
	imageTag := fmt.Sprintf("ghcr.io/coder/coder:%s", strings.TrimPrefix(newVersion.String(), "v"))
	fmt.Fprintf(&notes, "\n## Container image\n\n- `docker pull %s`\n", imageTag)

	// Install/upgrade links.
	fmt.Fprintln(&notes, "\n## Install/upgrade")
	fmt.Fprintln(&notes, "\nRefer to our docs to [install](https://coder.com/docs/install) or [upgrade](https://coder.com/docs/install/upgrade) Coder, or use a release asset below.")

	releaseNotes := notes.String()

	// Write to file.
	releaseNotesFile := fmt.Sprintf("build/RELEASE-%s.md", newVersion)
	if err := os.MkdirAll("build", 0o755); err != nil {
		return xerrors.Errorf("creating build directory: %w", err)
	}
	if err := os.WriteFile(releaseNotesFile, []byte(releaseNotes), 0o600); err != nil {
		return xerrors.Errorf("writing release notes: %w", err)
	}

	// --- Preview ---
	fmt.Fprintln(w)
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "--- Release Notes Preview ---"))
	fmt.Fprintln(w)
	fmt.Fprint(w, releaseNotes)
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "--- End Preview ---"))
	fmt.Fprintln(w)
	infof(w, "Release notes written to %s", releaseNotesFile)
	fmt.Fprintln(w)

	// --- Offer to edit ---
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("GIT_EDITOR")
	}
	if editor != "" {
		if err := confirmWithDefault(inv, fmt.Sprintf("Edit release notes in %s?", editor), cliui.ConfirmNo); err == nil {
			cmd := exec.Command(editor, releaseNotesFile)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return xerrors.Errorf("editor: %w", err)
			}
			updated, err := os.ReadFile(releaseNotesFile)
			if err != nil {
				return xerrors.Errorf("reading edited release notes: %w", err)
			}
			// The file will be re-read from disk before the
			// workflow trigger step.
			_ = string(updated)
			infof(w, "Release notes updated.")
		}
		fmt.Fprintln(w)
	}

	// --- Tag ---
	ref, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return xerrors.Errorf("resolving HEAD: %w", err)
	}
	shortRef := ref[:12]

	if !tagExists {
		fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Next step: create an annotated tag."))
		fmt.Fprintf(w, "  Tag:    %s\n", newVersion)
		fmt.Fprintf(w, "  Commit: %s\n", shortRef)
		fmt.Fprintf(w, "  Branch: %s\n", currentBranch)
		fmt.Fprintln(w)
		if err := confirm(inv, "Create tag?"); err != nil {
			return xerrors.New("cannot proceed without a tag")
		}
		if err := executor.CreateTag(ctx, newVersion.String(), ref, "Release "+newVersion.String(), gpgConfigured); err != nil {
			return xerrors.Errorf("creating tag: %w", err)
		}
		successf(w, "Tag %s created.", newVersion)
		fmt.Fprintln(w)
	} else {
		infof(w, "Tag %s already exists, skipping creation.", newVersion)
		fmt.Fprintln(w)
	}

	// --- Push tag ---
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), fmt.Sprintf("Next step: push tag '%s' to origin.", newVersion)))
	fmt.Fprintf(w, "  This will run: git push origin %s\n", newVersion)
	fmt.Fprintln(w)
	if err := confirm(inv, "Push tag?"); err != nil {
		return xerrors.New("cannot trigger release without pushing the tag")
	}
	if err := executor.PushTag(ctx, newVersion.String()); err != nil {
		return xerrors.Errorf("pushing tag: %w", err)
	}
	successf(w, "Tag pushed.")
	fmt.Fprintln(w)

	// --- Trigger release workflow ---
	// Re-read release notes from disk in case the user edited the
	// file externally between the editor step and now.
	freshNotes, err := os.ReadFile(releaseNotesFile)
	if err != nil {
		return xerrors.Errorf("re-reading release notes: %w", err)
	}
	releaseNotes = string(freshNotes)

	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Next step: trigger the 'release.yaml' GitHub Actions workflow."))
	fmt.Fprintf(w, "  Workflow: release.yaml\n")
	fmt.Fprintf(w, "  Repo:    %s/%s\n", owner, repo)
	fmt.Fprintf(w, "  Ref:     %s\n", newVersion)
	fmt.Fprintln(w)
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "  Payload fields:"))
	fmt.Fprintf(w, "    release_channel: %s\n", channel)
	fmt.Fprintf(w, "    dry_run:         false\n")
	fmt.Fprintln(w)
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "  release_notes:"))
	for _, line := range strings.Split(releaseNotes, "\n") {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	if err := confirm(inv, "Trigger release workflow?"); err != nil {
		infof(w, "Skipped workflow trigger. You can trigger it manually from GitHub Actions.")
		fmt.Fprintln(w)
		successf(w, "Done! 🎉")
		return nil
	}
	if err := executor.TriggerWorkflow(ctx, newVersion.String(), channel, releaseNotes); err != nil {
		return xerrors.Errorf("triggering workflow: %w", err)
	}
	successf(w, "Release workflow triggered!")

	// --- Update release docs ---
	// RC releases skip docs updates (calendar, helm versions, etc.)
	// since they are not production releases.
	if newVersion.IsRC() {
		infof(w, "Skipping docs update for release candidate.")
	} else {
		promptAndUpdateDocs(inv, newVersion, channel, dryRun)
	}

	fmt.Fprintln(w)
	successf(w, "Done! 🎉")
	return nil
}
