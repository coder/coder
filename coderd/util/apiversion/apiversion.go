package apiversion

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

func New(maj []int, min int) *APIVersion {
	v := &APIVersion{
		supportedMajors: maj,
		supportedMinor:  min,
	}
	return v
}

type APIVersion struct {
	supportedMajors []int
	supportedMinor  int
}

func (v *APIVersion) Validate(version string) error {
	if len(v.supportedMajors) == 0 {
		return xerrors.Errorf("developer error: SupportedMajors is empty")
	}
	currentMajor := slices.Max(v.supportedMajors)
	major, minor, err := Parse(version)
	if err != nil {
		return err
	}
	if major > currentMajor {
		return xerrors.Errorf("server is at version %d.%d, behind requested major version %s",
			currentMajor, v.supportedMinor, version)
	}
	if major == currentMajor {
		if minor > v.supportedMinor {
			return xerrors.Errorf("server is at version %d.%d, behind requested minor version %s",
				currentMajor, v.supportedMinor, version)
		}
		return nil
	}
	for _, mjr := range v.supportedMajors {
		if major == mjr {
			return nil
		}
	}
	return xerrors.Errorf("version %s is no longer supported", version)
}

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
