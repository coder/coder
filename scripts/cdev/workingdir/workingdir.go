package workingdir

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

type Paths struct {
	current string
	repo    string
}

func (p Paths) Root() string {
	return p.repo
}

func WorkingContext(next serpent.HandlerFunc) serpent.HandlerFunc {
	return func(inv *serpent.Invocation) error {
		current, err := os.Getwd()
		if err != nil {
			return xerrors.Errorf("get working directory: %w", err)
		}

		repoRoot, err := GitRepoRoot(current)
		if err != nil {
			return xerrors.Errorf("get git repo root: %w", err)
		}

		moduleName, err := GoModuleName(repoRoot)
		if err != nil {
			return xerrors.Errorf("get go module name: %w", err)
		}

		if strings.TrimSpace(moduleName) != "github.com/coder/coder/v2" {
			return xerrors.New("this executable must be called within a directory of the coderd repo")
		}

		inv = inv.WithContext(WithPaths(inv.Context(), Paths{
			current: current,
			repo:    repoRoot,
		}))

		return next(inv)
	}
}

type workingDirKey struct{}

func From(ctx context.Context) Paths {
	return ctx.Value(workingDirKey{}).(Paths)
}

func WithPaths(ctx context.Context, p Paths) context.Context {
	return context.WithValue(ctx, workingDirKey{}, p)
}

// GitRepoRoot returns the root directory of the git repository containing the
// given directory. If dir is empty, it uses the current working directory.
func GitRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GoModuleName returns the Go module name for the given directory.
// If dir is empty, it uses the current working directory.
func GoModuleName(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("go list -m: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
