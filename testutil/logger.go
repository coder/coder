package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
)

// Logger returns a "standard" testing logger, with debug level and common flaky
// errors ignored.
//
// By default, Logger applies a deny-list that drops known-noisy debug entries
// (tailnet, wireguard, pubsub publish chatter, provisioner echo per-file
// extraction, key cache and key rotator boot chatter, periodic background
// loops). Info, warn, error, critical, and fatal entries are never filtered.
//
// To opt out of the default deny-list, pass WithNoFilter(). To add a custom
// filter on top of (or instead of) the defaults, pass WithFilter(fn).
func Logger(t testing.TB, opts ...LoggerOption) slog.Logger {
	cfg := loggerConfig{
		applyDefaults: true,
	}
	for _, o := range opts {
		o(&cfg)
	}

	inner := newTestSink(t, sinkOptions{
		ignoreAllErrors:     cfg.ignoreAllErrors,
		extraIgnoredErrs:    cfg.extraIgnoredErrs,
		extraIgnoreErrorFns: cfg.extraIgnoreErrorFns,
	})
	drop := buildDropFn(cfg)
	return slog.Make(&filteringSink{inner: inner, drop: drop}).Leveled(slog.LevelDebug)
}

// LoggerOption configures the Logger returned by Logger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	applyDefaults       bool
	extra               []func(slog.SinkEntry) bool
	ignoreAllErrors     bool
	extraIgnoredErrs    []error
	extraIgnoreErrorFns []func(slog.SinkEntry) bool
}

// WithNoFilter disables the default deny-list. The returned logger will emit
// every debug entry that the underlying test sink would have emitted. Useful
// when investigating test failures where you want the full firehose.
//
// WithNoFilter can be combined with WithFilter; in that case the defaults are
// off but caller-supplied predicates still apply.
func WithNoFilter() LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.applyDefaults = false
	}
}

// WithFilter adds a caller-supplied predicate to the filter chain. If fn
// returns true for a debug-level entry, that entry is dropped. WithFilter
// composes with the default deny-list via logical OR: an entry is dropped if
// the defaults match it or any WithFilter predicate matches it. Multiple
// WithFilter options compose the same way.
//
// WithFilter only affects debug-level entries; info, warn, and error entries
// are always emitted.
func WithFilter(fn func(slog.SinkEntry) bool) LoggerOption {
	return func(cfg *loggerConfig) {
		if fn != nil {
			cfg.extra = append(cfg.extra, fn)
		}
	}
}

// WithIgnoreErrors causes the logger to never fail the test on error or
// critical log entries. Such entries are still emitted via t.Log; they are
// just downgraded so that they do not call t.Errorf.
//
// This mirrors slogtest.Options.IgnoreErrors and exists to ease migration of
// call sites that previously used slogtest.Make directly.
func WithIgnoreErrors() LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.ignoreAllErrors = true
	}
}

// WithIgnoredErrorIs adds errors that should not fail the test when they
// appear in a log entry's "error" field. Matching uses xerrors.Is.
//
// This is additive to the built-in ignore list (yamux session shutdown,
// context.Canceled / context.DeadlineExceeded, query-canceled errors) and to
// any previous WithIgnoredErrorIs options.
//
// This mirrors slogtest.Options.IgnoredErrorIs.
func WithIgnoredErrorIs(errs ...error) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.extraIgnoredErrs = append(cfg.extraIgnoredErrs, errs...)
	}
}

// WithIgnoreErrorFn registers an additional predicate consulted when deciding
// whether an error or critical entry should fail the test. If the predicate
// returns true, the entry is downgraded to t.Log instead of t.Errorf.
//
// This composes additively with the built-in ignore list, WithIgnoreErrors,
// WithIgnoredErrorIs, and any previous WithIgnoreErrorFn options.
//
// This mirrors slogtest.Options.IgnoreErrorFn.
func WithIgnoreErrorFn(fn func(slog.SinkEntry) bool) LoggerOption {
	return func(cfg *loggerConfig) {
		if fn != nil {
			cfg.extraIgnoreErrorFns = append(cfg.extraIgnoreErrorFns, fn)
		}
	}
}

