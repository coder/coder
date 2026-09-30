package agentbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/sys"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// GuestDir is the guest path of the box directory.
const GuestDir = "/box"

// guestScriptDir is the read-only guest mount holding the running script.
const guestScriptDir = "/script"

// RunRequest describes one guest execution.
type RunRequest struct {
	Language string
	Code     string
	Stdin    string
	Args     []string
}

// RunResult is the outcome of a guest execution that started. Host-side
// failures are returned as errors instead.
type RunResult struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
	TimedOut        bool
	Duration        time.Duration
}

// ReadLinesResult mirrors the read_file tool result shape.
type ReadLinesResult struct {
	Content    string
	FileSize   int64
	TotalLines int
	LinesRead  int
}

// Box is one sandbox with a private directory. Methods are safe for
// concurrent use; Run calls serialize.
type Box struct {
	id        string
	engine    *Engine
	dir       string
	rootDir   string
	scriptDir string
	root      *os.Root
	quota     *quota

	mu     sync.Mutex
	closed bool
	// closing is set by Close before it cancels the current run, so a
	// run that has not stored its cancel yet, or is queued on mu, stops
	// instead of executing.
	closing   atomic.Bool
	runCancel atomic.Pointer[context.CancelFunc]
}

func newBox(engine *Engine, id string) (*Box, error) {
	dir := filepath.Join(engine.rootDir, id)
	rootDir := filepath.Join(dir, "root")
	scriptDir := filepath.Join(dir, "script")
	for _, d := range []string{rootDir, scriptDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			_ = os.RemoveAll(dir)
			return nil, xerrors.Errorf("create box dir: %w", err)
		}
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, xerrors.Errorf("open box root: %w", err)
	}
	return &Box{
		id:        id,
		engine:    engine,
		dir:       dir,
		rootDir:   rootDir,
		scriptDir: scriptDir,
		root:      root,
		quota:     newQuota(engine.limits.DiskBytes),
	}, nil
}

// ID is a random identifier unique within the engine.
func (b *Box) ID() string {
	return b.id
}

// Run executes req.Code with the requested language runtime. The guest
// sees the box at /box; stdout and stderr are captured up to the output
// limit. ErrBusy is returned when ctx ends before a run slot is free.
func (b *Box) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	rt, ok := runtimes[req.Language]
	if !ok {
		return RunResult{}, xerrors.Errorf("language %q: %w", req.Language, ErrUnknownLanguage)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return RunResult{}, ErrClosed
	}

	// Close cancels runCtx, so every blocking phase below (slot wait,
	// first-use compile, execution) ends promptly when the box goes away.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	b.runCancel.Store(&cancel)
	defer b.runCancel.Store(nil)
	if b.closing.Load() {
		return RunResult{}, ErrClosed
	}

	if err := b.engine.runs.Acquire(runCtx, 1); err != nil {
		return RunResult{}, xerrors.Errorf("acquire run slot (%v): %w", err, ErrBusy)
	}
	defer b.engine.runs.Release(1)

	compiled, err := b.engine.compile(runCtx, rt)
	if err != nil {
		return RunResult{}, err
	}
	scriptPath := filepath.Join(b.scriptDir, rt.entry)
	if err := os.WriteFile(scriptPath, []byte(req.Code), 0o600); err != nil {
		return RunResult{}, xerrors.Errorf("write script: %w", err)
	}

	var timedOut atomic.Bool
	timer := b.engine.clock.AfterFunc(b.engine.limits.RunTimeout, func() {
		timedOut.Store(true)
		cancel()
	}, "agentbox", "run-timeout")
	defer timer.Stop()

	stdout := newBoundedWriter(b.engine.limits.OutputBytes)
	stderr := newBoundedWriter(b.engine.limits.OutputBytes)
	fsConfig, err := b.fsConfig()
	if err != nil {
		return RunResult{}, err
	}
	config := wazero.NewModuleConfig().
		WithName("").
		WithArgs(rt.argv(path.Join(guestScriptDir, rt.entry), req.Args)...).
		WithStdin(strings.NewReader(req.Stdin)).
		WithStdout(stdout).
		WithStderr(stderr).
		WithFSConfig(fsConfig).
		WithSysWalltime().
		WithSysNanotime()

	start := b.engine.clock.Now("agentbox", "run-start")
	mod, err := b.engine.runtime.InstantiateModule(runCtx, compiled, config)
	duration := b.engine.clock.Since(start, "agentbox", "run-end")
	if mod != nil {
		_ = mod.Close(context.WithoutCancel(ctx))
	}

	result := RunResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		StdoutTruncated: stdout.truncated(),
		StderrTruncated: stderr.truncated(),
		Duration:        duration,
	}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := errors.AsType[*sys.ExitError](err); ok {
		result.ExitCode = int(exitErr.ExitCode())
		// The timer can fire after the guest exited on its own; only an
		// interrupted run timed out.
		switch exitErr.ExitCode() {
		case sys.ExitCodeContextCanceled, sys.ExitCodeDeadlineExceeded:
			result.TimedOut = timedOut.Load()
		}
		return result, nil
	}
	if runCtx.Err() != nil {
		// Cancellation surfaces as an ExitError with
		// ExitCodeContextCanceled in most paths; this covers the rest.
		result.ExitCode = int(sys.ExitCodeContextCanceled)
		result.TimedOut = timedOut.Load()
		return result, nil
	}
	return RunResult{}, xerrors.Errorf("run guest: %w", err)
}

