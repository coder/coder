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

const netAdminCapabilityMask = uint64(1) << unix.CAP_NET_ADMIN

// lockdownNetAdmin prevents children of the agent from acquiring
// CAP_NET_ADMIN while retaining the agent's effective and permitted sets for
// transparent reply sockets. Processes not spawned by the agent, including a
// container entrypoint or pre-existing sessions, are outside this lockdown.
// The reference deployment runs the agent as the only privileged entrypoint.
func lockdownNetAdmin() error {
	var data [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if err := unix.Capget(&header, &data[0]); err != nil {
		return xerrors.Errorf("read capabilities: %w", err)
	}
	index := unix.CAP_NET_ADMIN / 32
	bit := uint32(1) << (unix.CAP_NET_ADMIN % 32)
	data[index].Inheritable &^= bit
	if _, _, err := syscall.AllThreadsSyscall(
		syscall.SYS_CAPSET,
		uintptr(unsafe.Pointer(&header)),
		uintptr(unsafe.Pointer(&data[0])),
		0,
	); err != 0 {
		return allThreadsError("drop CAP_NET_ADMIN from inheritable set", err)
	}
	if _, _, err := syscall.AllThreadsSyscall(
		syscall.SYS_PRCTL,
		unix.PR_CAP_AMBIENT,
		unix.PR_CAP_AMBIENT_CLEAR_ALL,
		0,
	); err != 0 {
		return allThreadsError("clear ambient capabilities", err)
	}
	if _, _, err := syscall.AllThreadsSyscall(
		syscall.SYS_PRCTL,
		unix.PR_CAPBSET_DROP,
		unix.CAP_NET_ADMIN,
		0,
	); err != 0 {
		return allThreadsError("drop CAP_NET_ADMIN from bounding set", err)
	}
	if err := verifyNetAdminLockdown("/proc/self/task"); err != nil {
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

func verifyNetAdminLockdown(taskDir string) error {
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
			if value&netAdminCapabilityMask != 0 {
				return xerrors.Errorf("%s still has CAP_NET_ADMIN in %s", statusPath, name)
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
