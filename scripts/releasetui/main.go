package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

// version holds a parsed semver version.
type version struct {
	Major, Minor, Patch int
}

var semverRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

func parseVersion(s string) (version, bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return version{Major: maj, Minor: min, Patch: pat}, true
}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (a version) GT(b version) bool {
	if a.Major != b.Major {
		return a.Major > b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	return a.Patch > b.Patch
}

func (a version) Eq(b version) bool {
	return a.Major == b.Major && a.Minor == b.Minor && a.Patch == b.Patch
}

// ReleaseExecutor handles dangerous write/mutating operations
// that should be skipped in dry-run mode. Only actions that
// modify the git repo or trigger external side effects belong
// here. Safe operations (file writes, fetches, editor) are
// called directly.
type ReleaseExecutor interface {
	// CreateTag creates an annotated tag at the given ref. If sign
	// is true, the tag is GPG-signed.
	CreateTag(ctx context.Context, tag, ref, message string, sign bool) error
	// PushTag pushes a tag to origin.
	PushTag(ctx context.Context, tag string) error
	// TriggerWorkflow dispatches the release.yaml workflow.
	TriggerWorkflow(ctx context.Context, ref, channel, releaseNotes string) error
}

// liveExecutor performs real operations.
type liveExecutor struct{}

func (e *liveExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	args := []string{"tag", "-a"}
	if sign {
		args = append(args, "-s")
	}
	args = append(args, tag, "-m", message, ref)
	return gitRun(args...)
}

func (e *liveExecutor) PushTag(_ context.Context, tag string) error {
	return gitRun("push", "origin", tag)
}

func (e *liveExecutor) TriggerWorkflow(_ context.Context, ref, channel, releaseNotes string) error {
	payload := map[string]string{
		"dry_run":         "false",
		"release_channel": channel,
		"release_notes":   releaseNotes,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling workflow payload: %w", err)
	}
	cmd := exec.Command("gh", "workflow", "run", "release.yaml",
		"--repo", owner+"/"+repo,
		"--ref", ref,
		"--json",
	)
	cmd.Stdin = strings.NewReader(string(payloadJSON))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dryRunExecutor prints what would happen without doing it.
type dryRunExecutor struct {
	w io.Writer
}

func (e *dryRunExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	signFlag := ""
	if sign {
		signFlag = "-s "
	}
	fmt.Fprintf(e.w, "[DRYRUN] would run: git tag %s-a %s -m %q %s\n", signFlag, tag, message, ref)
	return nil
}

func (e *dryRunExecutor) PushTag(_ context.Context, tag string) error {
	fmt.Fprintf(e.w, "[DRYRUN] would run: git push origin %s\n", tag)
	return nil
}

func (e *dryRunExecutor) TriggerWorkflow(_ context.Context, ref, channel, _ string) error {
	fmt.Fprintf(e.w, "[DRYRUN] would trigger release.yaml workflow (ref=%s, channel=%s)\n", ref, channel)
	return nil
}

// gitOutput runs a read-only git command and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitRun runs a git command with stdout/stderr connected to the
// terminal.
func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ghOutput runs a gh CLI command and returns trimmed stdout.
func ghOutput(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// checkGHAuth verifies that the gh CLI is installed and
// authenticated. Returns true if gh is available.
func checkGHAuth() bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	err := exec.Command("gh", "auth", "status", "--hostname", "github.com").Run()
	return err == nil
}

// confirm asks a yes/no question. Returns nil if the user confirms,
// or a cancellation error otherwise.
func confirm(inv *serpent.Invocation, msg string) error {
	_, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:      msg,
		Default:   cliui.ConfirmYes,
		IsConfirm: true,
	})
	return err
}

// confirmWithDefault asks a yes/no question with the specified
// default ("yes" or "no").
func confirmWithDefault(inv *serpent.Invocation, msg, def string) error {
	_, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:      msg,
		Default:   def,
		IsConfirm: true,
	})
	return err
}

// outputPrefix is prepended to every message line. Set to
// "[DRYRUN] " when running in dry-run mode.
var outputPrefix string

// warnf prints a yellow warning to stderr.
func warnf(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.Warn, outputPrefix+"⚠️  WARNING: "+format+"\n", args...)
}

// infof prints a cyan info message to stderr.
func infof(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.DateTimeStamp, outputPrefix+format+"\n", args...)
}

// successf prints a green success message to stderr.
func successf(w io.Writer, format string, args ...any) {
	pretty.Fprintf(w, cliui.DefaultStyles.Keyword, outputPrefix+"✓ "+format+"\n", args...)
}

