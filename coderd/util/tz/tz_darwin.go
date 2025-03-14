//go:build darwin

package tz
import (

	"fmt"
	"errors"
	"path/filepath"
	"strings"
	"time"

)
const (
	etcLocaltime = "/etc/localtime"

	zoneInfoPath = "/var/db/timezone/zoneinfo/"
)
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
		return nil, fmt.Errorf("lookup timezone from env: %w", err)
	}
	lp, err := filepath.EvalSymlinks(etcLocaltime)
	if err != nil {
		return nil, fmt.Errorf("read location of %s: %w", etcLocaltime, err)
	}

	// On Darwin, /var/db/timezone/zoneinfo is also a symlink
	realZoneInfoPath, err := filepath.EvalSymlinks(zoneInfoPath)
	if err != nil {
		return nil, fmt.Errorf("read location of %s: %w", zoneInfoPath, err)
	}

	stripped := strings.Replace(lp, realZoneInfoPath, "", -1)
	stripped = strings.TrimPrefix(stripped, string(filepath.Separator))
	loc, err = time.LoadLocation(stripped)
	if err != nil {
		return nil, fmt.Errorf("invalid location %q guessed from %s: %w", stripped, lp, err)
	}

	return loc, nil
}
