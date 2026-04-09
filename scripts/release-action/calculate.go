package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// CalculateResult is the JSON output of the calculate-version
// subcommand.
type CalculateResult struct {
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
	Channel         string `json:"channel"`
	TargetRef       string `json:"target_ref"`
	CreateBranch    string `json:"create_branch,omitempty"`
}

// String returns the JSON representation of the result.
func (r *CalculateResult) String() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b) + "\n"
}

// branchRe matches release branch names like "release/2.32".
var branchRe = regexp.MustCompile(`^release/(\d+)\.(\d+)$`)

// calculateNextVersion computes the next release version based on
// the release type (rc or release), a commit SHA (for RCs), or a
// branch name (for releases).
func calculateNextVersion(releaseType, commitSHA, branch string) (*CalculateResult, error) {
	switch releaseType {
	case "rc":
		return calculateRC(commitSHA)
	case "release":
		return calculateRelease(branch)
	case "create-release-branch":
		return calculateCreateBranch(commitSHA)
	default:
		return nil, xerrors.Errorf("unknown release type %q, must be rc, release, or create-release-branch", releaseType)
	}
}

func calculateRC(commitSHA string) (*CalculateResult, error) {
	if commitSHA == "" {
		return nil, xerrors.New("--commit is required for RC releases")
	}

	// Validate that the commit SHA looks like a hex string to
	// prevent shell injection via git arguments.
	hexRe := regexp.MustCompile(`^[0-9a-fA-F]+$`)
	if !hexRe.MatchString(commitSHA) {
		return nil, xerrors.Errorf("--commit must be a hex SHA (got %q)", commitSHA)
	}

	// Verify the commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", commitSHA, "origin/main"); err != nil {
		return nil, xerrors.Errorf("commit %s is not an ancestor of origin/main", commitSHA)
	}

	allTags, err := allSemverTags()
	if err != nil {
		return nil, xerrors.Errorf("listing tags: %w", err)
	}

	// Find latest mainline (non-RC, non-prerelease) release.
	var latestMainline *version
	for _, t := range allTags {
		if t.Pre == "" {
			v := t
			latestMainline = &v
			break
		}
	}

	// Find the latest RC tag.
	var latestRC *version
	for _, t := range allTags {
		if t.IsRC() {
			v := t
			latestRC = &v
			break
		}
	}

	var suggested version
	var prevVersionStr string

	switch {
	case latestRC != nil:
		prevVersionStr = latestRC.String()

		// Check if a final release already exists for this
		// RC's minor series. If so, the series is complete
		// and we start the next minor's RC cycle.
		seriesComplete := false
		for _, t := range allTags {
			if t.Major == latestRC.Major && t.Minor == latestRC.Minor && t.Pre == "" {
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
		prevVersionStr = latestMainline.String()
		suggested = version{
			Major: latestMainline.Major,
			Minor: latestMainline.Minor + 1,
			Patch: 0,
			Pre:   "rc.0",
		}
	default:
		// No previous tags at all — use a sentinel so release
		// notes generation can still produce a valid range.
		prevVersionStr = "v0.0.0"
		suggested = version{Major: 2, Minor: 0, Patch: 0, Pre: "rc.0"}
	}

	return &CalculateResult{
		Version:         suggested.String(),
		PreviousVersion: prevVersionStr,
		Channel:         "rc",
		TargetRef:       commitSHA,
	}, nil
}

func calculateRelease(branch string) (*CalculateResult, error) {
	m := branchRe.FindStringSubmatch(branch)
	if m == nil {
		return nil, xerrors.Errorf("--branch must match release/X.Y (got %q)", branch)
	}
	branchMajor, _ := strconv.Atoi(m[1])
	branchMinor, _ := strconv.Atoi(m[2])

	allTags, err := allSemverTags()
	if err != nil {
		return nil, xerrors.Errorf("listing tags: %w", err)
	}

	// Resolve the branch HEAD as the target ref.
	targetRef, err := gitOutput("rev-parse", "origin/"+branch)
	if err != nil {
		return nil, xerrors.Errorf("resolving origin/%s: %w", branch, err)
	}

	// Find tags merged into that branch matching this
	// major.minor series.
	out, err := gitOutput("tag", "--merged", "origin/"+branch, "--sort=-v:refname")
	if err != nil {
		return nil, xerrors.Errorf("listing merged tags: %w", err)
	}

	var branchTags []version
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			v, ok := parseVersion(strings.TrimSpace(line))
			if !ok {
				continue
			}
			if v.Major == branchMajor && v.Minor == branchMinor {
				branchTags = append(branchTags, v)
			}
		}
	}

	// Determine new version: if no non-RC tag exists, start
	// at .0; otherwise increment the patch.
	var suggested version
	var prevVersionStr string

	var latestNonRC *version
	for _, t := range branchTags {
		if t.Pre == "" {
			v := t
			latestNonRC = &v
			break
		}
	}

	if latestNonRC == nil {
		suggested = version{Major: branchMajor, Minor: branchMinor, Patch: 0}
		// Use the latest RC as previous version for notes.
		if len(branchTags) > 0 {
			prevVersionStr = branchTags[0].String()
		} else {
			// No tags at all on this branch — use a sentinel.
			prevVersionStr = "v0.0.0"
		}
	} else {
		prevVersionStr = latestNonRC.String()
		suggested = version{
			Major: latestNonRC.Major,
			Minor: latestNonRC.Minor,
			Patch: latestNonRC.Patch + 1,
		}
	}

	// Determine channel: compare this minor against the
	// latest mainline release globally.
	channel := determineChannel(branchMajor, branchMinor, allTags)

	return &CalculateResult{
		Version:         suggested.String(),
		PreviousVersion: prevVersionStr,
		Channel:         channel,
		TargetRef:       targetRef,
	}, nil
}

