package tailnet

import (
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

const (
	CurrentMajor = 2
	CurrentMinor = 0
)

var SupportedMajors = []int{2, 1}

func ValidateVersion(version string) error {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return xerrors.Errorf("invalid version string: %s", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return xerrors.Errorf("invalid major version: %s", version)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return xerrors.Errorf("invalid minor version: %s", version)
	}
	if major > CurrentMajor {
		return xerrors.Errorf("server is at version %d.%d, behind requested version %s",
			CurrentMajor, CurrentMinor, version)
	}
	if major == CurrentMajor {
		if minor > CurrentMinor {
			return xerrors.Errorf("server is at version %d.%d, behind requested version %s",
				CurrentMajor, CurrentMinor, version)
		}
		return nil
	}
	for _, mjr := range SupportedMajors {
		if major == mjr {
			return nil
		}
	}
	return xerrors.Errorf("version %s is no longer supported", version)
}
