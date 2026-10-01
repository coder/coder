// Package agentbox runs untrusted guest scripts inside an in-process
// WebAssembly sandbox with a private, quota-bound scratch directory.
//
// An Engine owns one wazero runtime and a root directory. A Box is one
// sandbox instance: its files live under <root>/<id>/root and are exposed
// to the guest at /box; the script under execution is mounted read-only at
// /script. A box never outlives the process that created it. Engine roots
// left behind by dead processes are removed when a new engine starts.
package agentbox

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	// rootPrefix names engine roots so the startup sweep recognizes them.
	rootPrefix = "coder-agent-boxes-"
	lockFile   = ".lock"
	// staleRootAge is how old an unlocked sibling root must be before the
	// sweep removes it.
	staleRootAge = 24 * time.Hour

	DefaultRunTimeout  = 60 * time.Second
	DefaultMemoryBytes = 256 << 20
	DefaultOutputBytes = 1 << 20
	DefaultDiskBytes   = 256 << 20
	DefaultMaxBoxes    = 64

	wasmPageSize = 65536
)

var (
	// ErrClosed is returned by operations on a closed box or engine.
	ErrClosed = xerrors.New("agent box is closed")
	// ErrBusy is returned when a run cannot start because the engine is at
	// its concurrent run limit and the caller's context ended while waiting.
	ErrBusy = xerrors.New("agent box engine is busy")
	// ErrTooManyBoxes is returned by NewBox at the live box limit.
	ErrTooManyBoxes = xerrors.New("too many live agent boxes")
	// ErrUnknownLanguage is returned for a RunRequest.Language with no
	// registered runtime.
	ErrUnknownLanguage = xerrors.New("unknown language")
)

// Limits bound one box. Zero values take the defaults.
type Limits struct {
	// RunTimeout is the wall clock budget of one Run.
	RunTimeout time.Duration
	// MemoryBytes caps guest linear memory, rounded down to 64 KiB pages.
	MemoryBytes uint32
	// OutputBytes caps captured stdout and stderr separately. Bytes past
	// the cap are discarded and reported as truncated.
	OutputBytes int
	// DiskBytes caps the apparent size of files under /box plus 4 KiB
	// for every created file or directory.
	DiskBytes int64
}

func (l Limits) withDefaults() Limits {
	if l.RunTimeout <= 0 {
		l.RunTimeout = DefaultRunTimeout
	}
	if l.MemoryBytes == 0 {
		l.MemoryBytes = DefaultMemoryBytes
	}
	if l.OutputBytes <= 0 {
		l.OutputBytes = DefaultOutputBytes
	}
	if l.DiskBytes <= 0 {
		l.DiskBytes = DefaultDiskBytes
	}
	return l
}

// Options configures an Engine.
type Options struct {
	Logger slog.Logger
	// Clock drives run deadlines and the stale root sweep. Nil uses the
	// real clock.
	Clock quartz.Clock
	// RootDir is the parent directory of the engine root. Empty uses
	// os.TempDir().
	RootDir string
	Limits  Limits
	// MaxConcurrent bounds runs executing at once across all boxes. Zero
	// uses min(4, max(2, GOMAXPROCS/2)).
	MaxConcurrent int
	// MaxBoxes bounds live boxes. Zero uses DefaultMaxBoxes.
	MaxBoxes int
}

// Engine compiles embedded runtimes once and creates boxes under one
// process-private root directory.
type Engine struct {
	logger   slog.Logger
	clock    quartz.Clock
	limits   Limits
	rootDir  string
	lock     *flock.Flock
	runtime  wazero.Runtime
	runs     *semaphore.Weighted
	maxBoxes int
	// memory backs guest linear memory; nil uses wazero's default.
	memory experimental.MemoryAllocator

	compiled map[string]func() (wazero.CompiledModule, error)

	live   atomic.Int64
	closes sync.WaitGroup
	closed atomic.Bool
}

// defaultMaxConcurrent bounds guest memory at a few runs' worth
// regardless of core count.
func defaultMaxConcurrent(procs int) int {
	return min(4, max(2, procs/2))
}

