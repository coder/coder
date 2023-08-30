package agentproc

import (
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

const DefaultProcDir = "/proc"

type Syscaller interface {
	SetPriority(pid int32, priority int) error
}

type UnixSyscaller struct{}

func (UnixSyscaller) SetPriority(pid int32, nice int) error {
	err := unix.Setpriority(unix.PRIO_PROCESS, int(pid), nice)
	if err != nil {
		return xerrors.Errorf("set priority: %w", err)
	}
	return nil
}

type Process struct {
	Dir     string
	CmdLine string
	PID     int32
	fs      afero.Fs
}

func (p *Process) SetOOMAdj(score int) error {
	path := filepath.Join(p.Dir, "oom_score_adj")
	err := afero.WriteFile(p.fs,
		path,
		[]byte(strconv.Itoa(score)),
		0644,
	)
	if err != nil {
		return xerrors.Errorf("write %q: %w", path, err)
	}

	return nil
}

func (p *Process) SetNiceness(sc Syscaller, score int) error {
	err := sc.SetPriority(p.PID, score)
	if err != nil {
		return xerrors.Errorf("set priority for %q: %w", p.CmdLine, err)
	}
	return nil
}

func (p *Process) Name() string {
	args := strings.Split(p.CmdLine, "\x00")
	// Split will always return at least one element.
	return args[0]
}

func List(fs afero.Fs, dir string) ([]*Process, error) {
	d, err := fs.Open(dir)
	if err != nil {
		return nil, xerrors.Errorf("open dir %q: %w", dir, err)
	}

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
		cmdline, err := afero.ReadFile(fs, filepath.Join(dir, entry, "cmdline"))
		if err != nil {
			var errNo syscall.Errno
			if xerrors.As(err, &errNo) && errNo == syscall.EPERM {
				continue
			}
			return nil, xerrors.Errorf("read cmdline: %w", err)
		}
		processes = append(processes, &Process{
			PID:     int32(pid),
			CmdLine: string(cmdline),
			Dir:     filepath.Join(dir, entry),
			fs:      fs,
		})
	}

	return processes, nil
}
