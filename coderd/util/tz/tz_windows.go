// go:build windows

package tz

import (
	"errors"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// cmdTimezone is a Powershell incantation that will return the system
// time location in IANA format.
const cmdTimezone = "[Windows.Globalization.Calendar,Windows.Globalization,ContentType=WindowsRuntime]::New().GetTimeZone()"

// TimezoneIANA attempts to determine the local timezone in IANA format.
// If the TZ environment variable is set, this is used.
// Otherwise, /etc/localtime is used to determine the timezone.
// Reference: https://stackoverflow.com/a/63805394
// On Windows platforms, instead of reading /etc/localtime, powershell
// is used instead to get the current time location in IANA format.
// Reference: https://superuser.com/a/1584968
func TimezoneIANA() (*time.Location, error) {
	loc, err := locationFromEnv()
	if err == nil {
		return loc, nil
	}
	if !errors.Is(err, errNoEnvSet) {
		return nil, xerrors.Errorf("lookup timezone from env: %w", err)
	}

	// https://superuser.com/a/1584968
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive")
	// Powershell echoes its stdin so write a newline
	cmd.Stdin = strings.NewReader(cmdTimezone + "\n")

	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		return nil, xerrors.Errorf("execute powershell command %q: %w", cmdTimezone, err)
	}

	outLines := strings.Split(string(outBytes), "\n")
	if len(outLines) < 2 {
		return nil, xerrors.Errorf("unexpected output from powershell command %q: %q", cmdTimezone, outLines)
	}
	// What we want is the second line of output
	locStr := strings.TrimSpace(outLines[1])
	loc, err = time.LoadLocation(locStr)
	if err != nil {
		return nil, xerrors.Errorf("invalid location %q from powershell: %w", locStr, err)
	}

	return loc, nil
}
