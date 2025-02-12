package dbtime

import "time"

// Now returns a standardized timezone used for database resources.
func Now() time.Time {
	return Time(time.Now().UTC())
}

// Time returns a time compatible with Postgres. Postgres only stores dates with
// microsecond precision.
// FIXME(dannyk): refactor all calls to Time() to expect the input time to be modified to UTC; there are currently a
//
//	few calls whose behavior would change subtly.
//	See https://github.com/coder/coder/pull/14274#discussion_r1718427461
func Time(t time.Time) time.Time {
	return t.Round(time.Microsecond)
}

// StartOfDay returns the first timestamp of the day of the input timestamp in its location.
func StartOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
