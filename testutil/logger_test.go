package testutil_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/testutil"
)

// recorderTB is a minimal testing.TB implementation used to capture calls made
// by the testutil.Logger sink. It is intentionally not goroutine-safe except
// where the sink itself synchronises, which matches how testing.TB is used in
// practice.
type recorderTB struct {
	testing.TB

	mu       sync.Mutex
	logs     []string
	errors   []string
	fatals   []string
	failed   bool
	cleanups []func()
	name     string
}

func newRecorderTB(name string) *recorderTB {
	return &recorderTB{name: name}
}

func (r *recorderTB) Cleanup(f func()) {
	r.mu.Lock()
	r.cleanups = append(r.cleanups, f)
	r.mu.Unlock()
}

func (r *recorderTB) runCleanups() {
	r.mu.Lock()
	cs := r.cleanups
	r.cleanups = nil
	r.mu.Unlock()
	// LIFO, matching testing.T.
	for i := len(cs) - 1; i >= 0; i-- {
		cs[i]()
	}
}

func (r *recorderTB) Error(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprint(args...))
	r.failed = true
}

func (r *recorderTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
	r.failed = true
}

func (r *recorderTB) Fatal(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fatals = append(r.fatals, fmt.Sprint(args...))
	r.failed = true
}

func (r *recorderTB) Fatalf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
	r.failed = true
}

func (r *recorderTB) Log(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprint(args...))
}

func (r *recorderTB) Logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *recorderTB) Helper()      {}
func (r *recorderTB) Name() string { return r.name }

func (r *recorderTB) Failed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed
}

func (r *recorderTB) logCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.logs)
}

func (r *recorderTB) errorCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.errors)
}

// logsContain returns true if any captured log entry contains substr.
func (r *recorderTB) logsContain(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// emitDebug logs a single debug entry under the given dotted logger name with
// the given message and returns the recorder.
func emitDebug(t *testing.T, loggerName, msg string, opts ...testutil.LoggerOption) *recorderTB {
	t.Helper()
	rec := newRecorderTB(t.Name())
	log := testutil.Logger(rec, opts...)
	log = withNamed(log, loggerName)
	log.Debug(context.Background(), msg)
	rec.runCleanups()
	return rec
}

func emitAt(t *testing.T, level slog.Level, loggerName, msg string, opts ...testutil.LoggerOption) *recorderTB {
	t.Helper()
	rec := newRecorderTB(t.Name())
	log := testutil.Logger(rec, opts...)
	log = withNamed(log, loggerName)
	switch level {
	case slog.LevelDebug:
		log.Debug(context.Background(), msg)
	case slog.LevelInfo:
		log.Info(context.Background(), msg)
	case slog.LevelWarn:
		log.Warn(context.Background(), msg)
	case slog.LevelError:
		log.Error(context.Background(), msg)
	default:
		t.Fatalf("unsupported level %v", level)
	}
	rec.runCleanups()
	return rec
}

// withNamed applies a dotted logger name as a series of .Named() calls.
func withNamed(log slog.Logger, dotted string) slog.Logger {
	if dotted == "" {
		return log
	}
	for _, part := range strings.Split(dotted, ".") {
		log = log.Named(part)
	}
	return log
}

// --- Red phase tests ---

func TestLogger_DropsTailnetDebug(t *testing.T) {
	t.Parallel()
	rec := emitDebug(t, "coderd.servertailnet.net.wgengine", "wgengine: Reconfig: configuring router")
	if got := rec.logCount(); got != 0 {
		t.Fatalf("expected 0 log lines, got %d: %v", got, rec.logs)
	}
}

func TestLogger_DropsPubsubPublishDebug(t *testing.T) {
	t.Parallel()
	rec := emitDebug(t, "pubsub", "publish")
	if got := rec.logCount(); got != 0 {
		t.Fatalf("expected pubsub publish debug to be dropped, got %d log lines", got)
	}
}

func TestLogger_KeepsPubsubInfo(t *testing.T) {
	t.Parallel()
	rec := emitAt(t, slog.LevelInfo, "pubsub", "publish")
	if got := rec.logCount(); got != 1 {
		t.Fatalf("expected pubsub publish info to be kept, got %d log lines", got)
	}
}

func TestLogger_KeepsHTTPAccessLogsDebug(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE", "PUT", "HEAD"} {
		rec := emitDebug(t, "coderd", method)
		if got := rec.logCount(); got != 1 {
			t.Fatalf("expected coderd %s debug to be kept, got %d log lines", method, got)
		}
	}
}

