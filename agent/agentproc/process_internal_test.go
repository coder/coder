package agentproc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentrunonce"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type managerOption func(*manager)

func withUpdateEnv(fn func(current []string) (updated []string, err error)) managerOption {
	return func(m *manager) { m.updateEnv = fn }
}

func withWorkingDir(fn func() string) managerOption {
	return func(m *manager) { m.workingDir = fn }
}

func withMockClock(clock quartz.Clock) managerOption {
	return func(m *manager) { m.clock = clock }
}

func newTestManager(t *testing.T, opts ...managerOption) *manager {
	t.Helper()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	for _, opt := range opts {
		opt(m)
	}
	t.Cleanup(func() {
		_ = m.Close()
	})
	return m
}

// startGate stalls the first start between its reservation and the
// spawn, so a test can act while that reservation is pending. Later
// starts pass through.
type startGate struct {
	stalled  chan struct{}
	released chan struct{}
	first    atomic.Bool
	release  func()
}

func newStartGate(t *testing.T) *startGate {
	t.Helper()

	g := &startGate{
		stalled:  make(chan struct{}),
		released: make(chan struct{}),
	}
	g.release = sync.OnceFunc(func() { close(g.released) })
	t.Cleanup(g.release)
	return g
}

func (g *startGate) option() managerOption {
	return withUpdateEnv(func(current []string) ([]string, error) {
		if g.first.CompareAndSwap(false, true) {
			close(g.stalled)
			<-g.released
		}
		return current, nil
	})
}

func (g *startGate) waitStalled(t *testing.T) {
	t.Helper()

	select {
	case <-g.stalled:
	case <-time.After(testutil.WaitShort):
		t.Fatal("no start reached the gate")
	}
}

type startResult struct {
	proc     *process
	attached bool
	err      error
}

func startAsync(ctx context.Context, m *manager, req workspacesdk.StartProcessRequest, chatID string) <-chan startResult {
	results := make(chan startResult, 1)
	go func() {
		proc, attached, err := m.start(ctx, req, chatID)
		results <- startResult{proc: proc, attached: attached, err: err}
	}()
	return results
}

func awaitStart(t *testing.T, results <-chan startResult) startResult {
	t.Helper()

	select {
	case res := <-results:
		return res
	case <-time.After(testutil.WaitShort):
		t.Fatal("start did not return")
		return startResult{}
	}
}

func requireStarted(t *testing.T, m *manager, req workspacesdk.StartProcessRequest, chatID string) *process {
	t.Helper()

	proc, attached, err := m.start(context.Background(), req, chatID)
	require.NoError(t, err)
	require.False(t, attached)
	return proc
}

func requireAttached(t *testing.T, m *manager, req workspacesdk.StartProcessRequest, chatID string) *process {
	t.Helper()

	proc, attached, err := m.start(context.Background(), req, chatID)
	require.NoError(t, err)
	require.True(t, attached)
	return proc
}

func requireTracked(t *testing.T, m *manager, id string) bool {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.procs[id]
	return ok
}

func TestReapFreesKeyedReservation(t *testing.T) {
	t.Parallel()

	mClock := quartz.NewMock(t)
	m := newTestManager(t, withMockClock(mClock))

	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-1",
	}
	proc := requireStarted(t, m, req, "chat-1")
	<-proc.done

	// The key keeps attaching to the exited process until the reap
	// age passes, so a retried start still sees the result.
	require.Equal(t, proc.id, requireAttached(t, m, req, "chat-1").id)

	mClock.Advance(keyedProcessReapAge + time.Minute)

	// The sweep on start reaps the exited process, frees its
	// reservation, and the same key starts fresh.
	fresh := requireStarted(t, m, req, "chat-1")
	require.NotEqual(t, proc.id, fresh.id)
	require.False(t, requireTracked(t, m, proc.id))
	require.Equal(t, 1, m.runOnce.Len())
	<-fresh.done
}