func main() {
	var dryRun bool
	cmd := &serpent.Command{
		Use:   "releasetui",
		Short: "Interactive release tagging for coder/coder.",
		Long:  "Run this from a release branch (release/X.Y). The tool detects the branch, infers the next version, and walks you through tagging, pushing, and triggering the release workflow.",
		Options: serpent.OptionSet{
			{
				Name:        "dry-run",
				Flag:        "dry-run",
				Description: "Print write commands instead of executing them.",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			w := inv.Stderr

			// --- Check dependencies ---
			if _, err := exec.LookPath("git"); err != nil {
				return fmt.Errorf("git is required but not found in PATH")
			}

			// --- Check GPG signing ---
			signingKey, _ := gitOutput("config", "--get", "user.signingkey")
			gpgFormat, _ := gitOutput("config", "--get", "gpg.format")
			gpgConfigured := signingKey != "" || gpgFormat != ""
			if !gpgConfigured {
				warnf(w, "GPG signing is not configured. Tags will be unsigned — there will be no way to verify who pushed the tag.")
				fmt.Fprintf(w, "  To fix: set git config user.signingkey or gpg.format\n")
				if err := confirmWithDefault(inv, "Continue without signing?", cliui.ConfirmNo); err != nil {
					return err
				}
				fmt.Fprintln(w)
			}

			// --- Check gh CLI auth ---
			ghAvailable := checkGHAuth()
			if !ghAvailable {
				warnf(w, "gh CLI is not available or not authenticated.")
				infof(w, "Continuing without GitHub features (PR checks, label lookups, workflow trigger).")
				fmt.Fprintln(w)
			}

			// --- Wire up executor ---
			var executor ReleaseExecutor
			if dryRun {
				outputPrefix = "[DRYRUN] "
				executor = &dryRunExecutor{w: w}
			} else {
				executor = &liveExecutor{}
			}

			return runRelease(ctx, inv, executor, ghAvailable, gpgConfigured)
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.ErrCanceled) {
			os.Exit(1)
		}
		// Unwrap serpent's "running command ..." wrapper to
		// keep output clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		pretty.Fprintf(os.Stderr, cliui.DefaultStyles.Error, "Error: %s\n", err)
		os.Exit(1)
	}
}

// allSemverTags returns all semver tags sorted descending.
func allSemverTags() ([]version, error) {
	out, err := gitOutput("tag", "--sort=-v:refname")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var tags []version
	for _, line := range strings.Split(out, "\n") {
		if v, ok := parseVersion(strings.TrimSpace(line)); ok {
			tags = append(tags, v)
		}
	}
	return tags, nil
}

// mergedSemverTags returns semver tags reachable from HEAD, sorted
// descending.
func mergedSemverTags() ([]version, error) {
	out, err := gitOutput("tag", "--merged", "HEAD", "--sort=-v:refname")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var tags []version
	for _, line := range strings.Split(out, "\n") {
		if v, ok := parseVersion(strings.TrimSpace(line)); ok {
			tags = append(tags, v)
		}
	}
	return tags, nil
}

// commitEntry represents a single non-merge commit.
type commitEntry struct {
	SHA   string
	Title string
	PRNum int // 0 if no PR number found
}

var prNumRe = regexp.MustCompile(`\(#(\d+)\)`)

// commitLog returns non-merge commits in the given range.
func commitLog(commitRange string) ([]commitEntry, error) {
	out, err := gitOutput("log", "--no-merges", "--pretty=format:%h %s", commitRange)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var entries []commitEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		spaceIdx := strings.IndexByte(line, ' ')
		if spaceIdx < 0 {
			continue
		}
		e := commitEntry{
			SHA:   line[:spaceIdx],
			Title: line[spaceIdx+1:],
		}
		if m := prNumRe.FindStringSubmatch(e.Title); m != nil {
			e.PRNum, _ = strconv.Atoi(m[1])
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// breakingCommitRe matches conventional commit "!:" breaking changes.
var breakingCommitRe = regexp.MustCompile(`^[a-zA-Z]+(\(.+\))?!:`)

// categorizeCommit determines the release note section for a commit.
func categorizeCommit(title string, labels []string) string {
	// Label-based categorization takes priority.
	for _, l := range labels {
		if l == "release/breaking" {
			return "breaking"
		}
		if l == "security" {
			return "security"
		}
	}
	if breakingCommitRe.MatchString(title) {
		return "breaking"
	}
	switch {
	case strings.HasPrefix(title, "feat"):
		return "feat"
	case strings.HasPrefix(title, "fix"):
		return "fix"
	case strings.HasPrefix(title, "docs"):
		return "docs"
	case strings.HasPrefix(title, "refactor"):
		return "refactor"
	default:
		return "other"
	}
}

//nolint:revive // Long function is fine for a sequential release flow.
func runRelease(ctx context.Context, inv *serpent.Invocation, executor ReleaseExecutor, ghAvailable, gpgConfigured bool) error {
	w := inv.Stderr

	// --- Release landscape ---
	infof(w, "Checking current releases...")
	allTags, err := allSemverTags()
	if err != nil {
		return fmt.Errorf("listing tags: %w", err)
	}

	var latestMainline *version
	if len(allTags) > 0 {
		v := allTags[0]
		latestMainline = &v
	}

	stableMinor := -1
	latestStableStr := "(unknown)"
	if latestMainline != nil {
		stableMinor = latestMainline.Minor - 1
		// Find highest tag in the stable minor series.
		for _, t := range allTags {
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

	// --- Branch detection ---
	currentBranch, err := gitOutput("branch", "--show-current")
	if err != nil {
		return fmt.Errorf("detecting branch: %w", err)
	}

	branchRe := regexp.MustCompile(`^release/(\d+)\.(\d+)$`)
	m := branchRe.FindStringSubmatch(currentBranch)
	if m == nil {
		warnf(w, "Current branch %q is not a release branch (release/X.Y).", currentBranch)
		branchInput, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text: "Enter the release branch to use (e.g. release/2.21)",
			Validate: func(s string) error {
				if !branchRe.MatchString(s) {
					return fmt.Errorf("must be in format release/X.Y (e.g. release/2.21)")
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
	branchMajor, _ := strconv.Atoi(m[1])
	branchMinor, _ := strconv.Atoi(m[2])
	successf(w, "Using release branch: %s", currentBranch)

	// --- Fetch & sync check ---
	infof(w, "Fetching latest from origin...")
	if err := gitRun("fetch", "--quiet", "--tags", "origin", currentBranch); err != nil {
		return fmt.Errorf("fetching: %w", err)
	}

	localHead, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
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
		return fmt.Errorf("listing merged tags: %w", err)
	}

	var prevVersion *version
	for _, t := range mergedTags {
		v := t
		prevVersion = &v
		break
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
			if _, ok := parseVersion(s); !ok {
				return fmt.Errorf("must be in format vMAJOR.MINOR.PATCH (e.g. v2.31.1)")
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	newVersion, _ := parseVersion(versionInput)

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

	// --- Semver sanity checks ---
	if prevVersion != nil {
		// Downgrade check.
		if prevVersion.GT(newVersion) {
			warnf(w, "Version DOWNGRADE detected: %s → %s.", prevVersion, newVersion)
			if err := confirmWithDefault(inv, "Continue?", cliui.ConfirmNo); err != nil {
				return err
			}
			fmt.Fprintln(w)
		}

		// Duplicate check.
		if prevVersion.Eq(newVersion) {
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
				return fmt.Errorf("reading commit log: %w", err)
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

	// --- Check open PRs ---
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

	// --- Generate release notes ---
	infof(w, "Generating release notes...")

	commitRange := "HEAD"
	if prevVersion != nil {
		commitRange = prevVersion.String() + "..HEAD"
	}

	commits, err := commitLog(commitRange)
	if err != nil {
		return fmt.Errorf("reading commit log: %w", err)
	}

	// Build PR number → labels map via gh CLI.
	var prLabels map[int][]string
	if ghAvailable {
		prLabels, err = ghBuildPRLabelMap(currentBranch)
		if err != nil {
			warnf(w, "Failed to fetch PR labels: %v", err)
		}
	}
	if prLabels == nil {
		prLabels = make(map[int][]string)
	}

	type section struct {
		Key   string
		Title string
	}
	sections := []section{
		{"breaking", "⚠️ BREAKING CHANGES"},
		{"security", "🔒 Security"},
		{"feat", "✨ Features"},
		{"fix", "🐛 Bug Fixes"},
		{"docs", "📖 Documentation"},
		{"refactor", "♻️ Refactor"},
		{"other", "📦 Other Changes"},
	}
	sectionCommits := make(map[string][]string)

	for _, c := range commits {
		labels := prLabels[c.PRNum]
		cat := categorizeCommit(c.Title, labels)
		entry := fmt.Sprintf("- %s (%s)", c.Title, c.SHA)
		sectionCommits[cat] = append(sectionCommits[cat], entry)
	}

	// Build release notes markdown.
	var notes strings.Builder
	fmt.Fprintf(&notes, "## %s\n\n", newVersion)
	if prevVersion != nil {
		fmt.Fprintf(&notes, "Compare: https://github.com/%s/%s/compare/%s...%s\n\n", owner, repo, prevVersion, newVersion)
	}

	hasContent := false
	for _, s := range sections {
		if entries, ok := sectionCommits[s.Key]; ok && len(entries) > 0 {
			fmt.Fprintf(&notes, "### %s\n\n", s.Title)
			for _, e := range entries {
				fmt.Fprintln(&notes, e)
			}
			fmt.Fprintln(&notes)
			hasContent = true
		}
	}
	if !hasContent {
		prevStr := "the beginning of time"
		if prevVersion != nil {
			prevStr = prevVersion.String()
		}
		fmt.Fprintf(&notes, "_No changes since %s._\n", prevStr)
	}

	releaseNotes := notes.String()

	// Write to file.
	releaseNotesFile := fmt.Sprintf("build/RELEASE-%s.md", newVersion)
	if err := os.MkdirAll("build", 0o755); err != nil {
		return fmt.Errorf("creating build directory: %w", err)
	}
	if err := os.WriteFile(releaseNotesFile, []byte(releaseNotes), 0o644); err != nil {
		return fmt.Errorf("writing release notes: %w", err)
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
				return fmt.Errorf("editor: %w", err)
			}
			updated, err := os.ReadFile(releaseNotesFile)
			if err != nil {
				return fmt.Errorf("reading edited release notes: %w", err)
			}
			releaseNotes = string(updated)
			infof(w, "Release notes updated.")
		}
		fmt.Fprintln(w)
	}

	// --- Channel selection ---
	channelDefault := cliui.ConfirmNo
	channelHint := ""
	if newVersion.Minor == stableMinor {
		channelDefault = cliui.ConfirmYes
		channelHint = " (this looks like a stable release)"
	}

	channel := "mainline"
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

	// --- Tag ---
	ref, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
	}
	shortRef := ref[:12]

	if !tagExists {
		fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Next step: create an annotated tag."))
		fmt.Fprintf(w, "  Tag:    %s\n", newVersion)
		fmt.Fprintf(w, "  Commit: %s\n", shortRef)
		fmt.Fprintf(w, "  Branch: %s\n", currentBranch)
		fmt.Fprintln(w)
		if err := confirm(inv, "Create tag?"); err != nil {
			return fmt.Errorf("cannot proceed without a tag")
		}
		if err := executor.CreateTag(ctx, newVersion.String(), ref, "Release "+newVersion.String(), gpgConfigured); err != nil {
			return fmt.Errorf("creating tag: %w", err)
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
		return fmt.Errorf("cannot trigger release without pushing the tag")
	}
	if err := executor.PushTag(ctx, newVersion.String()); err != nil {
		return fmt.Errorf("pushing tag: %w", err)
	}
	successf(w, "Tag pushed.")
	fmt.Fprintln(w)

	// --- Trigger release workflow ---
	fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(), "Next step: trigger the 'release.yaml' GitHub Actions workflow."))
	fmt.Fprintf(w, "  Ref:     %s\n", newVersion)
	fmt.Fprintf(w, "  Channel: %s\n", channel)
	fmt.Fprintf(w, "  Payload:\n")
	fmt.Fprintf(w, "    release_channel: %s\n", channel)
	fmt.Fprintf(w, "    release_notes:   (%d chars, written to %s)\n", len(releaseNotes), releaseNotesFile)
	fmt.Fprintln(w)
	if err := confirm(inv, "Trigger release workflow?"); err != nil {
		infof(w, "Skipped workflow trigger. You can trigger it manually from GitHub Actions.")
		fmt.Fprintln(w)
		successf(w, "Done! 🎉")
		return nil
	}
	if err := executor.TriggerWorkflow(ctx, newVersion.String(), channel, releaseNotes); err != nil {
		return fmt.Errorf("triggering workflow: %w", err)
	}
	successf(w, "Release workflow triggered!")
	fmt.Fprintln(w)
	successf(w, "Done! 🎉")
	return nil
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

// ghBuildPRLabelMap returns a map of PR number → label names for
// merged PRs targeting the given branch.
func ghBuildPRLabelMap(branch string) (map[int][]string, error) {
	out, err := ghOutput("pr", "list",
		"--repo", owner+"/"+repo,
		"--base", branch,
		"--state", "merged",
		"--limit", "500",
		"--json", "number,labels",
		"--jq", `.[] | "\(.number)\t\([.labels[].name] | join(","))"`,
	)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	result := make(map[int][]string)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		num, _ := strconv.Atoi(parts[0])
		labels := strings.Split(parts[1], ",")
		sort.Strings(labels)
		result[num] = labels
	}
	return result, nil
}