// fsConfig mounts the box directory at /box through the quota adapter and
// the script directory read-only at /script. Both mounts share one open
// file limit.
func (b *Box) fsConfig() (wazero.FSConfig, error) {
	limit := &openLimit{max: maxOpenFiles}
	boxMount := limitedFS{FS: newBoxFS(sysfs.DirFS(b.rootDir), b.quota), limit: limit}
	scriptMount := limitedFS{FS: &sysfs.ReadFS{FS: sysfs.DirFS(b.scriptDir)}, limit: limit}

	base, ok := wazero.NewFSConfig().(sysfs.FSConfig)
	if !ok {
		return nil, xerrors.New("wazero FSConfig does not support sys.FS mounts")
	}
	withBox, ok := base.WithSysFSMount(boxMount, GuestDir).(sysfs.FSConfig)
	if !ok {
		return nil, xerrors.New("wazero FSConfig does not support sys.FS mounts")
	}
	return withBox.WithSysFSMount(scriptMount, guestScriptDir), nil
}

// WriteFile writes data to p under the box, creating parent directories.
// p is relative to /box or prefixed by it.
func (b *Box) WriteFile(p string, data []byte) error {
	rel, err := boxPath(p)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}

	var charged int64
	dir := path.Dir(rel)
	if dir != "." {
		// Count the directories MkdirAll creates so they are charged.
		for d := dir; d != "."; d = path.Dir(d) {
			if _, err := b.root.Lstat(d); errors.Is(err, fs.ErrNotExist) {
				charged += entryCost
			}
		}
	}
	var replaced int64
	switch st, err := b.root.Lstat(rel); {
	case err == nil && st.Mode().IsRegular():
		replaced = st.Size()
	case err == nil:
		return xerrors.Errorf("%s is not a regular file", p)
	case errors.Is(err, fs.ErrNotExist):
		charged += entryCost
	default:
		return xerrors.Errorf("stat %s: %w", p, err)
	}
	charged += int64(len(data)) - replaced
	if !b.quota.charge(charged) {
		return xerrors.Errorf("write %s: disk quota of %d bytes exceeded", p, b.engine.limits.DiskBytes)
	}
	if dir != "." {
		if err := b.root.MkdirAll(dir, 0o700); err != nil {
			b.quota.refund(charged)
			return xerrors.Errorf("create parent dirs for %s: %w", p, err)
		}
	}
	if err := b.root.WriteFile(rel, data, 0o600); err != nil {
		b.quota.refund(charged)
		return xerrors.Errorf("write %s: %w", p, err)
	}
	// A shrinking replacement charged nothing; release the difference.
	b.quota.refund(-charged)
	return nil
}

