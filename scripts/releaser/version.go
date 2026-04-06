package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// version holds a parsed semver version with optional prerelease
// suffix (e.g. "rc.0").
type version struct {
	Major int
	Minor int
	Patch int
	Pre   string // e.g. "rc.0", "" for stable releases.
}

var semverRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(-(.+))?$`)

func parseVersion(s string) (version, bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	mnr, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return version{Major: maj, Minor: mnr, Patch: pat, Pre: m[5]}, true
}

func (v version) String() string {
	if v.Pre != "" {
		return fmt.Sprintf("v%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Pre)
	}
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// IsRC returns true when the version has a prerelease suffix starting
// with "rc." (e.g. "rc.0", "rc.1").
func (v version) IsRC() bool {
	return strings.HasPrefix(v.Pre, "rc.")
}

// rcNumber returns the numeric RC identifier (e.g. 0 for "rc.0").
// It returns -1 when the version is not an RC.
func (v version) rcNumber() int {
	if !v.IsRC() {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(v.Pre, "rc."))
	if err != nil {
		return -1
	}
	return n
}

func (v version) GreaterThan(b version) bool {
	if v.Major != b.Major {
		return v.Major > b.Major
	}
	if v.Minor != b.Minor {
		return v.Minor > b.Minor
	}
	if v.Patch != b.Patch {
		return v.Patch > b.Patch
	}
	// A release without prerelease suffix is greater than one
	// with a prerelease suffix (v2.32.0 > v2.32.0-rc.0).
	if v.Pre == "" && b.Pre != "" {
		return true
	}
	if v.Pre != "" && b.Pre == "" {
		return false
	}
	// Both have prerelease: compare numerically for RC versions.
	if v.IsRC() && b.IsRC() {
		return v.rcNumber() > b.rcNumber()
	}
	// Fallback for non-RC prerelease strings.
	return v.Pre > b.Pre
}

func (v version) Equal(b version) bool {
	return v.Major == b.Major && v.Minor == b.Minor && v.Patch == b.Patch && v.Pre == b.Pre
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