func calculateCreateBranch(commitSHA string) (*CalculateResult, error) {
	if commitSHA == "" {
		return nil, xerrors.New("--commit is required for create-release-branch")
	}

	// Validate that the commit SHA looks like a hex string to
	// prevent shell injection via git arguments.
	hexRe := regexp.MustCompile(`^[0-9a-fA-F]+$`)
	if !hexRe.MatchString(commitSHA) {
		return nil, xerrors.Errorf("--commit must be a hex SHA (got %q)", commitSHA)
	}

	// Verify the commit is an ancestor of origin/main.
	if err := gitRun("merge-base", "--is-ancestor", commitSHA, "origin/main"); err != nil {
		return nil, xerrors.Errorf("commit %s is not an ancestor of origin/main", commitSHA)
	}

	allTags, err := allSemverTags()
	if err != nil {
		return nil, xerrors.Errorf("listing tags: %w", err)
	}

	// Find latest mainline (non-RC, non-prerelease) release.
	var latestMainline *version
	for _, t := range allTags {
		if t.Pre == "" {
			v := t
			latestMainline = &v
			break
		}
	}

	// Determine the next minor version for the branch.
	var major, minor int
	if latestMainline != nil {
		major = latestMainline.Major
		minor = latestMainline.Minor + 1
	} else {
		// No mainline release found — start from a safe default.
		major = 2
		minor = 0
	}

	branchName := fmt.Sprintf("release/%d.%d", major, minor)

	// Check whether the branch already exists on the remote.
	if err := gitRun("rev-parse", "--verify", "origin/"+branchName); err == nil {
		return nil, xerrors.Errorf("branch %s already exists", branchName)
	}

	// Find the latest RC tag for this major.minor series.
	var seriesRC *version
	for _, t := range allTags {
		if t.IsRC() && t.Major == major && t.Minor == minor {
			v := t
			seriesRC = &v
			break
		}
	}

	var suggested version
	var prevVersionStr string

	if seriesRC != nil {
		prevVersionStr = seriesRC.String()
		suggested = version{
			Major: major,
			Minor: minor,
			Patch: 0,
			Pre:   fmt.Sprintf("rc.%d", seriesRC.rcNumber()+1),
		}
	} else {
		if latestMainline != nil {
			prevVersionStr = latestMainline.String()
		} else {
			prevVersionStr = "v0.0.0"
		}
		suggested = version{
			Major: major,
			Minor: minor,
			Patch: 0,
			Pre:   "rc.0",
		}
	}

	return &CalculateResult{
		Version:         suggested.String(),
		PreviousVersion: prevVersionStr,
		Channel:         "rc",
		TargetRef:       commitSHA,
		CreateBranch:    branchName,
	}, nil
}

// determineChannel finds the latest mainline (non-RC,
// non-prerelease) release globally and compares it against the
// given major.minor to decide the channel.
func determineChannel(major, minor int, allTags []version) string {
	var latestMainline *version
	for _, t := range allTags {
		if t.Pre == "" {
			v := t
			latestMainline = &v
			break
		}
	}

	if latestMainline == nil {
		return "mainline"
	}

	if major == latestMainline.Major && minor == latestMainline.Minor {
		return "mainline"
	}
	if major == latestMainline.Major && minor == latestMainline.Minor-1 {
		return "stable"
	}

	return "mainline"
}
