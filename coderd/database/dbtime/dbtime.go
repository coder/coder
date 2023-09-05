package dbtime

import "time"

// Now returns a standardized timezone used for database resources.
func Now() time.Time {
	return Time(time.Now().UTC())
}

// Time returns a time compatible with Postgres. Postgres only stores dates with
// microsecond precision.
func Time(t time.Time) time.Time {
	return t.Round(time.Microsecond)
}
