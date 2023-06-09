//go:build !linux

package clistat

import "github.com/spf13/afero"

// IsContainerized returns whether the host is containerized.
// This is adapted from https://github.com/elastic/go-sysinfo/tree/main/providers/linux/container.go#L31
// with modifications to support Sysbox containers.
// On non-Linux platforms, it always returns false.
func IsContainerized(_ afero.Fs) (bool, error) {
	return false, nil
}
