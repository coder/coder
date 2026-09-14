// Package tz includes utilities for cross-platform timezone/location detection.
package tz

import (
	"os"
	"time"

	"golang.org/x/xerrors"
)

var errNoEnvSet = xerrors.New("no env set")

func locationFromEnv() (*time.Location, error) {
	tzEnv, found := os.LookupEnv("TZ")
	if !found {
		return nil, errNoEnvSet
	}

	// TZ set but empty means UTC.
	if tzEnv == "" {
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(tzEnv)
	if err != nil {
		return nil, xerrors.Errorf("load location from TZ env: %w", err)
	}

	return loc, nil
}