func TestKeyedRetryIgnoresDefaultWorkdirDrift(t *testing.T) {
	t.Parallel()

	// The default directory resolves differently across starts, like
	// an agent whose manifest loads between a dispatch and its retry.
	dirs := make(chan string, 2)
	dirs <- ""
	dirs <- t.TempDir()
	m := newTestManager(t, withWorkingDir(func() string { return <-dirs }))

	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-dir-drift",
	}
	proc := requireStarted(t, m, req, "chat-1")
	<-proc.done

	require.Equal(t, proc.id, requireAttached(t, m, req, "chat-1").id)
}

func TestUnkeyedProcessesReapSooner(t *testing.T) {
	t.Parallel()

	mClock := quartz.NewMock(t)
	m := newTestManager(t, withMockClock(mClock))

	plain := requireStarted(t, m, workspacesdk.StartProcessRequest{
		Command: "echo hello",
	}, "chat-1")
	<-plain.done

	keyed := requireStarted(t, m, workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-1",
	}, "chat-1")
	<-keyed.done

	// Past exitedProcessReapAge but within keyedProcessReapAge.
	mClock.Advance(exitedProcessReapAge + time.Minute)
	m.mu.Lock()
	m.reapExitedLocked(mClock.Now())
	m.mu.Unlock()

	require.False(t, requireTracked(t, m, plain.id))
	require.True(t, requireTracked(t, m, keyed.id))
}

func TestConcurrentSameKeyStartsOnce(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-race",
	}

	const workers = 8
	results := make([]<-chan startResult, 0, workers)
	for range workers {
		results = append(results, startAsync(context.Background(), m, req, "chat-1"))
	}

	ids := make(map[string]struct{})
	attaches := 0
	for _, result := range results {
		res := awaitStart(t, result)
		require.NoError(t, res.err)
		ids[res.proc.id] = struct{}{}
		if res.attached {
			attaches++
		}
	}
	require.Len(t, ids, 1)
	require.Equal(t, workers-1, attaches)

	m.mu.Lock()
	procCount := len(m.procs)
	m.mu.Unlock()
	require.Equal(t, 1, procCount)
}

func TestSameKeyWaiterHonorsCancellation(t *testing.T) {
	t.Parallel()

	gate := newStartGate(t)
	m := newTestManager(t, gate.option())
	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-cancel",
	}

	firstStart := startAsync(context.Background(), m, req, "chat-1")
	gate.waitStalled(t)

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiter := startAsync(waiterCtx, m, req, "chat-1")
	cancelWaiter()

	res := awaitStart(t, waiter)
	require.ErrorIs(t, res.err, context.Canceled)
	// The sentinel routes the failure to a 409: the stalled start may
	// still publish a process, so callers must not treat the dispatch
	// as failed.
	require.ErrorIs(t, res.err, agentrunonce.ErrPublicationPending)

	// The stalled start is unaffected by the waiter's cancellation.
	gate.release()
	require.NoError(t, awaitStart(t, firstStart).err)

	proc := requireAttached(t, m, req, "chat-1")
	<-proc.done
}

func TestSameKeyInDifferentChatsStartsIndependently(t *testing.T) {
	t.Parallel()

	gate := newStartGate(t)
	m := newTestManager(t, gate.option())
	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-cross-chat",
	}

	chatA := startAsync(context.Background(), m, req, "chat-a")
	gate.waitStalled(t)

	// Chat B reserves its own key even while chat A's reservation is
	// pending, rather than waiting on it or conflicting with it.
	procB := requireStarted(t, m, req, "chat-b")

	gate.release()
	resA := awaitStart(t, chatA)
	require.NoError(t, resA.err)
	require.False(t, resA.attached)
	require.NotEqual(t, procB.id, resA.proc.id)
	require.Equal(t, 2, m.runOnce.Len())
}

func TestSameKeyWaiterUnblocksOnClose(t *testing.T) {
	t.Parallel()

	gate := newStartGate(t)
	m := newTestManager(t, gate.option())
	req := workspacesdk.StartProcessRequest{
		Command:        "echo hello",
		IdempotencyKey: "key-close",
	}

	firstStart := startAsync(context.Background(), m, req, "chat-1")
	gate.waitStalled(t)
	waiter := startAsync(context.Background(), m, req, "chat-1")

	require.NoError(t, m.Close())

	res := awaitStart(t, waiter)
	require.ErrorContains(t, res.err, "manager is closed")
	require.ErrorIs(t, res.err, agentrunonce.ErrPublicationPending)

	// The stalled start never reached the spawn, so it reports the
	// close instead of dispatching a command after shutdown.
	gate.release()
	require.ErrorContains(t, awaitStart(t, firstStart).err, "manager is closed")
}

