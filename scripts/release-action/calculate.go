package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// CalculateResult is the JSON output of calculate-version.
type CalculateResult struct {
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
	Channel         string `json:"channel"`
	TargetRef       string `json:"target_ref"`
	CreateBranch    string `json:"create_branch,omitempty"`
}

// String returns the result as indented JSON.
func (r CalculateResult) String() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

var branchRe = regexp.MustCompile(`^release/(\d+)\.(\d+)$`)

// calculateNextVersion dispatches to the appropriate calculation based
// on releaseType.
func calculateNextVersion(releaseType, commitSHA, branch string) (CalculateResult, error) {
	// Ensure we have up-to-date remote state.
	if _, err := gitOutput("fetch", "--tags", "--force", "origin"); err != nil {
		return CalculateResult{}, xerrors.Errorf("git fetch: %w", err)
	}

	switch releaseType {
	case "rc":
		return calculateRC(commitSHA, branch)
	case "release":
		return calculateRelease(branch)
	case "create-release-branch":
		return calculateCreateBranch(commitSHA)
	default:
		return CalculateResult{}, xerrors.Errorf("unknown release type %q (expected rc, release, or create-release-branch)", releaseType)
	}
}

// calculateRC calculates the next RC version.
//
// If commitSHA is set, it tags an RC from a specific commit on main.
// If branch is set, it tags an RC from the tip of a release branch.
func calculateRC(commitSHA, branch string) (CalculateResult, error) {
	if commitSHA != "" && branch != "" {
		return CalculateResult{}, xerrors.New("only one of --commit or --branch may be set for rc")
	}
	if commitSHA == "" && branch == "" {
		return CalculateResult{}, xerrors.New("one of --commit or --branch is required for rc")
	}

	if commitSHA != "" {
		return calculateRCFromCommit(commitSHA)
	}
	return calculateRCFromBranch(branch)
}

