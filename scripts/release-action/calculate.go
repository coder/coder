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
	default:
		return nil, xerrors.Errorf("unknown release type %q, must be rc or release", releaseType)
	}
}

func calculateRC(commitSHA string) (*CalculateResult, error) {
	if commitSHA == "" {
		return nil, xerrors.New("--commit is required for RC releases")
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
	channel := determineChannel(branchMajor, branchMinor)

	return &CalculateResult{
		Version:         suggested.String(),
		PreviousVersion: prevVersionStr,
		Channel:         channel,
		TargetRef:       targetRef,
	}, nil
}

// determineChannel finds the latest mainline (non-RC,
// non-prerelease) release globally and compares it against the
// given major.minor to decide the channel.
func determineChannel(major, minor int) string {
	allTags, err := allSemverTags()
	if err != nil {
		// Default to mainline if we cannot determine.
		return "mainline"
	}

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
