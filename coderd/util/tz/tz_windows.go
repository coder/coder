// go:build windows

package tz

import (
	"fmt"
	"os"
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
	if tzEnv, found := os.LookupEnv("TZ"); found {
		if tzEnv == "" {
			return time.UTC, nil
		}
		loc, err := time.LoadLocation(tzEnv)
		if err != nil {
			return nil, xerrors.Errorf("load location from TZ env: %w", err)
		}
		return loc, nil
	}

	// https://superuser.com/a/1584968
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, xerrors.Errorf("run powershell: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer stdin.Close()
		defer close(done)
		_, _ = fmt.Fprintln(stdin, cmdTimezone)
	}()

	<-done

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
	loc, err := time.LoadLocation(locStr)
	if err != nil {
		return nil, xerrors.Errorf("invalid location %q from powershell: %w", locStr, err)
	}

	return loc, nil
}
