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

// calculateNextVersion dispatches to the appropriate calculation.
//
// ref is the branch name from the "Use workflow from" dropdown
// (github.ref_name). commitSHA is an optional override; when empty
// the tool defaults to HEAD of the ref.
func calculateNextVersion(releaseType, ref, commitSHA string) (CalculateResult, error) {
	// Ensure we have up-to-date remote state.
	if _, err := gitOutput("fetch", "--tags", "--force", "origin"); err != nil {
		return CalculateResult{}, xerrors.Errorf("git fetch: %w", err)
	}

	isReleaseBranch := branchRe.MatchString(ref)
	isMain := ref == "main"

	switch releaseType {
	case "rc":
		if !isMain && !isReleaseBranch {
			return CalculateResult{}, xerrors.Errorf("rc must be run from main or a release/X.Y branch, got %q", ref)
		}
		if isMain {
			return calculateRCFromMain(ref, commitSHA)
		}
		return calculateRCFromBranch(ref, commitSHA)

	case "release":
		if !isReleaseBranch {
			return CalculateResult{}, xerrors.Errorf("release must be run from a release/X.Y branch, got %q", ref)
		}
		return calculateRelease(ref)

	case "create-release-branch":
		if !isMain {
			return CalculateResult{}, xerrors.Errorf("create-release-branch must be run from main, got %q", ref)
		}
		return calculateCreateBranch(ref, commitSHA)

	default:
		return CalculateResult{}, xerrors.Errorf("unknown release type %q (expected rc, release, or create-release-branch)", releaseType)
	}
}

// resolveCommit returns the commit SHA to tag. If commitSHA is
// provided it is validated and returned; otherwise HEAD of the
// ref is used.
func resolveCommit(ref, commitSHA string) (string, error) {
	if commitSHA != "" {
		if !isHexSHA(commitSHA) {
			return "", xerrors.Errorf("invalid commit SHA %q: must be a hex string", commitSHA)
		}
		return commitSHA, nil
	}
	sha, err := gitOutput("rev-parse", fmt.Sprintf("origin/%s", ref))
	if err != nil {
		return "", xerrors.Errorf("resolve HEAD of %s: %w", ref, err)
	}
	return sha, nil
}

// calculateRCFromMain tags an RC from a commit on main.
func calculateRCFromMain(ref, commitSHA string) (CalculateResult, error) {
	targetRef, err := resolveCommit(ref, commitSHA)
	if err != nil {
		return CalculateResult{}, err
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", targetRef, "origin/main"); err != nil {
		return CalculateResult{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", targetRef)
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find latest RC globally to determine series.
	latestRC := findLatestRC(allTags)
	latestRelease := findLatestNonRC(allTags)

	var major, minor, rcNum int
	switch {
	case latestRC.original != "":
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
	case latestRelease.original != "":
		major = latestRelease.major
		minor = latestRelease.minor + 1
		rcNum = 0
	default:
		return CalculateResult{}, xerrors.New("no existing tags found to base RC on")
	}

	newVer := version{major: major, minor: minor, patch: 0, rc: rcNum}
	prevTag := findPreviousTag(allTags, newVer)

	return CalculateResult{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Channel:         "rc",
		TargetRef:       targetRef,
	}, nil
}

// calculateRCFromBranch tags an RC from the tip of a release branch.
func calculateRCFromBranch(ref, commitSHA string) (CalculateResult, error) {
	m := branchRe.FindStringSubmatch(ref)
	if m == nil {
		return CalculateResult{}, xerrors.Errorf("ref %q does not match release/X.Y", ref)
	}

	major := mustAtoi(m[1])
	minor := mustAtoi(m[2])

	targetRef, err := resolveCommit(ref, commitSHA)
	if err != nil {
		return CalculateResult{}, err
	}

	// Fail if there are open PRs targeting this release branch.
	if err := checkOpenPRs(ref); err != nil {
		return CalculateResult{}, err
	}

	allTags, err := listSemverTags()
	if err != nil {
		return CalculateResult{}, err
	}

	// Find tags for this series.
	seriesTags := filterTagsForSeries(allTags, major, minor)

	// If the series already has a final release, this is an error;
	// you should be cutting a new minor, not more RCs.
	for _, t := range seriesTags {
		if t.rc < 0 {
			return CalculateResult{}, xerrors.Errorf(
				"release %s already exists for this series; cut a new minor instead of another RC",
				t.original,
			)
		}
	}

	rcNum := 0
	for _, t := range seriesTags {
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
		TargetRef:       targetRef,
	}, nil
}

// calculateRelease calculates the next release (non-RC) version from
// a release branch. Uses HEAD of the branch.
func calculateRelease(ref string) (CalculateResult, error) {
	m := branchRe.FindStringSubmatch(ref)
	if m == nil {
		return CalculateResult{}, xerrors.Errorf("ref %q does not match release/X.Y", ref)
	}

	major := mustAtoi(m[1])
	minor := mustAtoi(m[2])

	// Resolve branch HEAD.
	headSHA, err := gitOutput("rev-parse", fmt.Sprintf("origin/%s", ref))
	if err != nil {
		return CalculateResult{}, xerrors.Errorf("resolve branch %s: %w", ref, err)
	}

	// Fail if there are open PRs targeting this release branch.
	if err := checkOpenPRs(ref); err != nil {
		return CalculateResult{}, err
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

// calculateCreateBranch creates a release branch and tags the next
// RC in one atomic step. Must be run from main.
func calculateCreateBranch(ref, commitSHA string) (CalculateResult, error) {
	targetRef, err := resolveCommit(ref, commitSHA)
	if err != nil {
		return CalculateResult{}, err
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", targetRef, "origin/main"); err != nil {
		return CalculateResult{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", targetRef)
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
		TargetRef:       targetRef,
		CreateBranch:    branchName,
	}, nil
}

// determineChannel returns the release channel. Currently always
// returns "stable" since all non-RC releases are stable from day one.
func determineChannel(_, _ int, _ []version) string {
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