func TestCloseBeforeSpawnAbortsStart(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		req  workspacesdk.StartProcessRequest
	}{
		{
			name: "Keyed",
			req: workspacesdk.StartProcessRequest{
				Command:        "sleep 600",
				IdempotencyKey: "key-close-race",
			},
		},
		{
			name: "Unkeyed",
			req:  workspacesdk.StartProcessRequest{Command: "sleep 600"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gate := newStartGate(t)
			m := newTestManager(t, gate.option())

			start := startAsync(context.Background(), m, tc.req, "chat-1")
			gate.waitStalled(t)
			require.NoError(t, m.Close())
			gate.release()

			// A start that loses the race to Close never runs the
			// command, and leaves no process or reservation behind.
			res := awaitStart(t, start)
			require.ErrorContains(t, res.err, "manager is closed")
			require.Nil(t, res.proc)

			m.mu.Lock()
			published := len(m.procs)
			m.mu.Unlock()
			require.Zero(t, published)
			require.Zero(t, m.runOnce.Len())
		})
	}
}

func TestSignalByKeyWaitsForPendingStart(t *testing.T) {
	t.Parallel()

	gate := newStartGate(t)
	m := newTestManager(t, gate.option())
	req := workspacesdk.StartProcessRequest{
		Command:        "sleep 600",
		IdempotencyKey: "key-pending",
		Background:     true,
	}

	start := startAsync(context.Background(), m, req, "chat-1")
	gate.waitStalled(t)

	// The reservation exists but names no process yet. This is the
	// expected state on the by-key path, because that path is used
	// precisely when the start response was lost.
	signalErr := make(chan error, 1)
	go func() {
		signalErr <- m.signalByKey(context.Background(), "chat-1", "key-pending", "kill")
	}()

	// Nothing resolves until the start publishes.
	select {
	case err := <-signalErr:
		t.Fatalf("signal resolved before the start published: %v", err)
	case <-time.After(testutil.IntervalMedium):
	}

	gate.release()
	res := awaitStart(t, start)
	require.NoError(t, res.err)

	select {
	case err := <-signalErr:
		require.NoError(t, err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("signal did not return after the start published")
	}
	<-res.proc.done
}

func TestSignalByKeyReportsPendingAtDeadline(t *testing.T) {
	t.Parallel()

	gate := newStartGate(t)
	m := newTestManager(t, gate.option())
	req := workspacesdk.StartProcessRequest{
		Command:        "sleep 600",
		IdempotencyKey: "key-pending-deadline",
		Background:     true,
	}

	start := startAsync(context.Background(), m, req, "chat-1")
	gate.waitStalled(t)

	// A caller that gives up must not learn "not found", which would
	// let it record the command as gone while it is about to run.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := m.signalByKey(ctx, "chat-1", "key-pending-deadline", "kill")
	require.ErrorIs(t, err, errProcessStartPending)
	require.NotErrorIs(t, err, errProcessNotFound)

	gate.release()
	require.NoError(t, awaitStart(t, start).err)
}

func TestSignalByKeyUnknownKeyNotFound(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	err := m.signalByKey(context.Background(), "chat-1", "key-missing", "kill")
	require.ErrorIs(t, err, errProcessNotFound)
}

func TestSignalByKeyIsChatScoped(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	proc := requireStarted(t, m, workspacesdk.StartProcessRequest{
		Command:        "sleep 600",
		IdempotencyKey: "key-1",
		Background:     true,
	}, "chat-a")

	require.ErrorIs(t, m.signalByKey(context.Background(), "chat-b", "key-1", "kill"), errProcessNotFound)
	require.True(t, proc.info().Running)

	require.NoError(t, m.signalByKey(context.Background(), "chat-a", "key-1", "kill"))
	<-proc.done
}
