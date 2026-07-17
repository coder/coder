package agentproc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestReapFreesClientTokenIndex(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})
	mClock := quartz.NewMock(t)
	m.clock = mClock

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-1",
	}
	proc, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-proc.done

	// The token keeps attaching to the exited process until the
	// reap age passes, so a retried start still sees the result.
	again, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.True(t, attached)
	require.Equal(t, proc.id, again.id)

	mClock.Advance(tokenedProcessReapAge + time.Minute)

	// The sweep on start reaps the exited process, frees its
	// token index entry, and the same token starts fresh.
	fresh, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	require.NotEqual(t, proc.id, fresh.id)

	m.mu.Lock()
	_, tracked := m.procs[proc.id]
	tokenCount := len(m.tokens)
	m.mu.Unlock()
	require.False(t, tracked)
	require.Equal(t, 1, tokenCount)
	<-fresh.done
}

func TestByTokenReapsExitedProcesses(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})
	mClock := quartz.NewMock(t)
	m.clock = mClock

	proc, attached, err := m.start(context.Background(), workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-1",
	}, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-proc.done

	found, _, ok := m.byToken("tok-1", "chat-1")
	require.True(t, ok)
	require.Equal(t, proc.id, found.id)

	mClock.Advance(tokenedProcessReapAge + time.Minute)

	// A probe-only caller must not see a token entry the index
	// would no longer honor: the probe itself reaps the exited
	// process and its token.
	_, pending, ok := m.byToken("tok-1", "chat-1")
	require.False(t, ok)
	require.False(t, pending)

	m.mu.Lock()
	_, tracked := m.procs[proc.id]
	tokenCount := len(m.tokens)
	m.mu.Unlock()
	require.False(t, tracked)
	require.Zero(t, tokenCount)
}

func TestTokenRetryIgnoresDefaultWorkdirDrift(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	// The default directory resolves differently across starts,
	// like an agent whose manifest loads between the original
	// dispatch and its retry.
	dirs := make(chan string, 2)
	dirs <- ""
	dirs <- t.TempDir()
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, func() string { return <-dirs })
	t.Cleanup(func() {
		_ = m.Close()
	})

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-dir-drift",
	}
	proc, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-proc.done

	// An identical retry must attach even though the resolved
	// default directory changed; the request said the same thing
	// both times.
	again, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.True(t, attached)
	require.Equal(t, proc.id, again.id)
}

func TestUntokenedProcessesReapSooner(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})
	mClock := quartz.NewMock(t)
	m.clock = mClock

	plain, attached, err := m.start(context.Background(), workspacesdk.StartProcessRequest{
		Command: "echo hello",
	}, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-plain.done

	tokened, attached, err := m.start(context.Background(), workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-1",
	}, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-tokened.done

	// Past the untokened retention but within the tokened one:
	// the plain process and its output buffer are freed, while
	// the tokened process stays to back dedup and result
	// recovery for retried dispatches.
	mClock.Advance(exitedProcessReapAge + time.Minute)
	m.mu.Lock()
	m.reapExitedLocked(mClock.Now())
	_, plainTracked := m.procs[plain.id]
	_, tokenedTracked := m.procs[tokened.id]
	m.mu.Unlock()
	require.False(t, plainTracked)
	require.True(t, tokenedTracked)
}

func TestByTokenPendingReservation(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})

	// A reservation whose start has not published a process yet
	// must report pending, never a definitive not-found.
	m.mu.Lock()
	m.tokens["tok-pending"] = &tokenEntry{done: make(chan struct{}), chatID: "chat-1"}
	m.mu.Unlock()

	proc, pending, found := m.byToken("tok-pending", "")
	require.Nil(t, proc)
	require.True(t, pending)
	require.False(t, found)

	proc, pending, found = m.byToken("tok-pending", "chat-1")
	require.Nil(t, proc)
	require.True(t, pending)
	require.False(t, found)

	// Another chat's probe must not learn that a reservation for
	// this token exists.
	proc, pending, found = m.byToken("tok-pending", "chat-2")
	require.Nil(t, proc)
	require.False(t, pending)
	require.False(t, found)

	_, pending, found = m.byToken("tok-absent", "")
	require.False(t, pending)
	require.False(t, found)
}

func TestConcurrentSameTokenStartsOnce(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-race",
	}

	const workers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		ids      = make(map[string]struct{})
		attaches int
		errs     []error
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proc, attached, err := m.start(context.Background(), req, "chat-1")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids[proc.id] = struct{}{}
			if attached {
				attaches++
			}
		}()
	}
	wg.Wait()
	require.Empty(t, errs)

	// Every concurrent start with the same token must resolve to
	// the same single process, with exactly one actual spawn.
	require.Len(t, ids, 1)
	require.Equal(t, workers-1, attaches)

	m.mu.Lock()
	procCount := len(m.procs)
	m.mu.Unlock()
	require.Equal(t, 1, procCount)
}

func TestSameTokenWaiterHonorsCancellation(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	// Block the owning start after it reserves the token so a
	// concurrent waiter is stuck behind it.
	updateEnv := func(current []string) ([]string, error) {
		<-release
		return current, nil
	}
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, updateEnv, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-cancel",
	}

	ownerErr := make(chan error, 1)
	go func() {
		_, _, err := m.start(context.Background(), req, "chat-1")
		ownerErr <- err
	}()
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.tokens) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// A canceled waiter must return promptly instead of blocking
	// until the owner finishes.
	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, _, err := m.start(waiterCtx, req, "chat-1")
		waiterErr <- err
	}()
	cancelWaiter()
	select {
	case err := <-waiterErr:
		require.ErrorIs(t, err, context.Canceled)
		// The wait-abort sentinel routes the failure to a 409:
		// the reservation owner may still publish a process, so
		// callers must not treat the dispatch as failed.
		require.ErrorIs(t, err, errTokenWaitAborted)
	case <-time.After(testutil.WaitShort):
		t.Fatal("waiter did not return after cancellation")
	}

	// The owner is unaffected by the waiter's cancellation.
	releaseOnce()
	select {
	case err := <-ownerErr:
		require.NoError(t, err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("owner start did not finish")
	}

	proc, attached, err := m.start(context.Background(), req, "chat-1")
	require.NoError(t, err)
	require.True(t, attached)
	<-proc.done
}