func TestLogger_KeepsAcquirerDebug(t *testing.T) {
	t.Parallel()
	rec := emitDebug(t, "coderd.acquirer", "acquiring job")
	if got := rec.logCount(); got != 1 {
		t.Fatalf("expected coderd.acquirer debug to be kept, got %d log lines", got)
	}
}

func TestLogger_DropsEchoArchiveEntries(t *testing.T) {
	t.Parallel()
	for _, msg := range []string{"read archive entry", "extracted file"} {
		rec := emitDebug(t, "coderd.echo", msg)
		if got := rec.logCount(); got != 0 {
			t.Fatalf("expected coderd.echo %q to be dropped, got %d log lines", msg, got)
		}
	}
	// Other echo debug messages should pass.
	rec := emitDebug(t, "coderd.echo", "unpacking source archive")
	if got := rec.logCount(); got != 1 {
		t.Fatalf("expected coderd.echo other-message to be kept, got %d log lines", got)
	}
}

func TestLogger_DropsPeriodicLoopDebug(t *testing.T) {
	t.Parallel()
	periodicLoggers := []string{
		"coderd.dbrollup",
		"coderd.metrics_cache",
		"coderd.metadata_batcher",
		"coderd.workspace_usage_tracker",
		"coderd.workspaceapps.stats_collector",
		"coderd.cli-telemetry",
	}
	for _, lg := range periodicLoggers {
		rec := emitDebug(t, lg, "some periodic message")
		if got := rec.logCount(); got != 0 {
			t.Fatalf("expected periodic logger %q debug to be dropped, got %d log lines", lg, got)
		}
	}
}

func TestLogger_DropsKeyRotatorDebug(t *testing.T) {
	t.Parallel()
	rec := emitDebug(t, "coderd.keyrotator", "inserted new key for feature")
	if got := rec.logCount(); got != 0 {
		t.Fatalf("expected keyrotator debug to be dropped, got %d log lines", got)
	}
}

func TestLogger_DropsKeyCacheDebug(t *testing.T) {
	t.Parallel()
	caches := []string{
		"coderd.app_signing_keycache.workspace_apps_token_signing_keycache",
		"coderd.workspace_apps_api_key_encryption_keycache",
		"coderd.oidc_convert_keycache.oidc_convert_signing_keycache",
	}
	for _, lg := range caches {
		rec := emitDebug(t, lg, "fetching crypto keys")
		if got := rec.logCount(); got != 0 {
			t.Fatalf("expected keycache logger %q debug to be dropped, got %d log lines", lg, got)
		}
	}
}

func TestLogger_KeepsAllWarn(t *testing.T) {
	t.Parallel()
	deniedLoggers := []string{
		"coderd.servertailnet.net.wgengine",
		"pubsub",
		"coderd.echo",
		"coderd.dbrollup",
		"coderd.keyrotator",
		"coderd.coord",
	}
	for _, lg := range deniedLoggers {
		rec := emitAt(t, slog.LevelWarn, lg, "something looks wrong")
		if got := rec.logCount(); got != 1 {
			t.Fatalf("expected warn on %q to be kept, got %d log lines", lg, got)
		}
	}
}

func TestLogger_KeepsAllInfo(t *testing.T) {
	t.Parallel()
	deniedLoggers := []string{
		"coderd.servertailnet.net.wgengine",
		"pubsub",
		"coderd.echo",
		"coderd.dbrollup",
		"coderd.keyrotator",
		"coderd.coord",
	}
	for _, lg := range deniedLoggers {
		rec := emitAt(t, slog.LevelInfo, lg, "an informative message")
		if got := rec.logCount(); got != 1 {
			t.Fatalf("expected info on %q to be kept, got %d log lines", lg, got)
		}
	}
}

