package v2

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// version represents a parsed semantic version with optional RC
// suffix. When rc < 0 the version is a final release. The original
// field preserves the string that was parsed (including the leading
// "v").
type version struct {
	major    int
	minor    int
	patch    int
	rc       int // -1 means not an RC
	original string
}

// String returns the canonical version string (e.g. "v2.21.0" or
// "v2.21.0-rc.3").
func (v version) String() string {
	if v.rc >= 0 {
		return fmt.Sprintf("v%d.%d.%d-rc.%d", v.major, v.minor, v.patch, v.rc)
	}
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

// IsRC returns true if this is a release candidate.
func (v version) IsRC() bool {
	return v.rc >= 0
}

// semverRe matches vMAJOR.MINOR.PATCH with optional -rc.N suffix.
var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$`)

// parseVersion parses a version string like "v2.21.0" or
// "v2.21.0-rc.3".
func parseVersion(s string) (version, error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return version{}, xerrors.Errorf("invalid version %q", s)
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	rc := -1
	if m[4] != "" {
		rc, _ = strconv.Atoi(m[4])
	}

	// Preserve the original string with leading "v".
	orig := s
	if !strings.HasPrefix(orig, "v") {
		orig = "v" + orig
	}

	return version{
		major:    major,
		minor:    minor,
		patch:    patch,
		rc:       rc,
		original: orig,
	}, nil
}