func buildDropFn(cfg loggerConfig) func(slog.SinkEntry) bool {
	preds := make([]func(slog.SinkEntry) bool, 0, 1+len(cfg.extra))
	if cfg.applyDefaults {
		preds = append(preds, defaultDropDebug)
	}
	preds = append(preds, cfg.extra...)
	switch len(preds) {
	case 0:
		return nil
	case 1:
		return preds[0]
	default:
		return func(ent slog.SinkEntry) bool {
			for _, p := range preds {
				if p(ent) {
					return true
				}
			}
			return false
		}
	}
}

// defaultDropDebug is the built-in deny-list. It matches debug-level entries
// that contribute the bulk of test-log noise without providing useful signal
// during post-mortem.
//
// Guidelines for adding a pattern:
//   - debug level only; never silence info/warn/error,
//   - the logger or message fires >=100 times in a single test run,
//   - the entry does not provide useful flake-debugging context.
//
// HTTP access logs (coderd: GET/POST/PATCH/...) are intentionally NOT in the
// deny-list because they provide essential post-mortem context.
func defaultDropDebug(ent slog.SinkEntry) bool {
	logger := strings.Join(ent.LoggerNames, ".")

	// Tailnet, wireguard, netstack, and coordinator chatter.
	if strings.Contains(logger, ".net.wgengine") ||
		strings.Contains(logger, ".net.netstack") ||
		strings.HasPrefix(logger, "coderd.servertailnet") ||
		strings.HasPrefix(logger, "agent.net.tailnet") ||
		strings.HasPrefix(logger, "cli.net.tailnet") ||
		strings.HasPrefix(logger, "cli.net.wgengine") ||
		strings.HasPrefix(logger, "coderd.coord") {
		return true
	}

	// Key cache and key rotator boot chatter.
	if strings.HasPrefix(logger, "coderd.keyrotator") ||
		strings.Contains(logger, "_keycache") {
		return true
	}

	// Periodic background loops.
	switch logger {
	case "coderd.dbrollup",
		"coderd.metrics_cache",
		"coderd.metadata_batcher",
		"coderd.workspace_usage_tracker",
		"coderd.workspaceapps.stats_collector",
		"coderd.cli-telemetry":
		return true
	}

	// Pubsub lifecycle and per-publish chatter.
	if logger == "pubsub" {
		switch ent.Message {
		case "publish",
			"started listening to event channel",
			"stopped listening to event channel",
			"removing queueSet":
			return true
		}
	}

	// Provisioner echo per-archive-entry chatter.
	if logger == "coderd.echo" {
		switch ent.Message {
		case "read archive entry", "extracted file":
			return true
		}
	}

	return false
}

// filteringSink wraps an inner slog.Sink and drops debug-level entries that
// match drop. Non-debug entries always pass through.
type filteringSink struct {
	inner slog.Sink
	drop  func(slog.SinkEntry) bool
}

func (f *filteringSink) LogEntry(ctx context.Context, ent slog.SinkEntry) {
	if ent.Level == slog.LevelDebug && f.drop != nil && f.drop(ent) {
		return
	}
	f.inner.LogEntry(ctx, ent)
}

func (f *filteringSink) Sync() { f.inner.Sync() }

// testSink mirrors cdr.dev/slog/v3/sloggers/slogtest's testSink, formatting
// entries with sloghuman and dispatching to the appropriate testing.TB method
// based on level.
//
// We own this sink so that we can wrap it in filteringSink. Going through
// slogtest.Make directly would not give us access to the underlying sink.
type testSink struct {
	tb   testing.TB
	opts sinkOptions

	mu       sync.RWMutex
	testDone bool

	// fmtSink is a sloghuman sink writing to a per-entry buffer. We reuse it
	// across entries; it is safe for concurrent use behind the mutex below.
	fmtMu   sync.Mutex
	fmtBuf  *lineBuffer
	fmtSink slog.Sink
}

type sinkOptions struct {
	ignoreAllErrors     bool
	extraIgnoredErrs    []error
	extraIgnoreErrorFns []func(slog.SinkEntry) bool
}

func newTestSink(tb testing.TB, opts sinkOptions) *testSink {
	buf := &lineBuffer{}
	s := &testSink{
		tb:      tb,
		opts:    opts,
		fmtBuf:  buf,
		fmtSink: sloghuman.Sink(buf),
	}
	tb.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.testDone = true
	})
	return s
}

