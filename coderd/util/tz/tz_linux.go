//go:build linux

package tz

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const etcLocaltime = "/etc/localtime"
const zoneInfoPath = "/usr/share/zoneinfo"

// TimezoneIANA attempts to determine the local timezone in IANA format.
// If the TZ environment variable is set, this is used.
// Otherwise, /etc/localtime is used to determine the timezone.
// Reference: https://stackoverflow.com/a/63805394
// On Windows platforms, instead of reading /etc/localtime, powershell
// is used instead to get the current time location in IANA format.
// Reference: https://superuser.com/a/1584968
func TimezoneIANA() (*time.Location, error) {
	if tzEnv, found := os.LookupEnv("TZ"); found {
		// TZ set but empty means UTC.
		if tzEnv == "" {
			return time.UTC, nil
		}
		loc, err := time.LoadLocation(tzEnv)
		if err == nil {
			return loc, nil
		}
	}

	lp, err := filepath.EvalSymlinks(etcLocaltime)
	if err != nil {
		return nil, err
	}

	stripped := strings.Replace(lp, etcLocaltime, "", -1)
	loc, err := time.LoadLocation(stripped)
	if err != nil {
		return nil, err
	}
	return loc, nil
}
