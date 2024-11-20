//go:build linux
// +build linux

package agentexec

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

// CLI runs the agent-exec command. It should only be called by the cli package.
func CLI(args []string, environ []string) error {
	// We lock the OS thread here to avoid a race conditino where the nice priority
	// we get is on a different thread from the one we set it on.
	runtime.LockOSThread()

	nice := flag.Int(niceArg, 0, "")
	oom := flag.Int(oomArg, 0, "")

	flag.Parse()

	if runtime.GOOS != "linux" {
		return xerrors.Errorf("agent-exec is only supported on Linux")
	}

	if len(args) < 2 {
		return xerrors.Errorf("malformed command %q", args)
	}

	// Slice off 'coder agent-exec'
	args = args[2:]

	pid := os.Getpid()

	var err error
	if nice == nil {
		// If an explicit nice score isn't set, we use the default.
		n, err := defaultNiceScore()
		if err != nil {
			return xerrors.Errorf("get default nice score: %w", err)
		}
		nice = &n
	}

	if oom == nil {
		// If an explicit oom score isn't set, we use the default.
		o, err := defaultOOMScore()
		if err != nil {
			return xerrors.Errorf("get default oom score: %w", err)
		}
		oom = &o
	}

	err = unix.Setpriority(unix.PRIO_PROCESS, 0, *nice)
	if err != nil {
		return xerrors.Errorf("set nice score: %w", err)
	}

	err = writeOOMScoreAdj(pid, *oom)
	if err != nil {
		return xerrors.Errorf("set oom score: %w", err)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return xerrors.Errorf("look path: %w", err)
	}

	// Remove environments variables specifically set for the agent-exec command.
	env := slices.DeleteFunc(environ, func(env string) bool {
		return strings.HasPrefix(env, EnvProcOOMScore) || strings.HasPrefix(env, EnvProcNiceScore)
	})

	return syscall.Exec(path, args, env)
}

func defaultNiceScore() (int, error) {
	score, err := unix.Getpriority(unix.PRIO_PROCESS, os.Getpid())
	if err != nil {
		return 0, xerrors.Errorf("get nice score: %w", err)
	}
	// See https://linux.die.net/man/2/setpriority#Notes
	score = 20 - score

	score += 5
	if score > 19 {
		return 19, nil
	}
	return score, nil
}

func defaultOOMScore() (int, error) {
	score, err := oomScoreAdj(os.Getpid())
	if err != nil {
		return 0, xerrors.Errorf("get oom score: %w", err)
	}

	// If the agent has a negative oom_score_adj, we set the child to 0
	// so it's treated like every other process.
	if score < 0 {
		return 0, nil
	}

	// If the agent is already almost at the maximum then set it to the max.
	if score >= 998 {
		return 1000, nil
	}

	// If the agent oom_score_adj is >=0, we set the child to slightly
	// less than the maximum. If users want a different score they set it
	// directly.
	return 998, nil
}

func oomScoreAdj(pid int) (int, error) {
	scoreStr, err := os.ReadFile(fmt.Sprintf("/proc/%d/oom_score_adj", pid))
	if err != nil {
		return 0, xerrors.Errorf("read oom_score_adj: %w", err)
	}
	return strconv.Atoi(strings.TrimSpace(string(scoreStr)))
}

func writeOOMScoreAdj(pid int, score int) error {
	return os.WriteFile(fmt.Sprintf("/proc/%d/oom_score_adj", pid), []byte(fmt.Sprintf("%d", score)), 0o600)
}