// NewEngine creates the engine root, removes stale roots left by dead
// processes, and prepares the wazero runtime. Runtime modules compile on
// first use.
func NewEngine(ctx context.Context, opts Options) (*Engine, error) {
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	parent := opts.RootDir
	if parent == "" {
		parent = os.TempDir()
	}
	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent(runtime.GOMAXPROCS(0))
	}
	maxBoxes := opts.MaxBoxes
	if maxBoxes <= 0 {
		maxBoxes = DefaultMaxBoxes
	}
	limits := opts.Limits.withDefaults()

	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, xerrors.Errorf("create agent box parent dir: %w", err)
	}
	rootDir, err := os.MkdirTemp(parent, rootPrefix)
	if err != nil {
		return nil, xerrors.Errorf("create agent box root: %w", err)
	}
	lock := flock.New(filepath.Join(rootDir, lockFile))
	locked, err := lock.TryLock()
	if err == nil && !locked {
		err = xerrors.New("lock held by another process")
	}
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, xerrors.Errorf("lock agent box root: %w", err)
	}

	e := &Engine{
		logger:   opts.Logger,
		clock:    clock,
		limits:   limits,
		rootDir:  rootDir,
		lock:     lock,
		runs:     semaphore.NewWeighted(int64(maxConcurrent)),
		maxBoxes: maxBoxes,
		memory:   newMemoryAllocator(uint64(limits.MemoryBytes/wasmPageSize) * wasmPageSize),
		compiled: make(map[string]func() (wazero.CompiledModule, error), len(runtimes)),
	}
	e.runtime = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithMemoryLimitPages(limits.MemoryBytes/wasmPageSize).
		WithCloseOnContextDone(true))
	wasi_snapshot_preview1.MustInstantiate(ctx, e.runtime)
	for name, rt := range runtimes {
		e.compiled[name] = sync.OnceValues(func() (wazero.CompiledModule, error) {
			return e.runtime.CompileModule(context.WithoutCancel(ctx), rt.module)
		})
	}

	e.sweepStaleRoots(ctx, parent)
	return e, nil
}

// sweepStaleRoots removes sibling engine roots older than staleRootAge
// whose lock is not held. A live engine holds its lock for its lifetime,
// so an idle process older than the age threshold keeps its root.
func (e *Engine) sweepStaleRoots(ctx context.Context, parent string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		e.logger.Warn(ctx, "list agent box roots for sweep", slog.Error(err))
		return
	}
	cutoff := e.clock.Now().Add(-staleRootAge)
	for _, entry := range entries {
		// Type() does not follow symlinks, so a link named like a root is
		// skipped rather than traversed.
		if !entry.Type().IsDir() || !strings.HasPrefix(entry.Name(), rootPrefix) {
			continue
		}
		dir := filepath.Join(parent, entry.Name())
		if dir == e.rootDir {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		// flock opens with O_CREATE and follows symlinks, so only an
		// absent or regular lock file is safe to lock.
		lockPath := filepath.Join(dir, lockFile)
		if st, err := os.Lstat(lockPath); err == nil && !st.Mode().IsRegular() {
			continue
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		lock := flock.New(lockPath)
		locked, err := lock.TryLock()
		if err != nil || !locked {
			continue
		}
		removeErr := os.RemoveAll(dir)
		_ = lock.Close()
		if removeErr != nil {
			e.logger.Warn(ctx, "remove stale agent box root", slog.F("dir", dir), slog.Error(removeErr))
			continue
		}
		e.logger.Info(ctx, "removed stale agent box root", slog.F("dir", dir))
	}
}

// Languages lists the registered runtime names in sorted order.
func (*Engine) Languages() []string {
	return slices.Sorted(maps.Keys(runtimes))
}

// RootDir is the engine's private root directory.
func (e *Engine) RootDir() string {
	return e.rootDir
}

// Limits returns the effective per-box limits.
func (e *Engine) Limits() Limits {
	return e.limits
}

// NewBox creates an empty box. It fails with ErrTooManyBoxes at the live
// box limit and with ErrClosed after Close.
func (e *Engine) NewBox() (*Box, error) {
	if e.closed.Load() {
		return nil, ErrClosed
	}
	if e.live.Add(1) > int64(e.maxBoxes) {
		e.live.Add(-1)
		return nil, ErrTooManyBoxes
	}
	box, err := newBox(e, rand.Text())
	if err != nil {
		e.live.Add(-1)
		return nil, err
	}
	return box, nil
}

func (e *Engine) compile(ctx context.Context, rt guestRuntime) (wazero.CompiledModule, error) {
	once := e.compiled[rt.name]
	if once == nil {
		return nil, ErrUnknownLanguage
	}
	// Compilation is shared by every box, so it runs on the engine's
	// context and a caller whose context ends still waits for it to
	// finish.
	compiled, err := once()
	if err != nil {
		return nil, xerrors.Errorf("compile %s runtime: %w", rt.name, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return compiled, nil
}

// Close waits for boxes closing in the background, releases the wazero
// runtime, and removes the engine root. Boxes still open lose their
// directories; callers close boxes first.
func (e *Engine) Close(ctx context.Context) error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.closes.Wait()
	var errs []error
	if err := e.runtime.Close(ctx); err != nil {
		errs = append(errs, xerrors.Errorf("close wazero runtime: %w", err))
	}
	if err := e.lock.Close(); err != nil {
		errs = append(errs, xerrors.Errorf("release root lock: %w", err))
	}
	if err := os.RemoveAll(e.rootDir); err != nil {
		errs = append(errs, xerrors.Errorf("remove agent box root: %w", err))
	}
	return errors.Join(errs...)
}