func TestLogger_KeepsErrorAndFailsTest(t *testing.T) {
	t.Parallel()
	// An error on a deny-listed logger must still call tb.Errorf and mark the
	// test as failed. This is the critical regression test: filtering must not
	// swallow real test signals.
	rec := newRecorderTB(t.Name())
	log := testutil.Logger(rec)
	log = withNamed(log, "coderd.servertailnet.net.wgengine")
	log.Error(context.Background(), "something is very wrong")
	rec.runCleanups()
	if rec.errorCount() == 0 {
		t.Fatalf("expected tb.Errorf to be called, got 0 errors")
	}
	if !rec.Failed() {
		t.Fatalf("expected recorder.Failed() to be true")
	}
}

func TestLogger_WithNoFilter_KeepsEverything(t *testing.T) {
	t.Parallel()
	cases := []struct {
		logger string
		msg    string
	}{
		{"coderd.servertailnet.net.wgengine", "wgengine: Reconfig: configuring router"},
		{"pubsub", "publish"},
		{"coderd.echo", "read archive entry"},
		{"coderd.dbrollup", "rolling up data"},
		{"coderd.keyrotator", "inserted new key for feature"},
		{"coderd.coord", "peerReadLoop got request"},
	}
	for _, c := range cases {
		rec := emitDebug(t, c.logger, c.msg, testutil.WithNoFilter())
		if got := rec.logCount(); got != 1 {
			t.Fatalf("with WithNoFilter, expected %q/%q debug to be kept, got %d log lines",
				c.logger, c.msg, got)
		}
	}
}

func TestLogger_RespectsIgnoreErrorFn(t *testing.T) {
	t.Parallel()
	// Errors matching testutil.IgnoreLoggedError should be downgraded to tb.Log
	// instead of failing the test. Reuses the existing semantics.
	cases := []error{
		context.Canceled,
		context.DeadlineExceeded,
		yamux.ErrSessionShutdown,
		xerrors.New("canceling statement due to user request"),
	}
	for _, ignored := range cases {
		rec := newRecorderTB(t.Name())
		log := testutil.Logger(rec).Named("coderd")
		log.Error(context.Background(), "transient error, retrying", slog.Error(ignored))
		rec.runCleanups()
		if rec.errorCount() != 0 {
			t.Fatalf("expected error %v to be downgraded to log, got %d errors", ignored, rec.errorCount())
		}
		if rec.logCount() != 1 {
			t.Fatalf("expected error %v to appear in tb.Log, got %d log lines", ignored, rec.logCount())
		}
		if rec.Failed() {
			t.Fatalf("expected recorder.Failed() to be false for ignored error %v", ignored)
		}
	}
}

func TestLogger_NoOptions_BehavesLikeBefore(t *testing.T) {
	t.Parallel()
	// Smoke test: Logger(t) returns a working logger with the default deny-list applied.
	rec := newRecorderTB(t.Name())
	log := testutil.Logger(rec)
	log.Named("pubsub").Debug(context.Background(), "publish")
	log.Named("coderd").Info(context.Background(), "starting up")
	rec.runCleanups()
	// pubsub.publish dropped, coderd.starting up kept.
	if rec.logCount() != 1 {
		t.Fatalf("expected exactly 1 log line, got %d: %v", rec.logCount(), rec.logs)
	}
	if !rec.logsContain("starting up") {
		t.Fatalf("expected 'starting up' in logs, got %v", rec.logs)
	}
}

func TestLogger_WithFilter_DropsCustomPattern(t *testing.T) {
	t.Parallel()
	custom := func(ent slog.SinkEntry) bool {
		return strings.Join(ent.LoggerNames, ".") == "my.pkg"
	}
	// Matching debug entry is dropped.
	rec := emitDebug(t, "my.pkg", "noisy detail", testutil.WithFilter(custom))
	if got := rec.logCount(); got != 0 {
		t.Fatalf("expected custom filter to drop my.pkg debug, got %d log lines", got)
	}
	// Non-matching, non-deny-listed debug entry passes.
	rec = emitDebug(t, "my.otherpkg", "interesting detail", testutil.WithFilter(custom))
	if got := rec.logCount(); got != 1 {
		t.Fatalf("expected custom filter to keep non-matching debug, got %d log lines", got)
	}
	// Deny-listed debug entry remains dropped even when custom filter doesn't match.
	rec = emitDebug(t, "pubsub", "publish", testutil.WithFilter(custom))
	if got := rec.logCount(); got != 0 {
		t.Fatalf("expected default deny-list still active, got %d log lines", got)
	}
}

