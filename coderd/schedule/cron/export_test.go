package cron

import (
	"strings"
	"time"
)

// StandardIn is Standard for a time zone that the test loaded, so that tests
// can use zone data other than the host's. The spec must be valid.
func StandardIn(spec string, loc *time.Location) (*Schedule, error) {
	return standardIn(strings.Fields(spec), loc)
}
