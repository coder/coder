package cliutil

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

var (
	hostname     string
	hostnameOnce sync.Once
)

// Hostname returns the hostname of the machine, lowercased,
// with any trailing domain suffix stripped.
// It is cached after the first call.
// If the hostname cannot be determined, this will panic.
func Hostname() string {
	hostnameOnce.Do(func() { hostname = getHostname() })
	return hostname
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		// Something must be very wrong if this fails.
		panic(xerrors.Errorf("lookup hostname: %w", err))
	}

	// On some platforms, the hostname can be an FQDN. We only want the hostname.
	if idx := strings.Index(h, "."); idx != -1 {
		h = h[:idx]
	}

	// For the sake of consistency, we also want to lowercase the hostname.
	// Per RFC 4343, DNS lookups must be case-insensitive.
	return strings.ToLower(h)
}
