package apiversion

import (
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// New returns an *APIVersion with the given major.minor and
// additional supported major versions.
func New(maj, min int) *APIVersion {
	v := &APIVersion{
		supportedMajor:   maj,
		supportedMinor:   min,
		additionalMajors: make([]int, 0),
	}
	return v
}

type APIVersion struct {
	supportedMajor   int
	supportedMinor   int
	additionalMajors []int
}

func (v *APIVersion) WithBackwardCompat(majs ...int) *APIVersion {
	v.additionalMajors = append(v.additionalMajors, majs[:]...)
	return v
}

// Validate validates the given version against the given constraints:
// A given major.minor version is valid iff:
//  1. The requested major version is contained within v.supportedMajors
//  2. If the requested major version is the 'current major', then
//     the requested minor version must be less than or equal to the supported
//     minor version.
//
// For example, given majors {1, 2} and minor 2, then:
// - 0.x is not supported,
// - 1.x is supported,
// - 2.0, 2.1, and 2.2 are supported,
// - 2.3+ is not supported.
func (v *APIVersion) Validate(version string) error {
	major, minor, err := Parse(version)
	if err != nil {
		return err
	}
	if major > v.supportedMajor {
		return xerrors.Errorf("server is at version %d.%d, behind requested major version %s",
			v.supportedMajor, v.supportedMinor, version)
	}
	if major == v.supportedMajor {
		if minor > v.supportedMinor {
			return xerrors.Errorf("server is at version %d.%d, behind requested minor version %s",
				v.supportedMajor, v.supportedMinor, version)
		}
		return nil
	}
	for _, mjr := range v.additionalMajors {
		if major == mjr {
			return nil
		}
	}
	return xerrors.Errorf("version %s is no longer supported", version)
}

// Parse parses a valid major.minor version string into (major, minor).
// Both major and minor must be valid integers separated by a period '.'.
func Parse(version string) (major int, minor int, err error) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, 0, xerrors.Errorf("invalid version string: %s", version)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid major version: %s", version)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid minor version: %s", version)
	}
	return major, minor, nil
}
