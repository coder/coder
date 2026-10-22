package filefinder

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
)

const (
	// maxGitProcs bounds concurrent git subprocesses per Ignorer so
	// bursts of events or listings cannot fork-bomb the workspace.
	maxGitProcs = 4
	// gitListTimeout bounds enumerating a repository's ignored paths.
	gitListTimeout = time.Minute
	// gitCheckTimeout bounds classifying one batch of paths.
	gitCheckTimeout = 10 * time.Second
)

// gitRunner runs git through the agent's execer with bounded
// concurrency and timeouts.
type gitRunner struct {
	execer agentexec.Execer
	bin    string
	procs  chan struct{}
}

func newGitRunner(execer agentexec.Execer, bin string) *gitRunner {
	if bin == "" {
		return nil
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil
	}
	return &gitRunner{execer: execer, bin: bin, procs: make(chan struct{}, maxGitProcs)}
}

func (g *gitRunner) run(ctx context.Context, timeout time.Duration, dir string, stdin []byte, args ...string) ([]byte, error) {
	select {
	case g.procs <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-g.procs }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := g.execer.CommandContext(ctx, g.bin, append([]string{"-C", dir}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, xerrors.Errorf("git %s in %s: %w: %s", args[0], dir, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// listIgnored returns the absolute paths under dir that git ignores.
// Ignored directories are reported once, without their contents, and
// git does not read them. The second result is set when dir itself is
// ignored.
func (g *gitRunner) listIgnored(ctx context.Context, dir string) (map[string]struct{}, bool, error) {
	out, err := g.run(ctx, gitListTimeout, dir, nil,
		"ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, false, err
	}
	ignored := make(map[string]struct{})
	for rel := range strings.SplitSeq(string(out), "\x00") {
		if rel == "" {
			continue
		}
		if rel == "./" {
			return nil, true, nil
		}
		ignored[filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(rel, "/")))] = struct{}{}
	}
	return ignored, false, nil
}

// checkIgnored reports, for each path relative to the repository top,
// whether git ignores it. Directory paths should end in "/".
func (g *gitRunner) checkIgnored(ctx context.Context, top string, rels []string) ([]bool, error) {
	var in bytes.Buffer
	for _, rel := range rels {
		_, _ = in.WriteString(rel)
		_ = in.WriteByte(0)
	}
	out, err := g.run(ctx, gitCheckTimeout, top, in.Bytes(),
		"check-ignore", "--stdin", "-z", "--verbose", "--non-matching")
	// check-ignore exits 1 when no path is ignored; --non-matching
	// still reports every path on stdout.
	if err != nil && !isExitCode(err, 1) {
		return nil, err
	}
	// Each record is source, line number, pattern, and path. A
	// matching negated pattern means the path is not ignored.
	fields := strings.Split(string(out), "\x00")
	byPath := make(map[string]bool, len(rels))
	for i := 0; i+3 < len(fields); i += 4 {
		pattern := fields[i+2]
		byPath[fields[i+3]] = pattern != "" && !strings.HasPrefix(pattern, "!")
	}
	result := make([]bool, len(rels))
	for i, rel := range rels {
		result[i] = byPath[rel]
	}
	return result, nil
}

func isExitCode(err error, code int) bool {
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	return ok && exitErr.ExitCode() == code
}

// excludeFile resolves the repository's info/exclude file, which lives
// outside the worktree for separate git dirs and worktrees.
func (g *gitRunner) excludeFile(ctx context.Context, top string) (string, error) {
	out, err := g.run(ctx, gitCheckTimeout, top, nil, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return "", err
	}
	path := filepath.FromSlash(strings.TrimSpace(string(out)))
	if !filepath.IsAbs(path) {
		path = filepath.Join(top, path)
	}
	return filepath.Clean(path), nil
}
