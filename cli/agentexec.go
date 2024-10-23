package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/xerrors"

	"github.com/spf13/afero"

	"github.com/coder/serpent"
)

const EnvProcOOMScore = "CODER_PROC_OOM_SCORE"
const EnvProcNiceScore = "CODER_PROC_NICE_SCORE"

func (*RootCmd) agentExec() *serpent.Command {
	return &serpent.Command{
		Use:     "agent-exec",
		Hidden:  true,
		RawArgs: true,
		Handler: func(inv *serpent.Invocation) error {
			if runtime.GOOS != "linux" {
				return xerrors.Errorf("agent-exec is only supported on Linux")
			}

			var (
				pid       = os.Getpid()
				args      = inv.Args
				oomScore  = inv.Environ.Get(EnvProcOOMScore)
				niceScore = inv.Environ.Get(EnvProcNiceScore)

				fs        = fsFromContext(inv.Context())
				syscaller = syscallerFromContext(inv.Context())
			)

			score, err := strconv.Atoi(niceScore)
			if err != nil {
				return xerrors.Errorf("invalid nice score: %w", err)
			}

			err = syscaller.Setpriority(syscall.PRIO_PROCESS, pid, score)
			if err != nil {
				return xerrors.Errorf("set nice score: %w", err)
			}

			oomPath := fmt.Sprintf("/proc/%d/oom_score_adj", pid)
			err = afero.WriteFile(fs, oomPath, []byte(oomScore), 0o600)
			if err != nil {
				return xerrors.Errorf("set oom score: %w", err)
			}

			path, err := exec.LookPath(args[0])
			if err != nil {
				return xerrors.Errorf("look path: %w", err)
			}

			env := slices.DeleteFunc(inv.Environ.ToOS(), excludeKeys(EnvProcOOMScore, EnvProcNiceScore))

			return syscall.Exec(path, args, env)
		},
	}
}

func excludeKeys(keys ...string) func(env string) bool {
	return func(env string) bool {
		for _, key := range keys {
			if strings.HasPrefix(env, key+"=") {
				return true
			}
		}
		return false
	}
}

type Syscaller interface {
	Setpriority(int, int, int) error
	Exec(string, []string, []string) error
}

type linuxSyscaller struct{}

func (linuxSyscaller) Setpriority(which, pid, nice int) error {
	return syscall.Setpriority(which, pid, nice)
}

func (linuxSyscaller) Exec(path string, args, env []string) error {
	return syscall.Exec(path, args, env)
}

type syscallerKey struct{}

func WithSyscaller(ctx context.Context, syscaller Syscaller) context.Context {
	return context.WithValue(ctx, syscallerKey{}, syscaller)
}

func syscallerFromContext(ctx context.Context) Syscaller {
	if syscaller, ok := ctx.Value(syscallerKey{}).(Syscaller); ok {
		return syscaller
	}
	return linuxSyscaller{}
}

type fsKey struct{}

func WithFS(ctx context.Context, fs afero.Fs) context.Context {
	return context.WithValue(ctx, fsKey{}, fs)
}

func fsFromContext(ctx context.Context) afero.Fs {
	if fs, ok := ctx.Value(fsKey{}).(afero.Fs); ok {
		return fs
	}
	return afero.NewOsFs()
}
