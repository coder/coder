package clistat

import (
	"bufio"
	"bytes"
	"os"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

const (
	procMounts                           = "/proc/mounts"
	procOneCgroup                        = "/proc/1/cgroup"
	sysCgroupType                        = "/sys/fs/cgroup/cgroup.type"
	kubernetesDefaultServiceAccountToken = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec
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

	// Sometimes the above method of sniffing /proc/1/cgroup isn't reliable.
	// If a Kubernetes service account token is present, that's
	// also a good indication that we are in a container.
	_, err = afero.ReadFile(fs, kubernetesDefaultServiceAccountToken)
	if err == nil {
		return true, nil
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

	// Adapted from https://github.com/systemd/systemd/blob/88bbf187a9b2ebe0732caa1e886616ae5f8186da/src/basic/virt.c#L603-L605
	// The file `/sys/fs/cgroup/cgroup.type` does not exist on the root cgroup.
	// If this file exists we can be sure we're in a container.
	cgTypeExists, err := afero.Exists(fs, sysCgroupType)
	if err != nil {
		return false, xerrors.Errorf("check file exists %s: %w", sysCgroupType, err)
	}
	if cgTypeExists {
		return true, nil
	}

	// If we get here, we are _probably_ not running in a container.
	return false, nil
}
