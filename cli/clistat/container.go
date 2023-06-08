//go:build !linux

package clistat

// IsContainerized returns whether the host is containerized.
// This is adapted from https://github.com/elastic/go-sysinfo/tree/main/providers/linux/container.go#L31
// with modifications to support Sysbox containers.
// On non-Linux platforms, it always returns false.
func IsContainerized() (bool, error) {
	return false, nil
}
