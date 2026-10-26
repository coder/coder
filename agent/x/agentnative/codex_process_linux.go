//go:build linux

package agentnative

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func codexReadProc(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		// Preserve ESRCH when a process exits after open but before read.
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errCodexOwnership
	}
	return data, nil
}

func codexProcessDomain() (bootID, pidNamespace string, err error) {
	boot, err := codexReadProc("/proc/sys/kernel/random/boot_id", 128)
	if err != nil {
		return "", "", errCodexOwnership
	}
	namespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return "", "", errCodexOwnership
	}
	return strings.TrimSpace(string(boot)), namespace, nil
}

// A nil record denotes a process that has exited, not an inspection failure.
//
//nolint:nilnil // Optional process lookup keeps absence distinct from unverifiable identity.
func codexProcStat(pid int) (*codexProcessRecord, error) {
	data, err := codexReadProc(filepath.Join("/proc", strconv.Itoa(pid), "stat"), 4096)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ESRCH) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return nil, errCodexOwnership
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return nil, errCodexOwnership
	}
	if fields[0] == "Z" || fields[0] == "X" {
		return nil, nil
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, errCodexOwnership
	}
	session, err := strconv.Atoi(fields[3])
	if err != nil {
		return nil, errCodexOwnership
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return nil, errCodexOwnership
	}
	return &codexProcessRecord{PID: pid, Group: group, Session: session, Start: fields[19]}, nil
}

func codexProcessIdentity(pid int) (*codexProcessRecord, string, error) {
	identity, err := codexProcStat(pid)
	if err != nil || identity == nil {
		return nil, "", err
	}
	root := filepath.Join("/proc", strconv.Itoa(pid))
	info, err := os.Stat(root)
	if err != nil {
		return codexProcessRace(pid, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Geteuid()) {
		return nil, "", errCodexOwnership
	}
	exe, err := os.Readlink(filepath.Join(root, "exe"))
	if err != nil {
		return codexProcessRace(pid, err)
	}
	exeInfo, err := os.Stat(filepath.Join(root, "exe"))
	if err != nil {
		return codexProcessRace(pid, err)
	}
	exeStat, ok := exeInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, "", errCodexOwnership
	}
	environment, err := codexReadProc(filepath.Join(root, "environ"), 1<<20)
	if err != nil {
		return codexProcessRace(pid, err)
	}
	marker := ""
	for entry := range strings.SplitSeq(string(environment), "\x00") {
		if value, ok := strings.CutPrefix(entry, codexProcessMarker+"="); ok {
			marker = value
		}
	}
	next, err := codexProcStat(pid)
	if err != nil || next == nil {
		return nil, "", err
	}
	if *next != *identity {
		return nil, "", errCodexOwnership
	}
	identity.Executable = exe
	identity.Device = exeStat.Dev
	identity.Inode = exeStat.Ino
	return identity, marker, nil
}

func codexProcessRace(pid int, cause error) (*codexProcessRecord, string, error) {
	identity, err := codexProcStat(pid)
	if err == nil && identity == nil {
		return nil, "", nil
	}
	return nil, "", errors.Join(errCodexOwnership, cause)
}

type codexOwnedProcess struct {
	identity codexProcessRecord
	fd       int
}

// Scan marked members of the dedicated session, including subprocesses which
// create their own process groups. An inherited marker authenticates native
// children after the npm launcher exits. Every signal uses a verified pidfd,
// never a recyclable numeric PID or process-group ID.
func codexOwnedProcesses(record codexServerRecord) ([]codexOwnedProcess, error) {
	boot, namespace, err := codexProcessDomain()
	if err != nil || boot != record.Boot || namespace != record.Namespace {
		return nil, errCodexOwnership
	}
	entries, err := os.ReadDir("/proc")
	if err != nil || len(entries) > 65536 {
		return nil, errCodexOwnership
	}
	var result []codexOwnedProcess
	fail := func(err error) ([]codexOwnedProcess, error) { codexCloseProcesses(result); return nil, err }
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue
		}
		identity, err := codexProcStat(pid)
		if err != nil {
			return fail(errCodexOwnership)
		}
		if identity == nil || identity.Session != record.Leader.Session {
			continue
		}
		fd, err := unix.PidfdOpen(pid, 0)
		if errors.Is(err, unix.ESRCH) {
			continue
		}
		if err != nil {
			return fail(errCodexOwnership)
		}
		current, marker, err := codexProcessIdentity(pid)
		pollFD, pollErr := codexPollDescriptor(fd)
		if pollErr != nil {
			_ = unix.Close(fd)
			return fail(pollErr)
		}
		poll := []unix.PollFd{pollFD}
		ready, pollErr := unix.Poll(poll, 0)
		if (current == nil && err == nil) || (pollErr == nil && ready > 0 && poll[0].Revents&unix.POLLIN != 0) {
			_ = unix.Close(fd)
			continue
		}
		if err != nil || marker != record.Marker || current.Start != identity.Start || current.Session != record.Leader.Session || (pid == record.Leader.PID && *current != record.Leader) {
			_ = unix.Close(fd)
			if pid == record.Leader.PID {
				return fail(errCodexOwnership)
			}
			// MCP servers and tools may scrub the inherited environment. Session
			// membership alone grants no authority over them, but their presence
			// must not revoke our independent authority over the native owner.
			continue
		}
		result = append(result, codexOwnedProcess{identity: *current, fd: fd})
	}
	return result, nil
}

func codexPollDescriptor(fd int) (unix.PollFd, error) {
	if fd < 0 || fd > math.MaxInt32 {
		return unix.PollFd{}, errCodexOwnership
	}
	return unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}, nil
}

func codexCloseProcesses(processes []codexOwnedProcess) {
	for _, process := range processes {
		_ = unix.Close(process.fd)
	}
}

func codexSignalProcesses(processes []codexOwnedProcess, signal unix.Signal) error {
	for _, process := range processes {
		if err := unix.PidfdSendSignal(process.fd, signal, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
			return errCodexOwnership
		}
	}
	return nil
}
