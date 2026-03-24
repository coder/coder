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

	// Use only stable (non-prerelease) tags for the landscape
	// display so RC tags don't skew the "latest" detection.
	stableTags := filterStable(allTags)

	var latestMainline *version
	if len(stableTags) > 0 {
		v := stableTags[0]
		latestMainline = &v
	}

	stableMinor := -1
	latestStableStr := "(unknown)"
	if latestMainline != nil {
		stableMinor = latestMainline.Minor - 1
		// Find highest tag in the stable minor series.
		for _, t := range stableTags {
			if t.Major == latestMainline.Major && t.Minor == stableMinor {
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

	// --- Release type prompt ---
	isRC := false
	_, err = cliui.Prompt(inv, cliui.PromptOptions{
		Text:      "Is this a release candidate (RC)?",
		Default:   cliui.ConfirmNo,
		IsConfirm: true,
	})
	if err == nil {
		isRC = true
	} else if !errors.Is(err, cliui.ErrCanceled) {
		return err
	}
	fmt.Fprintln(w)

	// rcRef holds the commit SHA to tag for RC releases. For
	// standard releases the tag target is resolved later from HEAD.
	var rcRef string

	var currentBranch string
	var branchMajor, branchMinor int
	var prevVersion *version
	var newVersion version
	var tagExists bool
	channel := "mainline"

	if isRC {
		// --- RC flow: commit selection & version ---
		infof(w, "Fetching tags from origin...")
		if err := gitRun("fetch", "--quiet", "--tags", "origin"); err != nil {
			return xerrors.Errorf("fetching tags: %w", err)
		}

		// Ask for the commit to tag.
		commitInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text: "Enter commit SHA to tag",
			Validate: func(s string) error {
				if strings.TrimSpace(s) == "" {
					return xerrors.New("a commit SHA is required")
				}
				_, err := gitOutput("rev-parse", "--verify", strings.TrimSpace(s)+"^{commit}")
				if err != nil {
					return xerrors.Errorf("could not resolve %q to a commit", s)
				}
				return nil
			},
		})
		if err != nil {
			return err
		}

		rcRef, err = gitOutput("rev-parse", strings.TrimSpace(commitInput))
		if err != nil {
			return xerrors.Errorf("resolving commit: %w", err)
		}

		// Display commit details and confirm.
		subject, _ := gitOutput("log", "-1", "--format=%s", rcRef)
		author, _ := gitOutput("log", "-1", "--format=%an", rcRef)
		date, _ := gitOutput("log", "-1", "--format=%ci", rcRef)

		fmt.Fprintln(w)
		fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Will tag commit:"))
		fmt.Fprintf(w, "  SHA:     %s\n", rcRef[:12])
		fmt.Fprintf(w, "  Subject: %s\n", subject)
		fmt.Fprintf(w, "  Author:  %s\n", author)
		fmt.Fprintf(w, "  Date:    %s\n", date)
		fmt.Fprintln(w)
		if err := confirm(inv, "Continue?"); err != nil {
			return xerrors.New("aborted")
		}
		fmt.Fprintln(w)

		// Suggest the next minor version based on the latest
		// mainline release. For example, if the latest is v2.31.5
		// the suggested target is v2.32.0.
		var suggestedTarget string
		if latestMainline != nil {
			suggestedTarget = version{
				Major: latestMainline.Major,
				Minor: latestMainline.Minor + 1,
				Patch: 0,
			}.String()
		}

		// Ask for the target version (e.g. v2.32.0).
		targetInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text:    "Target release version (e.g. v2.32.0)",
			Default: suggestedTarget,
			Validate: func(s string) error {
				v, ok := parseVersion(s)
				if !ok {
					return xerrors.New("must be in format vMAJOR.MINOR.PATCH (e.g. v2.32.0)")
				}
				if v.IsPrerelease() {
					return xerrors.New("enter the base version without a pre-release suffix")
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
		targetVersion, _ := parseVersion(targetInput)

		// Auto-detect the next RC number by scanning existing tags.
		nextRC := 0
		for _, t := range allTags {
			if t.baseEqual(targetVersion) && t.rcNumber() >= nextRC {
				nextRC = t.rcNumber() + 1
			}
		}
		suggestedRC := version{
			Major: targetVersion.Major,
			Minor: targetVersion.Minor,
			Patch: targetVersion.Patch,
			Pre:   fmt.Sprintf("rc.%d", nextRC),
		}

		// Find previous RC or stable tag for release notes range.
		// Prefer the last RC for this target, falling back to the
		// latest stable tag.
		for i := range allTags {
			t := allTags[i]
			if t.baseEqual(targetVersion) && t.IsPrerelease() {
				prevVersion = &t
				break
			}
		}
		if prevVersion == nil {
			for i := range stableTags {
				t := stableTags[i]
				if !t.GreaterThan(targetVersion) {
					prevVersion = &t
					break
				}
			}
		}

		versionInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text:    "Version to release",
			Default: suggestedRC.String(),
			Validate: func(s string) error {
				v, ok := parseVersion(s)
				if !ok {
					return xerrors.New("must be in format vMAJOR.MINOR.PATCH-rc.N (e.g. v2.32.0-rc.0)")
				}
				if !v.IsPrerelease() {
					return xerrors.New("RC version must include a pre-release suffix (e.g. -rc.0)")
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
		newVersion, _ = parseVersion(versionInput)

		fmt.Fprintln(w)
		infof(w, "=== Coder Release Candidate: %s ===", newVersion)
		fmt.Fprintln(w)

		// Check if tag already exists.
		existingTag, _ := gitOutput("tag", "-l", newVersion.String())
		if existingTag != "" {
			tagExists = true
			warnf(w, "Tag '%s' already exists!", newVersion)
			if err := confirmWithDefault(inv, "This will skip tagging. Continue?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}

		// RC is always mainline (prerelease on GitHub).
		infof(w, "Channel: mainline (RC is always marked as prerelease).")
		fmt.Fprintln(w)
	} else {
		// --- Standard release flow ---

		// --- Branch detection ---
		currentBranch, err = gitOutput("branch", "--show-current")
		if err != nil {
			return xerrors.Errorf("detecting branch: %w", err)
		}

		branchRe := regexp.MustCompile(`^release/(\d+)\.(\d+)$`)
		m := branchRe.FindStringSubmatch(currentBranch)
		if m == nil {
			warnf(w, "Current branch %q is not a release branch (release/X.Y).", currentBranch)
			branchInput, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text: "Enter the release branch to use (e.g. release/2.21)",
				Validate: func(s string) error {
					if !branchRe.MatchString(s) {
						return xerrors.New("must be in format release/X.Y (e.g. release/2.21)")
					}
					return nil
				},
			})
			if err != nil {
				return err
			}
			currentBranch = branchInput
			m = branchRe.FindStringSubmatch(currentBranch)
		}
		branchMajor, _ = strconv.Atoi(m[1])
		branchMinor, _ = strconv.Atoi(m[2])
		successf(w, "Using release branch: %s", currentBranch)

		// --- Fetch & sync check ---
		infof(w, "Fetching latest from origin...")
		if err := gitRun("fetch", "--quiet", "--tags", "origin", currentBranch); err != nil {
			return xerrors.Errorf("fetching: %w", err)
		}

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

		// --- Find previous version & suggest next ---
		mergedTags, err := mergedSemverTags()
		if err != nil {
			return xerrors.Errorf("listing merged tags: %w", err)
		}

		// Find the latest stable tag matching this branch's
		// major.minor. Without this filter, tags from newer
		// branches that are reachable via merge history would be
		// picked up incorrectly.
		for _, t := range mergedTags {
			if t.Major == branchMajor && t.Minor == branchMinor && !t.IsPrerelease() {
				v := t
				prevVersion = &v
				break
			}
		}

		var suggested version
		if prevVersion == nil {
			infof(w, "No previous release tag found on this branch.")
			suggested = version{Major: branchMajor, Minor: branchMinor, Patch: 0}
		} else {
			infof(w, "Previous release tag: %s", prevVersion.String())
			suggested = version{Major: prevVersion.Major, Minor: prevVersion.Minor, Patch: prevVersion.Patch + 1}
		}

		fmt.Fprintln(w)

		// --- Version prompt ---
		versionInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text:    "Version to release",
			Default: suggested.String(),
			Validate: func(s string) error {
				v, ok := parseVersion(s)
				if !ok {
					return xerrors.New("must be in format vMAJOR.MINOR.PATCH (e.g. v2.31.1)")
				}
				if v.IsPrerelease() {
					return xerrors.New("use RC mode for pre-release versions")
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
		newVersion, _ = parseVersion(versionInput)

		// Warn if version doesn't match branch.
		if newVersion.Major != branchMajor || newVersion.Minor != branchMinor {
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
		// This runs before breaking changes so any last-minute
		// merges are caught by the subsequent checks.
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
		fmt.Fprintln(w)
	}

	// --- Generate release notes ---
	infof(w, "Generating release notes...")

	// For RC releases, use the chosen commit ref; for standard
	// releases, use HEAD.
	notesRef := "HEAD"
	if rcRef != "" {
		notesRef = rcRef
	}

	commitRange := notesRef
	if prevVersion != nil {
		commitRange = prevVersion.String() + ".." + notesRef
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

	// Stable since header or mainline blurb.
	if channel == "stable" {
		fmt.Fprintf(&notes, "> ## Stable (since %s)\n\n", time.Now().Format("January 02, 2006"))
	}
	if isRC {
		fmt.Fprintf(&notes, "## [RC] Changelog\n")
		fmt.Fprintln(&notes)
		fmt.Fprintln(&notes, "> [!NOTE]")
		fmt.Fprintln(&notes, "> This is a **release candidate**. It is not intended for production use. Please test and report issues.")
	} else {
		fmt.Fprintln(&notes, "## Changelog")
	}
	if channel == "mainline" && !isRC {
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
	if prevVersion != nil {
		fmt.Fprintf(&notes, "\nCompare: [`%s...%s`](https://github.com/%s/%s/compare/%s...%s)\n",
			prevVersion, newVersion, owner, repo, prevVersion, newVersion)
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
	ref := rcRef
	if ref == "" {
		ref, err = gitOutput("rev-parse", "HEAD")
		if err != nil {
			return xerrors.Errorf("resolving HEAD: %w", err)
		}
	}
	shortRef := ref[:12]

	if !tagExists {
		fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Next step: create an annotated tag."))
		fmt.Fprintf(w, "  Tag:    %s\n", newVersion)
		fmt.Fprintf(w, "  Commit: %s\n", shortRef)
		if currentBranch != "" {
			fmt.Fprintf(w, "  Branch: %s\n", currentBranch)
		}
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
	// Skip doc updates for RC releases — no release calendar or
	// autoversion changes are needed for pre-releases.
	if !isRC {
		promptAndUpdateDocs(inv, newVersion, channel, dryRun)
	}

	fmt.Fprintln(w)
	successf(w, "Done! 🎉")
	return nil
}