func TestCrossChatPendingTokenRejectedWithoutWaiting(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	// Keep chat A's start pending so its reservation is the only
	// thing chat B can observe.
	updateEnv := func(current []string) ([]string, error) {
		<-release
		return current, nil
	}
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, updateEnv, nil)

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-cross-chat",
	}

	ownerErr := make(chan error, 1)
	go func() {
		_, _, err := m.start(context.Background(), req, "chat-a")
		ownerErr <- err
	}()
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.tokens) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Chat B must fail fast instead of stalling behind chat A's
	// in-flight start only to hit the same mismatch afterwards.
	_, _, err := m.start(context.Background(), req, "chat-b")
	require.ErrorIs(t, err, errClientTokenMismatch)

	releaseOnce()
	select {
	case err := <-ownerErr:
		require.NoError(t, err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("owner start did not finish")
	}
}

func TestSameTokenWaiterUnblocksOnClose(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	// Block the owning start after it reserves the token so a
	// concurrent waiter is stuck behind it when Close runs.
	updateEnv := func(current []string) ([]string, error) {
		<-release
		return current, nil
	}
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, updateEnv, nil)

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-close",
	}

	ownerErr := make(chan error, 1)
	go func() {
		_, _, err := m.start(context.Background(), req, "chat-1")
		ownerErr <- err
	}()
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.tokens) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	waiterErr := make(chan error, 1)
	go func() {
		_, _, err := m.start(context.Background(), req, "chat-1")
		waiterErr <- err
	}()

	require.NoError(t, m.Close())

	// The waiter must not stay stuck behind the blocked owner
	// after the manager shut down.
	select {
	case err := <-waiterErr:
		require.ErrorContains(t, err, "manager is closed")
		require.ErrorIs(t, err, errTokenWaitAborted)
	case <-time.After(testutil.WaitShort):
		t.Fatal("waiter did not return after Close")
	}

	// The owner never reached the spawn before Close, so it
	// reports the close instead of dispatching a command after
	// shutdown.
	releaseOnce()
	select {
	case err := <-ownerErr:
		require.ErrorContains(t, err, "manager is closed")
	case <-time.After(testutil.WaitShort):
		t.Fatal("owner start did not finish")
	}
}

func TestCloseBeforeSpawnAbortsTokenedStart(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	closeManager := make(chan struct{})
	// Block the start between its token reservation and the
	// spawn so Close lands first.
	updateEnv := func(current []string) ([]string, error) {
		close(closeManager)
		<-release
		return current, nil
	}
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, updateEnv, nil)

	req := workspacesdk.StartProcessRequest{
		Command:     "sleep 600",
		ClientToken: "tok-close-race",
	}

	type startResult struct {
		proc *process
		err  error
	}
	ownerDone := make(chan startResult, 1)
	go func() {
		proc, _, err := m.start(context.Background(), req, "chat-1")
		ownerDone <- startResult{proc: proc, err: err}
	}()

	<-closeManager
	require.NoError(t, m.Close())
	releaseOnce()

	var res startResult
	select {
	case res = <-ownerDone:
	case <-time.After(testutil.WaitShort):
		t.Fatal("start did not return after Close")
	}
	// The spawn happens in the same critical section as the
	// closed check, so a start that loses the race to Close never
	// runs the command: the error truthfully reports that nothing
	// was dispatched.
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "manager is closed")
	require.Nil(t, res.proc)

	// Nothing spawned, so nothing may be published and the token
	// must be released: a probe's trusted absence ("nothing
	// started") is accurate, not a green light for a duplicate of
	// something that ran.
	m.mu.Lock()
	published := len(m.procs)
	_, tokenHeld := m.tokens["tok-close-race"]
	m.mu.Unlock()
	require.Zero(t, published)
	require.False(t, tokenHeld)
}

// TestCloseBeforeSpawnAbortsUntokenedStart is the untokened variant:
// the caller sees the close error and nothing is published.
func TestCloseBeforeSpawnAbortsUntokenedStart(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	closeManager := make(chan struct{})
	updateEnv := func(current []string) ([]string, error) {
		close(closeManager)
		<-release
		return current, nil
	}
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, updateEnv, nil)

	req := workspacesdk.StartProcessRequest{
		Command: "sleep 600",
	}

	type startResult struct {
		proc *process
		err  error
	}
	ownerDone := make(chan startResult, 1)
	go func() {
		proc, _, err := m.start(context.Background(), req, "chat-1")
		ownerDone <- startResult{proc: proc, err: err}
	}()

	<-closeManager
	require.NoError(t, m.Close())
	releaseOnce()

	var res startResult
	select {
	case res = <-ownerDone:
	case <-time.After(testutil.WaitShort):
		t.Fatal("start did not return after Close")
	}
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "manager is closed")
	require.Nil(t, res.proc)

	m.mu.Lock()
	published := len(m.procs)
	m.mu.Unlock()
	require.Zero(t, published)
}