func TestLogger_WithFilter_DoesNotAffectNonDebug(t *testing.T) {
	t.Parallel()
	// A custom filter that returns true for everything must still allow non-debug entries through.
	always := func(slog.SinkEntry) bool { return true }
	for _, lvl := range []slog.Level{slog.LevelInfo, slog.LevelWarn} {
		rec := emitAt(t, lvl, "my.pkg", "important", testutil.WithFilter(always))
		if got := rec.logCount(); got != 1 {
			t.Fatalf("expected %v level to bypass custom filter, got %d log lines", lvl, got)
		}
	}
}

func TestLogger_WithFilter_ComposesWithDefaults(t *testing.T) {
	t.Parallel()
	custom := func(ent slog.SinkEntry) bool {
		return strings.Join(ent.LoggerNames, ".") == "my.pkg"
	}
	// Matched by defaults only.
	rec := emitDebug(t, "pubsub", "publish", testutil.WithFilter(custom))
	if rec.logCount() != 0 {
		t.Fatalf("expected default match to drop, got %d log lines", rec.logCount())
	}
	// Matched by custom only.
	rec = emitDebug(t, "my.pkg", "anything", testutil.WithFilter(custom))
	if rec.logCount() != 0 {
		t.Fatalf("expected custom match to drop, got %d log lines", rec.logCount())
	}
	// Matched by neither.
	rec = emitDebug(t, "other.pkg", "anything", testutil.WithFilter(custom))
	if rec.logCount() != 1 {
		t.Fatalf("expected unmatched debug to pass, got %d log lines", rec.logCount())
	}
}

func TestLogger_WithFilter_MultipleCompose(t *testing.T) {
	t.Parallel()
	f1 := func(ent slog.SinkEntry) bool {
		return strings.Join(ent.LoggerNames, ".") == "alpha"
	}
	f2 := func(ent slog.SinkEntry) bool {
		return strings.Join(ent.LoggerNames, ".") == "beta"
	}
	// f1 matches.
	rec := emitDebug(t, "alpha", "msg", testutil.WithFilter(f1), testutil.WithFilter(f2))
	if rec.logCount() != 0 {
		t.Fatalf("expected f1 to drop alpha, got %d log lines", rec.logCount())
	}
	// f2 matches.
	rec = emitDebug(t, "beta", "msg", testutil.WithFilter(f1), testutil.WithFilter(f2))
	if rec.logCount() != 0 {
		t.Fatalf("expected f2 to drop beta, got %d log lines", rec.logCount())
	}
	// Neither matches.
	rec = emitDebug(t, "gamma", "msg", testutil.WithFilter(f1), testutil.WithFilter(f2))
	if rec.logCount() != 1 {
		t.Fatalf("expected gamma to pass, got %d log lines", rec.logCount())
	}
}

func TestLogger_WithNoFilter_WithFilter_KeepsCustomOnly(t *testing.T) {
	t.Parallel()
	custom := func(ent slog.SinkEntry) bool {
		return strings.Join(ent.LoggerNames, ".") == "my.pkg"
	}
	// Defaults are off, custom is on.
	// Deny-listed pubsub.publish should now pass.
	rec := emitDebug(t, "pubsub", "publish", testutil.WithNoFilter(), testutil.WithFilter(custom))
	if rec.logCount() != 1 {
		t.Fatalf("expected pubsub.publish to pass with WithNoFilter, got %d log lines", rec.logCount())
	}
	// Custom-matched entry should still drop.
	rec = emitDebug(t, "my.pkg", "anything", testutil.WithNoFilter(), testutil.WithFilter(custom))
	if rec.logCount() != 0 {
		t.Fatalf("expected my.pkg debug to drop via custom filter, got %d log lines", rec.logCount())
	}
}
