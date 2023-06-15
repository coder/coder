package clistat

import (
	"bufio"
	"bytes"
	"os"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

const (
	procMounts    = "/proc/mounts"
	procOneCgroup = "/proc/1/cgroup"
)

// IsContainerized returns whether the host is containerized.
// This is adapted from https://github.com/elastic/go-sysinfo/tree/main/providers/linux/container.go#L31
// with modifications to support Sysbox containers.
// On non-Linux platforms, it always returns false.
func IsContainerized(fs afero.Fs) (ok bool, err error) {
	cgData, err := afero.ReadFile(fs, procOneCgroup)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, xerrors.Errorf("read file %s: %w", procOneCgroup, err)
	}

	scn := bufio.NewScanner(bytes.NewReader(cgData))
	for scn.Scan() {
		line := scn.Bytes()
		if bytes.Contains(line, []byte("docker")) ||
			bytes.Contains(line, []byte(".slice")) ||
			bytes.Contains(line, []byte("lxc")) ||
			bytes.Contains(line, []byte("kubepods")) {
			return true, nil
		}
	}

	// Last-ditch effort to detect Sysbox containers.
	// Check if we have anything mounted as type sysboxfs in /proc/mounts
	mountsData, err := afero.ReadFile(fs, procMounts)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, xerrors.Errorf("read file %s: %w", procMounts, err)
	}

	scn = bufio.NewScanner(bytes.NewReader(mountsData))
	for scn.Scan() {
		line := scn.Bytes()
		if bytes.Contains(line, []byte("sysboxfs")) {
			return true, nil
		}
	}

	// If we get here, we are _probably_ not running in a container.
	return false, nil
}
