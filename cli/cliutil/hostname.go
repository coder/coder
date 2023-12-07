package cliutil

import (
	"os"
	"sync"

	"golang.org/x/xerrors"
)

var (
	hostname     string
	hostnameOnce sync.Once
)

func Hostname() string {
	hostnameOnce.Do(func() {
		h, err := os.Hostname()
		if err != nil {
			// Something must be very wrong if this fails.
			panic(xerrors.Errorf("lookup hostname: %w", err))
		}
		hostname = h
	})
	return hostname
}
