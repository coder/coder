//go:build linux
// +build linux

package agentproc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

func List(fs afero.Fs, syscaller Syscaller) ([]*Process, error) {
	d, err := fs.Open(defaultProcDir)
	if err != nil {
		return nil, xerrors.Errorf("open dir %q: %w", defaultProcDir, err)
	}
	defer d.Close()

	entries, err := d.Readdirnames(0)
	if err != nil {
		return nil, xerrors.Errorf("readdirnames: %w", err)
	}

	processes := make([]*Process, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.ParseInt(entry, 10, 32)
		if err != nil {
			continue
		}

		// Check that the process still exists.
		exists, err := isProcessExist(syscaller, int32(pid))
		if err != nil {
			return nil, xerrors.Errorf("check process exists: %w", err)
		}
		if !exists {
			continue
		}

		cmdline, err := afero.ReadFile(fs, filepath.Join(defaultProcDir, entry, "cmdline"))
		if err != nil {
			var errNo syscall.Errno
			if xerrors.As(err, &errNo) && errNo == syscall.EPERM {
				continue
			}
			return nil, xerrors.Errorf("read cmdline: %w", err)
		}

		oomScore, err := afero.ReadFile(fs, filepath.Join(defaultProcDir, entry, "oom_score_adj"))
		if err != nil {
			if xerrors.Is(err, os.ErrPermission) {
				continue
			}

			return nil, xerrors.Errorf("read oom_score_adj: %w", err)
		}

		oom, err := strconv.Atoi(strings.TrimSpace(string(oomScore)))
		if err != nil {
			return nil, xerrors.Errorf("convert oom score: %w", err)
		}

		processes = append(processes, &Process{
			PID:         int32(pid),
			CmdLine:     string(cmdline),
			Dir:         filepath.Join(defaultProcDir, entry),
			OOMScoreAdj: oom,
		})
	}

	return processes, nil
}

func isProcessExist(syscaller Syscaller, pid int32) (bool, error) {
	err := syscaller.Kill(pid, syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if err.Error() == "os: process already finished" {
		return false, nil
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false, err
	}

	switch errno {
	case syscall.ESRCH:
		return false, nil
	case syscall.EPERM:
		return true, nil
	}

	return false, xerrors.Errorf("kill: %w", err)
}

func (p *Process) Niceness(sc Syscaller) (int, error) {
	nice, err := sc.GetPriority(p.PID)
	if err != nil {
		return 0, xerrors.Errorf("get priority for %q: %w", p.CmdLine, err)
	}
	return nice, nil
}

func (p *Process) SetNiceness(sc Syscaller, score int) error {
	err := sc.SetPriority(p.PID, score)
	if err != nil {
		return xerrors.Errorf("set priority for %q: %w", p.CmdLine, err)
	}
	return nil
}

func (p *Process) Cmd() string {
	return strings.Join(p.cmdLine(), " ")
}

func (p *Process) cmdLine() []string {
	return strings.Split(p.CmdLine, "\x00")
}
