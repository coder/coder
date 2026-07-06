package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// calculateResult is implemented by both ReleaseRequest and
// CreateBranchRequest so calculateNextVersion can return either.
type calculateResult interface {
	String() string
}

// ReleaseRequest is the JSON output of calculate-version for rc and
// release types.
type ReleaseRequest struct {
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
	Stable          bool   `json:"stable"`
	TargetRef       string `json:"target_ref"`
}

// String returns the result as indented JSON.
func (r ReleaseRequest) String() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

// CreateBranchRequest is the JSON output of calculate-version for the
// create-release-branch type.
type CreateBranchRequest struct {
	ReleaseRequest
	BranchName string `json:"create_branch"`
}

// String returns the result as indented JSON.
func (r CreateBranchRequest) String() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

var branchRe = regexp.MustCompile(`^release/(\d+)\.(\d+)$`)

// calculateNextVersion dispatches to the appropriate calculation.
//
// ref is the branch name from the "Use workflow from" dropdown
// (github.ref_name). commitSHA is an optional override; when empty
// the tool defaults to HEAD of the ref.
func calculateNextVersion(exec CommandExecutor, releaseType, ref, commitSHA string) (calculateResult, error) {
	// Ensure we have up-to-date remote state. Fetching only updates
	// local remote-tracking refs, so it runs even in dry-run mode to
	// keep version calculation accurate.
	if _, err := gitOutput(exec, "fetch", "--tags", "--force", "origin"); err != nil {
		return nil, xerrors.Errorf("git fetch: %w", err)
	}

	isReleaseBranch := branchRe.MatchString(ref)
	isMain := ref == "main"

	switch releaseType {
	case "rc":
		if !isMain && !isReleaseBranch {
			return nil, xerrors.Errorf("rc must be run from main or a release/X.Y branch, got %q", ref)
		}
		if isMain {
			return calculateRCFromMainReleaseRequest(exec, ref, commitSHA)
		}
		return calculateRCFromBranchReleaseRequest(exec, ref, commitSHA)

	case "release":
		if !isReleaseBranch {
			return nil, xerrors.Errorf("release must be run from a release/X.Y branch, got %q", ref)
		}
		return createRegularReleaseRequest(exec, ref)

	case "create-release-branch":
		if !isMain {
			return nil, xerrors.Errorf("create-release-branch must be run from main, got %q", ref)
		}
		return calculateCreateBranchRequest(exec, ref, commitSHA)

	default:
		return nil, xerrors.Errorf("unknown release type %q (expected rc, release, or create-release-branch)", releaseType)
	}
}

// resolveCommit returns the commit SHA to tag. If commitSHA is
// provided it is validated and returned; otherwise HEAD of the
// ref is used.
func resolveCommit(exec CommandExecutor, ref, commitSHA string) (string, error) {
	if commitSHA != "" {
		if !isHexSHA(commitSHA) {
			return "", xerrors.Errorf("invalid commit SHA %q: must be a hex string", commitSHA)
		}
		// Resolve to a full commit SHA. The idempotency checks in
		// createAndPushTag/createAndPushBranch compare targetRef
		// against full SHAs (git rev-parse of an existing tag,
		// ls-remote branch output), so a short SHA passed via the
		// commit input would never match and would break re-runs.
		sha, err := gitOutput(exec, "rev-parse", "--verify", commitSHA+"^{commit}")
		if err != nil {
			return "", xerrors.Errorf("resolve commit %s: %w", commitSHA, err)
		}
		return sha, nil
	}
	sha, err := gitOutput(exec, "rev-parse", fmt.Sprintf("origin/%s", ref))
	if err != nil {
		return "", xerrors.Errorf("resolve HEAD of %s: %w", ref, err)
	}
	return sha, nil
}