func calculateRCFromCommit(commitSHA string) (CalculateResult, error) {
	if !isHexSHA(commitSHA) {
		return CalculateResult{}, xerrors.Errorf("invalid commit SHA %q: must be a hex string", commitSHA)
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", commitSHA, "origin/main"); err != nil {
		return CalculateResult{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", commitSHA)
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find latest RC globally to determine series.
	latestRC := findLatestRC(allTags)
	latestRelease := findLatestNonRC(allTags)

	var major, minor, rcNum int
	if latestRC.original != "" {
		major = latestRC.major
		minor = latestRC.minor
		rcNum = latestRC.rc + 1

		// If there is a final release for this series, bump minor.
		if latestRelease.original != "" &&
			latestRelease.major == major &&
			latestRelease.minor == minor {
			minor++
			rcNum = 0
		}
	} else if latestRelease.original != "" {
		major = latestRelease.major
		minor = latestRelease.minor + 1
		rcNum = 0
	} else {
		return CalculateResult{}, xerrors.New("no existing tags found to base RC on")
	}

	newVer := version{major: major, minor: minor, patch: 0, rc: rcNum}
	prevTag := findPreviousTag(allTags, newVer)

	return CalculateResult{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Channel:         "rc",
		TargetRef:       commitSHA,
	}, nil
}

func calculateRCFromBranch(branch string) (CalculateResult, error) {
	m := branchRe.FindStringSubmatch(branch)
	if m == nil {
		return CalculateResult{}, xerrors.Errorf("branch %q does not match release/X.Y", branch)
	}

	major := mustAtoi(m[1])
	minor := mustAtoi(m[2])

	// Resolve branch HEAD.
	headSHA, err := gitOutput("rev-parse", fmt.Sprintf("origin/%s", branch))
	if err != nil {
		return CalculateResult{}, xerrors.Errorf("resolve branch %s: %w", branch, err)
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find tags merged into branch for this series.
	branchTags := filterTagsForSeries(allTags, major, minor)

	// If the series already has a final release, bump minor and start at rc.0.
	hasFinal := false
	for _, t := range branchTags {
		if t.rc < 0 {
			hasFinal = true
			break
		}
	}
	if hasFinal {
		minor++
		branchTags = filterTagsForSeries(allTags, major, minor)
	}

	rcNum := 0
	for _, t := range branchTags {
		if t.rc >= rcNum {
			rcNum = t.rc + 1
		}
	}

	newVer := version{major: major, minor: minor, patch: 0, rc: rcNum}
	prevTag := findPreviousTag(allTags, newVer)

	return CalculateResult{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Channel:         "rc",
		TargetRef:       headSHA,
	}, nil
}

// calculateRelease calculates the next release (non-RC) version from
// a release branch.
func calculateRelease(branch string) (CalculateResult, error) {
	if branch == "" {
		return CalculateResult{}, xerrors.New("--branch is required for release type")
	}

	m := branchRe.FindStringSubmatch(branch)
	if m == nil {
		return CalculateResult{}, xerrors.Errorf("branch %q does not match release/X.Y", branch)
	}

	major := mustAtoi(m[1])
	minor := mustAtoi(m[2])

	// Resolve branch HEAD.
	headSHA, err := gitOutput("rev-parse", fmt.Sprintf("origin/%s", branch))
	if err != nil {
		return CalculateResult{}, xerrors.Errorf("resolve branch %s: %w", branch, err)
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find tags for this series.
	seriesTags := filterTagsForSeries(allTags, major, minor)

	// Determine next patch version.
	nextPatch := 0
	for _, t := range seriesTags {
		if t.rc < 0 && t.patch >= nextPatch {
			nextPatch = t.patch + 1
		}
	}

	newVer := version{major: major, minor: minor, patch: nextPatch, rc: -1}
	channel := determineChannel(major, minor, allTags)
	prevTag := findPreviousTag(allTags, newVer)

	return CalculateResult{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Channel:         channel,
		TargetRef:       headSHA,
	}, nil
}

// calculateCreateBranch creates a release branch and tags the first
// RC in one atomic step.
func calculateCreateBranch(commitSHA string) (CalculateResult, error) {
	if commitSHA == "" {
		return CalculateResult{}, xerrors.New("--commit is required for create-release-branch type")
	}
	if !isHexSHA(commitSHA) {
		return CalculateResult{}, xerrors.Errorf("invalid commit SHA %q: must be a hex string", commitSHA)
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", commitSHA, "origin/main"); err != nil {
		return CalculateResult{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", commitSHA)
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find latest non-RC release.
	latest := findLatestNonRC(allTags)
	if latest.original == "" {
		return CalculateResult{}, xerrors.New("no existing releases found")
	}

	nextMajor := latest.major
	nextMinor := latest.minor + 1
	branchName := fmt.Sprintf("release/%d.%d", nextMajor, nextMinor)

	// Check that the branch doesn't already exist.
	if _, err := gitOutput("rev-parse", "--verify", fmt.Sprintf("origin/%s", branchName)); err == nil {
		return CalculateResult{}, xerrors.Errorf("branch %s already exists", branchName)
	}

	// Find existing RCs for this series to continue the sequence.
	rcNum := 0
	seriesTags := filterTagsForSeries(allTags, nextMajor, nextMinor)
	for _, t := range seriesTags {
		if t.rc >= rcNum {
			rcNum = t.rc + 1
		}
	}

	newVer := version{major: nextMajor, minor: nextMinor, patch: 0, rc: rcNum}
	prevTag := findPreviousTag(allTags, newVer)

	return CalculateResult{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Channel:         "rc",
		TargetRef:       commitSHA,
		CreateBranch:    branchName,
	}, nil
}

// determineChannel returns the release channel. Currently always
// returns "stable" since the mainline/stable distinction has been
// removed.
func determineChannel(major, minor int, allTags []version) string {
	_ = major
	_ = minor
	_ = allTags
	return "stable"
}

// isHexSHA validates that s looks like a hex commit SHA.
func isHexSHA(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// findLatestRC returns the highest RC version from the tag list.
func findLatestRC(tags []version) version {
	var best version
	for _, t := range tags {
		if t.rc < 0 {
			continue
		}
		if best.original == "" || versionLess(best, t) {
			best = t
		}
	}
	return best
}

// findLatestNonRC returns the highest non-RC version from the tag list.
func findLatestNonRC(tags []version) version {
	var best version
	for _, t := range tags {
		if t.rc >= 0 {
			continue
		}
		if best.original == "" || versionLess(best, t) {
			best = t
		}
	}
	return best
}

// filterTagsForSeries returns tags matching the given major.minor.
func filterTagsForSeries(tags []version, major, minor int) []version {
	var out []version
	for _, t := range tags {
		if t.major == major && t.minor == minor {
			out = append(out, t)
		}
	}
	return out
}

// findPreviousTag returns the version string of the best previous
// tag for building a changelog range. It picks the highest tag that
// is strictly less than newVer.
func findPreviousTag(tags []version, newVer version) string {
	var best version
	for _, t := range tags {
		if !versionLess(t, newVer) {
			continue
		}
		if best.original == "" || versionLess(best, t) {
			best = t
		}
	}
	return best.original
}

// versionLess returns true if a < b using semver ordering.
func versionLess(a, b version) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	if a.patch != b.patch {
		return a.patch < b.patch
	}
	// Non-RC (rc == -1) is greater than any RC.
	if a.rc < 0 && b.rc < 0 {
		return false
	}
	if a.rc < 0 {
		return false
	}
	if b.rc < 0 {
		return true
	}
	return a.rc < b.rc
}

func mustAtoi(s string) int {
	var n int
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// listSemverTags returns all semver tags from the repo.
func listSemverTags() ([]version, error) {
	out, err := gitOutput("tag", "--list", "v*")
	if err != nil {
		return nil, xerrors.Errorf("list tags: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	var tags []version
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v, err := parseVersion(line)
		if err != nil {
			continue // skip non-semver tags
		}
		tags = append(tags, v)
	}
	return tags, nil
}
