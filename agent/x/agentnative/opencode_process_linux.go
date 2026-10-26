//go:build linux

package agentnative

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func openCodePrivateDirectory(path string, mode openCodeDirectoryMode) error {
	if mode == openCodeCreateDirectory {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return errOpenCodeOwnership
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || int64(stat.Uid) != int64(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 {
		return errOpenCodeOwnership
	}
	return nil
}

func openCodePrivateFile(path string, flags int) (*os.File, error) {
	file, err := os.OpenFile(path, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, errOpenCodeOwnership
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || int64(stat.Uid) != int64(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, errOpenCodeOwnership
	}
	return file, nil
}

func openCodeLockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func openCodeUnlockFile(file *os.File) error {
	// A concurrent fork can retain the open file description until exec.
	// Explicitly unlock it rather than waiting for every descriptor to close.
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func openCodeReadProc(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return "", errOpenCodeOwnership
	}
	return string(data), nil
}

func openCodeProcessDomain() (string, error) {
	boot, err := openCodeReadProc("/proc/sys/kernel/random/boot_id", 128)
	if err != nil {
		return "", errOpenCodeOwnership
	}
	namespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return "", errOpenCodeOwnership
	}
	return strings.TrimSpace(boot) + "/" + namespace, nil
}

func openCodeProcStat(pid int) (string, bool, error) {
	text, err := openCodeReadProc(filepath.Join("/proc", strconv.Itoa(pid), "stat"), 4096)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ESRCH) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errOpenCodeOwnership
	}
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return "", false, errOpenCodeOwnership
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 20 {
		return "", false, errOpenCodeOwnership
	}
	if fields[0] == "Z" || fields[0] == "X" {
		return "", false, nil
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", false, errOpenCodeOwnership
	}
	return fields[19], true, nil
}

func openCodeProcessIdentity(pid int) (*openCodeProcess, error) {
	start, alive, err := openCodeProcStat(pid)
	if err != nil || !alive {
		return nil, err
	}
	process, err := openCodeProcessDetails(pid, start)
	if err == nil {
		return process, nil
	}
	// A missing environment or FD alone is not evidence of death. Confirm with
	// the process stat record before treating a descriptor as stale.
	_, alive, statErr := openCodeProcStat(pid)
	if statErr == nil && !alive {
		return nil, nil //nolint:nilnil // Confirmed process death is distinct from an inspection failure.
	}
	return nil, errOpenCodeOwnership
}

func openCodeProcessDetails(pid int, start string) (*openCodeProcess, error) {
	root := filepath.Join("/proc", strconv.Itoa(pid))
	environment, err := openCodeReadProc(filepath.Join(root, "environ"), openCodeMaxResponse)
	if err != nil {
		return nil, err
	}
	process := &openCodeProcess{StartTime: start}
	for entry := range strings.SplitSeq(environment, "\x00") {
		if marker, ok := strings.CutPrefix(entry, openCodeMarker+"="); ok {
			process.Marker = marker
		}
	}
	directory, err := os.Open(filepath.Join(root, "fd"))
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(65537)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 65536 {
		return nil, errOpenCodeOwnership
	}
	sockets := map[string]bool{}
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(root, "fd", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sockets[link] = true
	}
	tcp, err := openCodeReadProc(filepath.Join(root, "net", "tcp"), openCodeMaxResponse)
	if err != nil {
		return nil, err
	}
	for line := range strings.SplitSeq(tcp, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		address, port, ok := strings.Cut(fields[1], ":")
		if !ok || address != "0100007F" || !sockets["socket:["+fields[9]+"]"] {
			continue
		}
		value, err := strconv.ParseUint(port, 16, 16)
		if err != nil {
			return nil, errOpenCodeOwnership
		}
		process.Ports = append(process.Ports, int(value))
	}
	// Guard a concurrent exec/PID turnover while reading several proc files.
	next, alive, err := openCodeProcStat(pid)
	if err != nil || !alive || next != start {
		return nil, errOpenCodeOwnership
	}
	return process, nil
}

func openCodePrepareCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}

func openCodeSignal(d openCodeDescriptor) error {
	// A pidfd prevents a verified PID from being recycled before SIGTERM. On
	// kernels without pidfds, fail closed instead of signaling a numeric PID.
	fd, err := unix.PidfdOpen(d.PID, 0)
	if err != nil {
		return errOpenCodeOwnership
	}
	defer unix.Close(fd)
	domain, err := openCodeProcessDomain()
	if err != nil || domain != d.Domain {
		return errOpenCodeOwnership
	}
	process, err := openCodeProcessIdentity(d.PID)
	if err != nil || !openCodeOwnsListener(d, process) {
		return errOpenCodeOwnership
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return errOpenCodeOwnership
	}
	return nil
}
