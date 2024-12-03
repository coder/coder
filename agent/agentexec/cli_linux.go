//go:build linux
// +build linux

package agentexec

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

// unset is set to an invalid value for nice and oom scores.
const unset = -2000

// CLI runs the agent-exec command. It should only be called by the cli package.
func CLI() error {
	// We lock the OS thread here to avoid a race condition where the nice priority
	// we set gets applied to a different thread than the one we exec the provided
	// command on.
	runtime.LockOSThread()
	// Nop on success but we do it anyway in case of an error.
	defer runtime.UnlockOSThread()

	var (
		fs   = flag.NewFlagSet("agent-exec", flag.ExitOnError)
		nice = fs.Int("coder-nice", unset, "")
		oom  = fs.Int("coder-oom", unset, "")
	)

	if len(os.Args) < 3 {
		return xerrors.Errorf("malformed command %+v", os.Args)
	}

	// Parse everything after "coder agent-exec".
	err := fs.Parse(os.Args[2:])
	if err != nil {
		return xerrors.Errorf("parse flags: %w", err)
	}

	// Get everything after "coder agent-exec --"
	args := execArgs(os.Args)
	if len(args) == 0 {
		return xerrors.Errorf("no exec command provided %+v", os.Args)
	}

	if *nice == unset {
		// If an explicit nice score isn't set, we use the default.
		*nice, err = defaultNiceScore()
		if err != nil {
			return xerrors.Errorf("get default nice score: %w", err)
		}
	}

	if *oom == unset {
		// If an explicit oom score isn't set, we use the default.
		*oom, err = defaultOOMScore()
		if err != nil {
			return xerrors.Errorf("get default oom score: %w", err)
		}
	}

	err = unix.Setpriority(unix.PRIO_PROCESS, 0, *nice)
	if err != nil {
		// We alert the user instead of failing the command since it can be difficult to debug
		// for a template admin otherwise. It's quite possible (and easy) to set an
		// inappriopriate value for niceness.
		printfStdErr("failed to adjust niceness to %d for cmd %+v: %v", *nice, args, err)
	}

	err = writeOOMScoreAdj(*oom)
	if err != nil {
		// We alert the user instead of failing the command since it can be difficult to debug
		// for a template admin otherwise. It's quite possible (and easy) to set an
		// inappriopriate value for oom_score_adj.
		printfStdErr("failed to adjust oom score to %d for cmd %+v: %v", *oom, args, err)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return xerrors.Errorf("look path: %w", err)
	}

	return syscall.Exec(path, args, os.Environ())
}

func defaultNiceScore() (int, error) {
	score, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
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
	score, err := oomScoreAdj()
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

func oomScoreAdj() (int, error) {
	scoreStr, err := os.ReadFile("/proc/self/oom_score_adj")
	if err != nil {
		return 0, xerrors.Errorf("read oom_score_adj: %w", err)
	}
	return strconv.Atoi(strings.TrimSpace(string(scoreStr)))
}

func writeOOMScoreAdj(score int) error {
	return os.WriteFile(fmt.Sprintf("/proc/%d/oom_score_adj", os.Getpid()), []byte(fmt.Sprintf("%d", score)), 0o600)
}

// execArgs returns the arguments to pass to syscall.Exec after the "--" delimiter.
func execArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func printfStdErr(format string, a ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "coder-agent: %s\n", fmt.Sprintf(format, a...))
}
