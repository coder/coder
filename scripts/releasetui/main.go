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
	"time"

	"golang.org/x/xerrors"

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
	mnr, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return version{Major: maj, Minor: mnr, Patch: pat}, true
}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v version) GT(b version) bool {
	if v.Major != b.Major {
		return v.Major > b.Major
	}
	if v.Minor != b.Minor {
		return v.Minor > b.Minor
	}
	return v.Patch > b.Patch
}

func (v version) Eq(b version) bool {
	return v.Major == b.Major && v.Minor == b.Minor && v.Patch == b.Patch
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

//nolint:revive // sign flag is part of the ReleaseExecutor interface contract.
func (e *liveExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	args := []string{"tag", "-a"}
	if sign {
		args = append(args, "-s")
	}
	args = append(args, tag, "-m", message, ref)
	return gitRun(args...)
}

func (*liveExecutor) PushTag(_ context.Context, tag string) error {
	return gitRun("push", "origin", tag)
}

func (*liveExecutor) TriggerWorkflow(_ context.Context, ref, channel, releaseNotes string) error {
	payload := map[string]string{
		"dry_run":         "false",
		"release_channel": channel,
		"release_notes":   releaseNotes,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return xerrors.Errorf("marshaling workflow payload: %w", err)
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

//nolint:revive // sign flag is part of the ReleaseExecutor interface contract.
func (e *dryRunExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	signFlag := ""
	if sign {
		signFlag = "-s "
	}
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would run: git tag %s-a %s -m %q %s\n", signFlag, tag, message, ref)
	return nil
}

func (e *dryRunExecutor) PushTag(_ context.Context, tag string) error {
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would run: git push origin %s\n", tag)
	return nil
}

func (e *dryRunExecutor) TriggerWorkflow(_ context.Context, ref, channel, _ string) error {
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would trigger release.yaml workflow (ref=%s, channel=%s)\n", ref, channel)
	return nil
}

// gitOutput runs a read-only git command and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", xerrors.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return "", xerrors.Errorf("git %s: %w", strings.Join(args, " "), err)
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
			return "", xerrors.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, exitErr.Stderr)
		}
		return "", xerrors.Errorf("gh %s: %w", strings.Join(args, " "), err)
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
				return xerrors.New("git is required but not found in PATH")
			}

			// --- Check GPG signing ---
			signingKey, _ := gitOutput("config", "--get", "user.signingkey")
			gpgFormat, _ := gitOutput("config", "--get", "gpg.format")
			gpgConfigured := signingKey != "" || gpgFormat != ""
			if !gpgConfigured {
				warnf(w, "GPG signing is not configured. Tags will be unsigned — there will be no way to verify who pushed the tag.")
				_, _ = fmt.Fprintf(w, "  To fix: set git config user.signingkey or gpg.format\n")
				if err := confirmWithDefault(inv, "Continue without signing?", cliui.ConfirmNo); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(w)
			}

			// --- Check gh CLI auth ---
			ghAvailable := checkGHAuth()
			if !ghAvailable {
				warnf(w, "gh CLI is not available or not authenticated.")
				infof(w, "Continuing without GitHub features (PR checks, label lookups, workflow trigger).")
				_, _ = fmt.Fprintln(w)
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
	SHA       string
	FullSHA   string
	Title     string
	PRNum     int // 0 if no PR number found
	Timestamp int64
}

var prNumRe = regexp.MustCompile(`\(#(\d+)\)`)

// cherryPickPRRe matches cherry-pick bot titles like
// "chore: foo bar (cherry-pick #42) (#43)".
var cherryPickPRRe = regexp.MustCompile(`\(cherry-pick #(\d+)\)\s*\(#\d+\)$`)

