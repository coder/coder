package clistat

import (
	"bufio"
	"bytes"
	"os"
	"sync"

	"go.uber.org/atomic"
	"golang.org/x/xerrors"
)

var isContainerizedCacheOK atomic.Bool
var isContainerizedCacheErr atomic.Error
var isContainerizedCacheOnce sync.Once

// IsContainerized returns whether the host is containerized.
// This is adapted from https://github.com/elastic/go-sysinfo/tree/main/providers/linux/container.go#L31
// with modifications to support Sysbox containers.
// On non-Linux platforms, it always returns false.
// The result is only computed once and stored for subsequent calls.
func IsContainerized() (ok bool, err error) {
	isContainerizedCacheOnce.Do(func() {
		ok, err = isContainerizedOnce()
		isContainerizedCacheOK.Store(ok)
		isContainerizedCacheErr.Store(err)
	})
	return isContainerizedCacheOK.Load(), isContainerizedCacheErr.Load()
}

func isContainerizedOnce() (bool, error) {
	data, err := os.ReadFile(procOneCgroup)
	if err != nil {
		if os.IsNotExist(err) { // how?
			return false, nil
		}
		return false, xerrors.Errorf("read process cgroups: %w", err)
	}

	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := s.Bytes()
		if bytes.Contains(line, []byte("docker")) ||
			bytes.Contains(line, []byte(".slice")) ||
			bytes.Contains(line, []byte("lxc")) ||
			bytes.Contains(line, []byte("kubepods")) {
			return true, nil
		}
	}

	// Last-ditch effort to detect Sysbox containers.
	// Check if we have anything mounted as type sysboxfs in /proc/mounts
	data, err = os.ReadFile("/proc/mounts")
	if err != nil {
		return false, xerrors.Errorf("read /proc/mounts: %w", err)
	}

	s = bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := s.Bytes()
		if bytes.HasPrefix(line, []byte("sysboxfs")) {
			return true, nil
		}
	}

	// If we get here, we are _probably_ not running in a container.
	return false, nil
}
