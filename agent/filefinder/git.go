package filefinder

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
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
	// gitWaitDelay bounds waiting for git's output pipes after it is
	// killed, in case a descendant still holds them.
	gitWaitDelay = 2 * time.Second
	// maxGitOutputBytes bounds the stdout read from one git call.
	maxGitOutputBytes = 64 << 20
	// maxGitRecordBytes bounds one NUL-terminated output record.
	maxGitRecordBytes = 64 << 10
	// maxGitStderrBytes bounds the stderr kept for error messages.
	maxGitStderrBytes = 4 << 10
)

// errGitOutputTooLarge reports that git produced more output or
// records than the budget allows.
var errGitOutputTooLarge = xerrors.New("git output exceeds the budget")

// repoSelectingEnv are git environment variables that point git at a
// specific repository, work tree, or index. An agent started from a
// git hook or similar can inherit them; they must not redirect
// classification away from the repository being asked about.
var repoSelectingEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
	"GIT_CEILING_DIRECTORIES", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_PREFIX",
}

// gitRunner runs git through the agent's execer with bounded
// concurrency, time, and output.
type gitRunner struct {
	execer agentexec.Execer
	bin    string
	procs  chan struct{}
	// maxRecords bounds the records one call may return.
	maxRecords int
	// maxBytes bounds the stdout one call may return.
	maxBytes int
}

func newGitRunner(execer agentexec.Execer, bin string) *gitRunner {
	if bin == "" {
		return nil
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil
	}
	return &gitRunner{
		execer:     execer,
		bin:        bin,
		procs:      make(chan struct{}, maxGitProcs),
		maxRecords: maxIndexEntries,
		maxBytes:   maxGitOutputBytes,
	}
}

func gitEnv(environ []string) []string {
	env := make([]string, 0, len(environ)+2)
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if name == "GIT_OPTIONAL_LOCKS" || name == "GIT_TERMINAL_PROMPT" {
			continue
		}
		if !slices.Contains(repoSelectingEnv, name) {
			env = append(env, kv)
		}
	}
	// Classification must not take the index lock or prompt.
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

// run runs git in dir and calls onRecord for each NUL-terminated
// record of its stdout. It stops git and returns errGitOutputTooLarge
// when the output exceeds the runner's budgets, and returns any error
// from onRecord after stopping git.
func (g *gitRunner) run(ctx context.Context, timeout time.Duration, dir string, stdin []byte, onRecord func(string) error, args ...string) error {
	select {
	case g.procs <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.procs }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := g.execer.CommandContext(ctx, g.bin, append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv(cmd.Environ())
	cmd.WaitDelay = gitWaitDelay
	killProcessGroupOnCancel(cmd)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	stderr := &cappedBuffer{max: maxGitStderrBytes}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return xerrors.Errorf("git %s: %w", args[0], err)
	}
	if err := cmd.Start(); err != nil {
		return xerrors.Errorf("start git %s: %w", args[0], err)
	}
	// Killing git does not unblock the read below if a detached
	// descendant still holds stdout, and WaitDelay only applies once
	// Wait runs, so close the read end on cancellation.
	stopClosing := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stopClosing()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 4096), maxGitRecordBytes)
	sc.Split(splitNUL)
	var readErr error
	total, records := 0, 0
	for sc.Scan() {
		total += len(sc.Bytes()) + 1
		records++
		if total > g.maxBytes || records > g.maxRecords {
			readErr = errGitOutputTooLarge
			break
		}
		if err := onRecord(sc.Text()); err != nil {
			readErr = err
			break
		}
	}
	if readErr == nil {
		if err := sc.Err(); errors.Is(err, bufio.ErrTooLong) {
			readErr = errGitOutputTooLarge
		} else if err != nil {
			readErr = err
		}
	}
	if readErr != nil && ctx.Err() != nil {
		readErr = ctx.Err()
	}
	if readErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return xerrors.Errorf("git %s in %s: %w", args[0], dir, readErr)
	}
	if waitErr != nil {
		return xerrors.Errorf("git %s in %s: %w: %s", args[0], dir, waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

//nolint:revive // atEOF is part of the bufio.SplitFunc signature.
func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// cappedBuffer keeps the first max bytes written to it.
type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		_, _ = b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// ignoredListing is what git ls-files reports as ignored under a
// directory.
type ignoredListing struct {
	// files are absolute paths of ignored non-directory entries.
	files map[string]struct{}
	// dirs are directory records relative to the listed directory,
	// ending in "/". git also reports a directory, including "./" for
	// the listed one, when only its contents are ignored, so callers
	// must check whether each directory is ignored itself.
	dirs []string
}

// listIgnored lists the ignored entries under dir. git reports an
// ignored directory once, without reading it.
func (g *gitRunner) listIgnored(ctx context.Context, dir string) (ignoredListing, error) {
	listing := ignoredListing{files: make(map[string]struct{})}
	err := g.run(ctx, gitListTimeout, dir, nil, func(rel string) error {
		switch {
		case rel == "":
		case strings.HasSuffix(rel, "/"):
			listing.dirs = append(listing.dirs, rel)
		default:
			listing.files[filepath.Join(dir, filepath.FromSlash(rel))] = struct{}{}
		}
		return nil
	}, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	return listing, err
}

// checkIgnored reports, for each path relative to dir, whether git
// ignores it.
func (g *gitRunner) checkIgnored(ctx context.Context, dir string, rels []string) ([]bool, error) {
	var in bytes.Buffer
	for _, rel := range rels {
		_, _ = in.WriteString(rel)
		_ = in.WriteByte(0)
	}
	// Each record is source, line number, pattern, and path. A
	// matching negated pattern means the path is not ignored.
	byPath := make(map[string]bool, len(rels))
	var fields [4]string
	n := 0
	err := g.run(ctx, gitCheckTimeout, dir, in.Bytes(), func(field string) error {
		fields[n] = field
		n++
		if n == len(fields) {
			byPath[fields[3]] = fields[2] != "" && !strings.HasPrefix(fields[2], "!")
			n = 0
		}
		return nil
	}, "check-ignore", "--stdin", "-z", "--verbose", "--non-matching")
	// check-ignore exits 1 when no path is ignored; --non-matching
	// still reports every path on stdout.
	if err != nil && !isExitCode(err, 1) {
		return nil, err
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
	var path string
	err := g.run(ctx, gitCheckTimeout, top, nil, func(out string) error {
		path += out
		return nil
	}, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return "", err
	}
	path = filepath.FromSlash(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		path = filepath.Join(top, path)
	}
	return filepath.Clean(path), nil
}