// commitLog returns non-merge commits in the given range, filtering
// out left-side commits (already in the base) and deduplicating
// cherry-picks using git's --cherry-mark.
func commitLog(commitRange string) ([]commitEntry, error) {
	// Use --left-right --cherry-mark to identify equivalent
	// (cherry-picked) commits and left-side-only commits.
	out, err := gitOutput("log", "--no-merges", "--left-right", "--cherry-mark",
		"--pretty=format:%m %ct %h %H %s", commitRange)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	// Collect cherry-pick equivalent commits (marked with '=') so
	// we can skip duplicates. We keep only the right-side version.
	seen := make(map[string]bool)

	var entries []commitEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: %m %ct %h %H %s
		// mark timestamp shortSHA fullSHA title...
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}
		mark := parts[0]
		ts, _ := strconv.ParseInt(parts[1], 10, 64)
		shortSHA := parts[2]
		fullSHA := parts[3]
		title := parts[4]

		// Skip left-side commits (already in the old version).
		if mark == "<" {
			continue
		}
		// Skip cherry-pick equivalents that we've already seen
		// (marked '=' by --cherry-mark).
		if mark == "=" {
			if seen[title] {
				continue
			}
			seen[title] = true
		}

		// Normalize cherry-pick bot titles:
		// "chore: foo (cherry-pick #42) (#43)" → "chore: foo (#42)"
		if m := cherryPickPRRe.FindStringSubmatch(title); m != nil {
			title = title[:cherryPickPRRe.FindStringIndex(title)[0]] + "(#" + m[1] + ")"
		}

		e := commitEntry{
			SHA:       shortSHA,
			FullSHA:   fullSHA,
			Title:     title,
			Timestamp: ts,
		}
		if m := prNumRe.FindStringSubmatch(e.Title); m != nil {
			e.PRNum, _ = strconv.Atoi(m[1])
		}
		entries = append(entries, e)
	}

	// Sort by conventional commit prefix, then by timestamp
	// (matching the bash script's sort -k3,3 -k1,1n).
	sort.SliceStable(entries, func(i, j int) bool {
		pi := commitSortPrefix(entries[i].Title)
		pj := commitSortPrefix(entries[j].Title)
		if pi != pj {
			return pi < pj
		}
		return entries[i].Timestamp < entries[j].Timestamp
	})

	return entries, nil
}

// commitSortPrefix extracts the first word of a title for sorting.
func commitSortPrefix(title string) string {
	idx := strings.IndexAny(title, " (:")
	if idx < 0 {
		return title
	}
	return title[:idx]
}

// prMetadata holds labels and author for a merged PR.
type prMetadata struct {
	Labels []string
	Author string
}

// humanizedAreas maps conventional commit scopes to human-readable area
// names. Order matters: more specific prefixes must come first so that
// the first partial match wins.
var humanizedAreas = []struct {
	Prefix string
	Area   string
}{
	{"agent/agentssh", "Agent SSH"},
	{"coderd/database", "Database"},
	{"enterprise/audit", "Auditing"},
	{"enterprise/cli", "CLI"},
	{"enterprise/coderd", "Server"},
	{"enterprise/dbcrypt", "Database"},
	{"enterprise/derpmesh", "Networking"},
	{"enterprise/provisionerd", "Provisioner"},
	{"enterprise/tailnet", "Networking"},
	{"enterprise/wsproxy", "Workspace Proxy"},
	{"agent", "Agent"},
	{"cli", "CLI"},
	{"coderd", "Server"},
	{"codersdk", "SDK"},
	{"docs", "Documentation"},
	{"enterprise", "Enterprise"},
	{"examples", "Examples"},
	{"helm", "Helm"},
	{"install.sh", "Installer"},
	{"provisionersdk", "SDK"},
	{"provisionerd", "Provisioner"},
	{"provisioner", "Provisioner"},
	{"pty", "CLI"},
	{"scaletest", "Scale Testing"},
	{"site", "Dashboard"},
	{"support", "Support"},
	{"tailnet", "Networking"},
}

// conventionalPrefixRe extracts prefix, scope, and rest from a
// conventional commit title. Does NOT match breaking "!" suffix —
// those titles are left as-is (matching bash behavior).
var conventionalPrefixRe = regexp.MustCompile(`^([a-z]+)(\((.+)\))?:\s*(.*)$`)

