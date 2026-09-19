//go:build linux

package agentegress

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

const (
	netAdminCapabilityMask = uint64(1) << unix.CAP_NET_ADMIN
	netRawCapabilityMask   = uint64(1) << unix.CAP_NET_RAW
)

var lockedCapabilities = []struct {
	name string
	id   uintptr
	mask uint64
}{
	{name: "CAP_NET_ADMIN", id: unix.CAP_NET_ADMIN, mask: netAdminCapabilityMask},
	{name: "CAP_NET_RAW", id: unix.CAP_NET_RAW, mask: netRawCapabilityMask},
}

// lockdownNetworkCapabilities prevents children of the agent from acquiring
// CAP_NET_ADMIN or CAP_NET_RAW while retaining the agent's effective and
// permitted sets for transparent reply sockets. CAP_NET_RAW is also removed
// because Linux 5.17 and later permits it to set SO_MARK. The agent uses a
// userspace tailnet and does not require raw sockets after startup.
//
// Processes not spawned by the agent, including a container entrypoint or
// pre-existing sessions, are outside this lockdown. The reference deployment
// runs the agent as the only privileged entrypoint.
func lockdownNetworkCapabilities() error {
	var data [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if err := unix.Capget(&header, &data[0]); err != nil {
		return xerrors.Errorf("read capabilities: %w", err)
	}
	for _, capability := range lockedCapabilities {
		index := capability.id / 32
		bit := uint32(1) << (capability.id % 32)
		data[index].Inheritable &^= bit
	}
	if _, _, err := syscall.AllThreadsSyscall(
		syscall.SYS_CAPSET,
		uintptr(unsafe.Pointer(&header)),
		uintptr(unsafe.Pointer(&data[0])),
		0,
	); err != 0 {
		return allThreadsError("drop network capabilities from inheritable set", err)
	}
	if _, _, err := syscall.AllThreadsSyscall(
		syscall.SYS_PRCTL,
		unix.PR_CAP_AMBIENT,
		unix.PR_CAP_AMBIENT_CLEAR_ALL,
		0,
	); err != 0 {
		return allThreadsError("clear ambient capabilities", err)
	}
	for _, capability := range lockedCapabilities {
		if _, _, err := syscall.AllThreadsSyscall(
			syscall.SYS_PRCTL,
			unix.PR_CAPBSET_DROP,
			capability.id,
			0,
		); err != 0 {
			return allThreadsError("drop "+capability.name+" from bounding set", err)
		}
	}
	if err := verifyNetworkCapabilityLockdown("/proc/self/task"); err != nil {
		return xerrors.Errorf("verify capability lockdown: %w", err)
	}
	return nil
}

func allThreadsError(action string, err syscall.Errno) error {
	if err == syscall.ENOTSUP {
		return xerrors.Errorf("%s on all threads: %w (cgo-linked binaries are unsupported)", action, err)
	}
	return xerrors.Errorf("%s on all threads: %w", action, err)
}

func verifyNetworkCapabilityLockdown(taskDir string) error {
	statuses, err := filepath.Glob(filepath.Join(taskDir, "*", "status"))
	if err != nil {
		return xerrors.Errorf("list thread status files: %w", err)
	}
	if len(statuses) == 0 {
		return xerrors.Errorf("no thread status files found in %s", taskDir)
	}
	for _, statusPath := range statuses {
		contents, err := os.ReadFile(statusPath)
		if err != nil {
			return xerrors.Errorf("read %s: %w", statusPath, err)
		}
		caps, err := statusCapabilities(contents)
		if err != nil {
			return xerrors.Errorf("parse %s: %w", statusPath, err)
		}
		for _, name := range []string{"CapBnd", "CapAmb"} {
			value, ok := caps[name]
			if !ok {
				return xerrors.Errorf("%s is missing %s", statusPath, name)
			}
			for _, capability := range lockedCapabilities {
				if value&capability.mask != 0 {
					return xerrors.Errorf("%s still has %s in %s", statusPath, capability.name, name)
				}
			}
		}
	}
	return nil
}

func statusCapabilities(contents []byte) (map[string]uint64, error) {
	caps := make(map[string]uint64, 5)
	for line := range strings.SplitSeq(string(contents), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(name, "Cap") {
			continue
		}
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
		if err != nil {
			return nil, xerrors.Errorf("parse %s: %w", name, err)
		}
		caps[name] = parsed
	}
	return caps, nil
}