// calculateRCFromMainReleaseRequest tags an RC from a commit on main.
func calculateRCFromMainReleaseRequest(exec CommandExecutor, ref, commitSHA string) (ReleaseRequest, error) {
	targetRef, err := resolveCommit(exec, ref, commitSHA)
	if err != nil {
		return ReleaseRequest{}, err
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun(exec, "merge-base", "--is-ancestor", targetRef, "origin/main"); err != nil {
		return ReleaseRequest{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", targetRef)
	}

	allTags, err := listSemverTags(exec)
	if err != nil {
		return ReleaseRequest{}, err
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
		return ReleaseRequest{}, xerrors.New("no existing tags found to base RC on")
	}

	newVer := version{major: major, minor: minor, patch: 0, rc: rcNum}
	prevTag := findPreviousTag(allTags, newVer)

	return ReleaseRequest{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		TargetRef:       targetRef,
	}, nil
}

// calculateRCFromBranchReleaseRequest tags an RC from the tip of a release branch.
func calculateRCFromBranchReleaseRequest(exec CommandExecutor, ref, commitSHA string) (ReleaseRequest, error) {
	m := branchRe.FindStringSubmatch(ref)
	if m == nil {
		return ReleaseRequest{}, xerrors.Errorf("ref %q does not match release/X.Y", ref)
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])

	targetRef, err := resolveCommit(exec, ref, commitSHA)
	if err != nil {
		return ReleaseRequest{}, err
	}

	// Fail if there are open PRs targeting this release branch.
	if err := checkOpenPRs(exec, ref); err != nil {
		return ReleaseRequest{}, err
	}

	allTags, err := listSemverTags(exec)
	if err != nil {
		return ReleaseRequest{}, err
	}

	// Find tags for this series.
	seriesTags := filterTagsForSeries(allTags, major, minor)

	// If the series already has a final release, this is an error;
	// you should be cutting a new minor, not more RCs.
	for _, t := range seriesTags {
		if t.rc < 0 {
			return ReleaseRequest{}, xerrors.Errorf(
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

	return ReleaseRequest{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		TargetRef:       targetRef,
	}, nil
}

// createRegularReleaseRequest calculates the next release (non-RC) version from
// a release branch. Uses HEAD of the branch.
func createRegularReleaseRequest(exec CommandExecutor, ref string) (ReleaseRequest, error) {
	m := branchRe.FindStringSubmatch(ref)
	if m == nil {
		return ReleaseRequest{}, xerrors.Errorf("ref %q does not match release/X.Y", ref)
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])

	// Resolve branch HEAD.
	headSHA, err := gitOutput(exec, "rev-parse", fmt.Sprintf("origin/%s", ref))
	if err != nil {
		return ReleaseRequest{}, xerrors.Errorf("resolve branch %s: %w", ref, err)
	}

	// Fail if there are open PRs targeting this release branch.
	if err := checkOpenPRs(exec, ref); err != nil {
		return ReleaseRequest{}, err
	}

	allTags, err := listSemverTags(exec)
	if err != nil {
		return ReleaseRequest{}, err
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
	prevTag := findPreviousTag(allTags, newVer)

	return ReleaseRequest{
		Version:         newVer.String(),
		PreviousVersion: prevTag,
		Stable:          isStable(major, minor, allTags),
		TargetRef:       headSHA,
	}, nil
}

// calculateCreateBranchRequest creates a release branch and tags the next
// RC in one atomic step. Must be run from main.
func calculateCreateBranchRequest(exec CommandExecutor, ref, commitSHA string) (CreateBranchRequest, error) {
	targetRef, err := resolveCommit(exec, ref, commitSHA)
	if err != nil {
		return CreateBranchRequest{}, err
	}

	// Verify commit is an ancestor of origin/main.
	if err := gitRun(exec, "merge-base", "--is-ancestor", targetRef, "origin/main"); err != nil {
		return CreateBranchRequest{}, xerrors.Errorf("commit %s is not an ancestor of origin/main", targetRef)
	}

	allTags, err := listSemverTags(exec)
	if err != nil {
		return CreateBranchRequest{}, err
	}

	// Find latest non-RC release.
	latest := findLatestNonRC(allTags)
	if latest.original == "" {
		return CreateBranchRequest{}, xerrors.New("no existing releases found")
	}

	nextMajor := latest.major
	nextMinor := latest.minor + 1
	branchName := fmt.Sprintf("release/%d.%d", nextMajor, nextMinor)

	// Check that the branch doesn't already exist.
	if _, err := gitOutput(exec, "rev-parse", "--verify", fmt.Sprintf("origin/%s", branchName)); err == nil {
		return CreateBranchRequest{}, xerrors.Errorf("branch %s already exists", branchName)
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

	return CreateBranchRequest{
		ReleaseRequest: ReleaseRequest{
			Version:         newVer.String(),
			PreviousVersion: prevTag,
			TargetRef:       targetRef,
		},
		BranchName: branchName,
	}, nil
}

// isStable returns true if this minor series is exactly one behind
// the latest released minor (i.e. it is the "stable" channel).
func isStable(major, minor int, allTags []version) bool {
	latest := findLatestNonRC(allTags)
	return latest.original != "" && latest.major == major && latest.minor == minor+1
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
		if best.original == "" || versionIsLess(best, t) {
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
		if best.original == "" || versionIsLess(best, t) {
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
		if !versionIsLess(t, newVer) {
			continue
		}
		if best.original == "" || versionIsLess(best, t) {
			best = t
		}
	}
	return best.original
}

// versionIsLess returns true if a < b using semver ordering.
func versionIsLess(a, b version) bool {
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

// listSemverTags returns all semver tags from the repo.
func listSemverTags(exec CommandExecutor) ([]version, error) {
	out, err := gitOutput(exec, "tag", "--list", "v*")
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