// humanizeTitle converts a conventional commit title to a
// human-readable form, e.g. "feat(site): add bar" → "Dashboard: Add bar".
func humanizeTitle(title string) string {
	m := conventionalPrefixRe.FindStringSubmatch(title)
	if m == nil {
		return title
	}
	scope := m[3] // may be empty
	rest := m[4]
	if rest == "" {
		return title
	}
	// Capitalize the first letter of the rest.
	rest = strings.ToUpper(rest[:1]) + rest[1:]

	if scope == "" {
		return rest
	}

	// Look up scope in humanizedAreas (first partial match wins).
	for _, ha := range humanizedAreas {
		if strings.HasPrefix(scope, ha.Prefix) {
			return ha.Area + ": " + rest
		}
	}
	// Scope not found in map — return as-is.
	return title
}

// breakingCommitRe matches conventional commit "!:" breaking changes.
var breakingCommitRe = regexp.MustCompile(`^[a-zA-Z]+(\(.+\))?!:`)

// categorizeCommit determines the release note section for a commit.
// The priority order matches the bash script: breaking title first,
// then labels (breaking, security, experimental), then prefix.
func categorizeCommit(title string, labels []string) string {
	// Check breaking title first (matches bash behavior).
	if breakingCommitRe.MatchString(title) {
		return "breaking"
	}

	// Label-based categorization.
	for _, l := range labels {
		if l == "release/breaking" {
			return "breaking"
		}
		if l == "security" {
			return "security"
		}
		if l == "release/experimental" {
			return "experimental"
		}
	}

	// Extract the conventional commit prefix (e.g. "feat", "fix(scope)").
	prefixRe := regexp.MustCompile(`^([a-z]+)(\(.+\))?[!]?:`)
	m := prefixRe.FindStringSubmatch(title)
	if m != nil {
		switch m[1] {
		case "feat":
			return "feat"
		case "fix":
			return "fix"
		case "docs":
			return "docs"
		case "style":
			return "other"
		case "refactor":
			return "refactor"
		case "perf":
			return "perf"
		case "test":
			return "test"
		case "build":
			return "build"
		case "ci":
			return "ci"
		case "chore":
			return "chore"
		case "revert":
			return "revert"
		}
	}
	return "other"
}

//nolint:revive // Long function is fine for a sequential release flow.
func runRelease(ctx context.Context, inv *serpent.Invocation, executor ReleaseExecutor, ghAvailable, gpgConfigured bool) error {
	w := inv.Stderr

	// --- Release landscape ---
	infof(w, "Checking current releases...")
	allTags, err := allSemverTags()
	if err != nil {
		return xerrors.Errorf("listing tags: %w", err)
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
	branchMajor, _ := strconv.Atoi(m[1])
	branchMinor, _ := strconv.Atoi(m[2])
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
				return xerrors.New("must be in format vMAJOR.MINOR.PATCH (e.g. v2.31.1)")
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
	if prevVersion != nil { //nolint:nestif // Sequential release checks are inherently nested.
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

	// --- Channel selection ---
	// This is done before release notes generation because the
	// notes format differs between mainline and stable channels.
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

	// --- Generate release notes ---
	infof(w, "Generating release notes...")

	commitRange := "HEAD"
	if prevVersion != nil {
		commitRange = prevVersion.String() + "..HEAD"
	}

	commits, err := commitLog(commitRange)
	if err != nil {
		return xerrors.Errorf("reading commit log: %w", err)
	}

	// Build merge-commit SHA → metadata map via gh CLI.
	var prMeta map[string]prMetadata
	if ghAvailable {
		prMeta, err = ghBuildPRMetadataMap(commits)
		if err != nil {
			warnf(w, "Failed to fetch PR metadata: %v", err)
		}
	}
	if prMeta == nil {
		prMeta = make(map[string]prMetadata)
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
		meta := prMeta[c.FullSHA]
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
		if c.PRNum > 0 {
			entry = fmt.Sprintf("- %s (#%d, %s)", humanTitle, c.PRNum, c.SHA)
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
	fmt.Fprintln(&notes, "## Changelog")
	if channel == "mainline" {
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
