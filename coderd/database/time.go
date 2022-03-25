package database

import "time"

// Now returns a standardized timezone used for database resources.
func Now() time.Time {
	return time.Now().UTC()
}