// lineBuffer is an io.Writer that accumulates bytes for one formatted entry at
// a time. The sloghuman sink internally calls Write once per entry with a
// fully-formatted byte slice (terminated with '\n').
type lineBuffer struct {
	buf bytes.Buffer
}

func (b *lineBuffer) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

// take returns the accumulated formatted entry with a single trailing newline
// trimmed (matching slogtest's tb.Log behaviour, which adds its own newline).
func (b *lineBuffer) take() string {
	s := b.buf.String()
	b.buf.Reset()
	return strings.TrimRight(s, "\n")
}

func (s *testSink) LogEntry(ctx context.Context, ent slog.SinkEntry) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.testDone {
		return
	}

	formatted := s.format(ctx, ent)

	switch ent.Level {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn:
		s.tb.Log(formatted)
	case slog.LevelError, slog.LevelCritical:
		if s.shouldIgnoreError(ent) {
			s.tb.Log(formatted)
		} else {
			s.tb.Errorf("%s\n *** slogtest: log detected at level %s; TEST FAILURE ***",
				formatted, ent.Level)
		}
	case slog.LevelFatal:
		s.tb.Fatal(fmt.Sprintf("%s\n *** slogtest: FATAL log detected; TEST FAILURE ***", formatted))
	}
}

func (s *testSink) shouldIgnoreError(ent slog.SinkEntry) bool {
	if s.opts.ignoreAllErrors {
		return true
	}
	if IgnoreLoggedError(ent) {
		return true
	}
	if err, ok := findFirstError(ent); ok {
		for _, ig := range s.opts.extraIgnoredErrs {
			if xerrors.Is(err, ig) {
				return true
			}
		}
	}
	for _, fn := range s.opts.extraIgnoreErrorFns {
		if fn(ent) {
			return true
		}
	}
	return false
}

func (s *testSink) format(ctx context.Context, ent slog.SinkEntry) string {
	s.fmtMu.Lock()
	defer s.fmtMu.Unlock()
	s.fmtSink.LogEntry(ctx, ent)
	return s.fmtBuf.take()
}

func (s *testSink) Sync() {}

// IgnoreLoggedError returns true if the entry's first error field should not
// fail the test. This preserves the previous behaviour of testutil.Logger:
// yamux session shutdown and query-canceled errors are common during test
// teardown and not actionable.
func IgnoreLoggedError(ent slog.SinkEntry) bool {
	err, ok := findFirstError(ent)
	if !ok {
		return false
	}
	// Yamux sessions get shut down when we are shutting down tests, so ignoring
	// them should reduce flakiness.
	if xerrors.Is(err, yamux.ErrSessionShutdown) {
		return true
	}
	// Canceled queries usually happen when we're shutting down tests, and so
	// ignoring them should reduce flakiness. This also includes context.Canceled
	// and context.DeadlineExceeded errors, even if they are not part of a query.
	return isQueryCanceledError(err)
}

// findFirstError finds the first slog.Field named "error" that contains an
// error value.
func findFirstError(ent slog.SinkEntry) (error, bool) {
	for _, f := range ent.Fields {
		if f.Name == "error" {
			if err, ok := f.Value.(error); ok {
				return err, true
			}
		}
	}
	return nil, false
}

// isQueryCanceledError checks if the error is due to a query being canceled.
// This reproduces database.IsQueryCanceledError, but is reimplemented here to
// avoid importing the database package, which would result in import loops. We
// also use string matching on the error PostgreSQL returns to us, rather than
// the pq error type, because we don't want testutil, which is imported in lots
// of places, to import lib/pq, which we have our own fork of.
func isQueryCanceledError(err error) bool {
	if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if strings.Contains(err.Error(), "canceling statement due to user request") {
		return true
	}
	return false
}

type testLogWriter struct {
	t        testing.TB
	mu       sync.Mutex
	testOver bool
}

func NewTestLogWriter(t testing.TB) io.Writer {
	w := &testLogWriter{t: t}
	t.Cleanup(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.testOver = true
	})
	return w
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.testOver {
		return n, nil
	}
	w.t.Logf("%q", string(p))

	return n, nil
}