// ReadFile returns at most limit bytes of the file at p.
func (b *Box) ReadFile(p string, limit int64) ([]byte, error) {
	rel, err := boxPath(p)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	f, err := b.root.Open(rel)
	if err != nil {
		return nil, xerrors.Errorf("open %s: %w", p, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", p, err)
	}
	return data, nil
}

// ReadLines returns a numbered window of the file at p using read_file
// semantics: offset is a 1-based line number and limit a line count, both
// bounded by workspacesdk.DefaultReadFileLinesLimits.
func (b *Box) ReadLines(p string, offset, limit int) (ReadLinesResult, error) {
	limits := workspacesdk.DefaultReadFileLinesLimits()
	data, err := b.ReadFile(p, limits.MaxFileSize+1)
	if err != nil {
		return ReadLinesResult{}, err
	}
	if int64(len(data)) > limits.MaxFileSize {
		return ReadLinesResult{}, xerrors.Errorf(
			"file exceeds the maximum of %d bytes; use box_run to extract the content you need",
			limits.MaxFileSize,
		)
	}
	if len(data) == 0 {
		return ReadLinesResult{}, nil
	}
	lines := strings.Split(string(data), "\n")
	total := len(lines)
	if offset < 1 {
		offset = 1
	}
	if offset > total {
		return ReadLinesResult{}, xerrors.Errorf("offset %d is beyond the file length of %d lines", offset, total)
	}
	if limit <= 0 {
		limit = limits.MaxResponseLines
	}
	end := min(offset-1+limit, total)

	var out strings.Builder
	read := 0
	for i := offset - 1; i < end; i++ {
		line := lines[i]
		if len(line) > limits.MaxLineBytes {
			line = line[:limits.MaxLineBytes] + "... [truncated]"
		}
		numbered := fmt.Sprintf("%d\t%s", i+1, line)
		next := out.Len() + len(numbered)
		if read > 0 {
			next++
		}
		if next > limits.MaxResponseBytes {
			return ReadLinesResult{}, xerrors.Errorf(
				"output would exceed %d bytes; read less at a time using offset and limit",
				limits.MaxResponseBytes,
			)
		}
		if read >= limits.MaxResponseLines {
			return ReadLinesResult{}, xerrors.Errorf(
				"output would exceed %d lines; read less at a time using offset and limit",
				limits.MaxResponseLines,
			)
		}
		if read > 0 {
			_ = out.WriteByte('\n')
		}
		_, _ = out.WriteString(numbered)
		read++
	}
	return ReadLinesResult{
		Content:    out.String(),
		FileSize:   int64(len(data)),
		TotalLines: total,
		LinesRead:  read,
	}, nil
}

// Close cancels any in-flight run, waits for it to return, and removes
// the box directory. It is idempotent.
func (b *Box) Close() error {
	b.closing.Store(true)
	if cancel := b.runCancel.Load(); cancel != nil {
		(*cancel)()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.engine.live.Add(-1)
	var errs []error
	if err := b.root.Close(); err != nil {
		errs = append(errs, xerrors.Errorf("close box root: %w", err))
	}
	if err := os.RemoveAll(b.dir); err != nil {
		errs = append(errs, xerrors.Errorf("remove box dir: %w", err))
	}
	return errors.Join(errs...)
}

// CloseAsync closes the box in the background and logs a failure.
// Engine.Close waits for closes started this way.
func (b *Box) CloseAsync() {
	b.engine.closes.Go(func() {
		if err := b.Close(); err != nil {
			b.engine.logger.Warn(context.Background(), "failed to close agent box", slog.F("box_id", b.id), slog.Error(err))
		}
	})
}

// boxPath validates a caller path and returns it relative to the box root.
func boxPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", xerrors.New("path is required")
	}
	if strings.ContainsRune(p, 0) {
		return "", xerrors.New("path contains a NUL byte")
	}
	rel := p
	if strings.HasPrefix(p, "/") {
		r, ok := strings.CutPrefix(p, GuestDir)
		if !ok || (r != "" && !strings.HasPrefix(r, "/")) {
			return "", xerrors.Errorf("path %q must be under %s", p, GuestDir)
		}
		rel = r
	}
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return "", xerrors.Errorf("path %q is the box directory", p)
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == ".." {
			return "", xerrors.Errorf("path %q must not contain ..", p)
		}
	}
	rel = path.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", xerrors.Errorf("path %q must be under %s", p, GuestDir)
	}
	return rel, nil
}

// boundedWriter keeps the first limit bytes and reports every write as
// complete so a guest writing past the cap does not spin on short writes.
type boundedWriter struct {
	buf       bytes.Buffer
	limit     int
	discarded int64
}

func newBoundedWriter(limit int) *boundedWriter {
	return &boundedWriter{limit: limit}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	room := w.limit - w.buf.Len()
	if room > 0 {
		_, _ = w.buf.Write(p[:min(room, len(p))])
	}
	if len(p) > room {
		w.discarded += int64(len(p) - max(room, 0))
	}
	return len(p), nil
}

func (w *boundedWriter) String() string {
	return w.buf.String()
}

func (w *boundedWriter) truncated() bool {
	return w.discarded > 0
}
