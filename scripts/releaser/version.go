package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// version holds a parsed semver version with optional pre-release
// suffix (e.g. "rc.0").
type version struct {
	Major int
	Minor int
	Patch int
	Pre   string // e.g. "rc.0", empty for stable releases.
}

var semverRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(-([a-zA-Z0-9.]+))?$`)

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
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// IsPrerelease returns true for versions with a pre-release suffix
// (e.g. v2.32.0-rc.0).
func (v version) IsPrerelease() bool {
	return v.Pre != ""
}

// rcNumber extracts the numeric RC identifier from a pre-release
// suffix like "rc.3". Returns -1 when the suffix is not an RC.
func (v version) rcNumber() int {
	if !strings.HasPrefix(v.Pre, "rc.") {
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
	// Per semver: a version without a pre-release suffix has
	// higher precedence than one with a suffix.
	if v.Pre == "" && b.Pre != "" {
		return true
	}
	if v.Pre != "" && b.Pre == "" {
		return false
	}
	// Both have pre-release suffixes — compare RC numbers if
	// applicable, otherwise fall back to lexicographic order.
	vRC, bRC := v.rcNumber(), b.rcNumber()
	if vRC >= 0 && bRC >= 0 {
		return vRC > bRC
	}
	return v.Pre > b.Pre
}

func (v version) Equal(b version) bool {
	return v.Major == b.Major && v.Minor == b.Minor && v.Patch == b.Patch && v.Pre == b.Pre
}

// baseEqual returns true when two versions share the same
// major.minor.patch triple, ignoring the pre-release suffix.
func (v version) baseEqual(b version) bool {
	return v.Major == b.Major && v.Minor == b.Minor && v.Patch == b.Patch
}

// filterStable returns only non-prerelease versions.
func filterStable(tags []version) []version {
	var out []version
	for _, t := range tags {
		if !t.IsPrerelease() {
			out = append(out, t)
		}
	}
	return out
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
