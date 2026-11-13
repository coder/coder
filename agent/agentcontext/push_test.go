package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const (
	initialPushVersion  uint64 = 1
	resyncVersion       uint64 = 2
	newerPassServerName        = "pass-2"
)

// fakePusher records every push and lets the test control the
// returned response and error.
type fakePusher struct {
	mu       sync.Mutex
	requests []*agentcontext.PushRequest
	resp     *agentcontext.PushResponse
	err      error
	// errOnce is non-nil to simulate a single transient
	// failure followed by success.
	errOnce     error
	pushStarted chan *agentcontext.PushRequest
	// release blocks each recorded push until the test sends an acknowledgement.
	release <-chan struct{}
}

func newFakePusher() *fakePusher {
	return &fakePusher{
		resp:        &agentcontext.PushResponse{Accepted: true},
		pushStarted: make(chan *agentcontext.PushRequest, 16),
	}
}

func (p *fakePusher) PushContextState(ctx context.Context, req *agentcontext.PushRequest) (*agentcontext.PushResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	if p.errOnce != nil {
		err := p.errOnce
		p.errOnce = nil
		p.mu.Unlock()
		return nil, err
	}
	select {
	case p.pushStarted <- req:
	default:
	}
	resp, err := p.resp, p.err
	p.mu.Unlock()
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return resp, err
}

func (p *fakePusher) snapshot() []*agentcontext.PushRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*agentcontext.PushRequest, len(p.requests))
	copy(out, p.requests)
	return out
}

func TestManager_ResyncWaitsForPush(t *testing.T) {
	t.Parallel()

	// Even unchanged content must wait for acknowledgement of the new version.
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "Unchanged", content: "first"},
		{name: "Changed", content: "second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				dir := t.TempDir()
				mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "first")
				m := newTestManager(t, agentcontext.ManagerOptions{
					WorkingDir: func() string { return dir },
				})
				p := newFakePusher()
				acknowledgePush := make(chan struct{})
				p.release = acknowledgePush
				startTestPush(t, m, p)

				// Hold the initial push while the resync publishes its version.
				initialPush := <-p.pushStarted
				require.Equal(t, initialPushVersion, initialPush.Version)
				mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), tc.content)
				done := startResync(t.Context(), m)
				synctest.Wait()
				require.Empty(t, done, "resync must wait for its push")

				// Acknowledging the initial version must not release the resync.
				acknowledgePush <- struct{}{}
				resyncPush := <-p.pushStarted
				require.Equal(t, resyncVersion, resyncPush.Version)
				synctest.Wait()
				require.Empty(t, done, "an older acknowledgement must not release resync")

				// Acknowledging the resync's version releases its waiter.
				acknowledgePush <- struct{}{}
				result := testutil.RequireReceive(t.Context(), t, done)
				require.NoError(t, result.err)
				require.Equal(t, resyncVersion, result.snapshot.Version)
				require.Equal(t, resyncPush.AggregateHash, result.snapshot.AggregateHash)
				require.Len(t, result.snapshot.Resources, 1)
				require.Equal(t, tc.content, string(result.snapshot.Resources[0].Payload))
			})
		})
	}

	t.Run("Superseded", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			m := newTestManager(t, agentcontext.ManagerOptions{})
			p := newFakePusher()
			acknowledgePush := make(chan struct{})
			p.release = acknowledgePush
			startTestPush(t, m, p)

			// Hold the initial push while the first resync starts waiting.
			initialPush := <-p.pushStarted
			require.Equal(t, initialPushVersion, initialPush.Version)
			first := startResync(t.Context(), m)
			synctest.Wait()
			require.Empty(t, first)
			acknowledgePush <- struct{}{}
			resyncPush := <-p.pushStarted
			require.Equal(t, resyncVersion, resyncPush.Version)
			synctest.Wait()
			require.Empty(t, first)

			// A later resolve must not extend the first resync's target.
			later := startResync(t.Context(), m)
			synctest.Wait()
			acknowledgePush <- struct{}{}
			firstResult := testutil.RequireReceive(t.Context(), t, first)
			require.NoError(t, firstResult.err)
			require.Equal(t, resyncVersion, firstResult.snapshot.Version)

			// The later resync still waits for its own acknowledgement.
			laterPush := <-p.pushStarted
			require.Equal(t, resyncVersion+1, laterPush.Version)
			require.Empty(t, later)
			acknowledgePush <- struct{}{}
			laterResult := testutil.RequireReceive(t.Context(), t, later)
			require.NoError(t, laterResult.err)
			require.Equal(t, laterPush.Version, laterResult.snapshot.Version)
		})
	})
}

func TestManager_ResyncWaitsForNewerResolve(t *testing.T) {
	t.Parallel()

	t.Run("NewerSucceeds", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			resyncs := startOverlappingResyncs(t)
			close(resyncs.olderPassRelease)
			synctest.Wait()
			require.Empty(t, resyncs.olderDone, "the published snapshot predates both resync passes")

			close(resyncs.newerPassRelease)
			newerResult := testutil.RequireReceive(t.Context(), t, resyncs.newerDone)
			require.NoError(t, newerResult.err)
			require.Equal(t, resyncVersion, newerResult.snapshot.Version)
			olderResult := testutil.RequireReceive(t.Context(), t, resyncs.olderDone)
			require.NoError(t, olderResult.err)
			require.Equal(t, resyncVersion, olderResult.snapshot.Version)
			require.NotNil(t, findMCPServerResource(olderResult.snapshot, newerPassServerName))
		})
	})

	t.Run("NewerCanceled", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			resyncs := startOverlappingResyncs(t)
			close(resyncs.olderPassRelease)
			synctest.Wait()
			require.Empty(t, resyncs.olderDone, "the published snapshot predates both resync passes")

			resyncs.cancelNewer()
			synctest.Wait()
			require.Len(t, resyncs.newerDone, 1, "caller cancellation must not wait for the resolve pass")
			newerResult := testutil.RequireReceive(t.Context(), t, resyncs.newerDone)
			require.ErrorIs(t, newerResult.err, context.Canceled)

			close(resyncs.newerPassRelease)
			olderResult := testutil.RequireReceive(t.Context(), t, resyncs.olderDone)
			require.NoError(t, olderResult.err, "a canceled newer caller must not strand a live resync")
			require.Equal(t, resyncVersion, olderResult.snapshot.Version)
			require.NotNil(t, findMCPServerResource(olderResult.snapshot, newerPassServerName))
		})
	})
}

type overlappingResyncs struct {
	olderPassRelease chan struct{}
	newerPassRelease chan struct{}
	olderDone        <-chan resyncResult
	newerDone        <-chan resyncResult
	cancelNewer      context.CancelFunc
}

func startOverlappingResyncs(t *testing.T) overlappingResyncs {
	t.Helper()
	olderPassStarted := make(chan struct{})
	newerPassStarted := make(chan struct{})
	resyncs := overlappingResyncs{
		olderPassRelease: make(chan struct{}),
		newerPassRelease: make(chan struct{}),
	}
	var calls atomic.Int32
	m := newTestManager(t, agentcontext.ManagerOptions{
		MCPCatalog: func() []agentcontext.MCPServerStatus {
			switch calls.Add(1) {
			case 1:
				// SetReady's initial resolve pass is not blocked.
				return []agentcontext.MCPServerStatus{{Name: "pass-0"}}
			case 2:
				close(olderPassStarted)
				select {
				case <-resyncs.olderPassRelease:
				case <-t.Context().Done():
				}
				return []agentcontext.MCPServerStatus{{Name: "pass-1"}}
			case 3:
				close(newerPassStarted)
				select {
				case <-resyncs.newerPassRelease:
				case <-t.Context().Done():
				}
				return []agentcontext.MCPServerStatus{{Name: newerPassServerName}}
			default:
				t.Error("unexpected resolve pass")
				return nil
			}
		},
	})
	startTestPush(t, m, newFakePusher())
	resyncs.olderDone = startResync(t.Context(), m)
	testutil.TryReceive(t.Context(), t, olderPassStarted)
	newerCtx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	resyncs.cancelNewer = cancel
	resyncs.newerDone = startResync(newerCtx, m)
	testutil.TryReceive(t.Context(), t, newerPassStarted)
	return resyncs
}

func TestManager_ResyncPushWaitErrors(t *testing.T) {
	t.Parallel()

	t.Run("NoPushLoop", func(t *testing.T) {
		t.Parallel()
		waiter := startWaitingResync(t)
		_, advanced := waiter.clock.AdvanceNext()
		advanced.MustWait(waiter.ctx)
		result := testutil.RequireReceive(waiter.ctx, t, waiter.done)
		require.ErrorIs(t, result.err, context.DeadlineExceeded)
		require.ErrorContains(t, result.err, "no acknowledged context snapshot within 30s")
		require.ErrorContains(t, result.err, "disconnected from coderd or context push may be failing")
		require.ErrorContains(t, result.err, "check the agent logs for context push errors")
	})

	t.Run("Canceled", func(t *testing.T) {
		t.Parallel()
		waiter := startWaitingResync(t)
		waiter.cancel()
		result := testutil.RequireReceive(waiter.ctx, t, waiter.done)
		require.ErrorIs(t, result.err, context.Canceled)
	})

	t.Run("CanceledWithAcknowledgement", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTimer()
			defer trap.Close()
			m := newTestManager(t, agentcontext.ManagerOptions{Clock: clock})
			p := newFakePusher()
			startTestPush(t, m, p)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := startResync(ctx, m)

			// Pause the waiter before it can observe the acknowledgement.
			call := trap.MustWait(t.Context())
			synctest.Wait()
			pushes := p.snapshot()
			require.NotEmpty(t, pushes)
			require.Equal(t, resyncVersion, pushes[len(pushes)-1].Version)

			cancel()
			call.MustRelease(t.Context())
			result := testutil.RequireReceive(t.Context(), t, done)
			require.ErrorIs(t, result.err, context.Canceled)
		})
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()
		waiter := startWaitingResync(t)
		require.NoError(t, waiter.manager.Close())
		result := testutil.RequireReceive(waiter.ctx, t, waiter.done)
		require.ErrorIs(t, result.err, agentcontext.ErrManagerClosed)
	})
}

type waitingResync struct {
	manager *agentcontext.Manager
	clock   *quartz.Mock
	ctx     context.Context
	cancel  context.CancelFunc
	done    <-chan resyncResult
}

func startWaitingResync(t *testing.T) waitingResync {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer()
	defer trap.Close()
	m := newTestManager(t, agentcontext.ManagerOptions{Clock: clock})
	resyncCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	done := startResync(resyncCtx, m)
	trap.MustWait(ctx).MustRelease(ctx)
	return waitingResync{manager: m, clock: clock, ctx: ctx, cancel: cancel, done: done}
}

type resyncResult struct {
	snapshot agentcontext.Snapshot
	err      error
}

func startResync(ctx context.Context, m *agentcontext.Manager) <-chan resyncResult {
	done := make(chan resyncResult, 1)
	go func() {
		snapshot, err := m.Resync(ctx)
		done <- resyncResult{snapshot: snapshot, err: err}
	}()
	return done
}

func startTestPush(t *testing.T, m *agentcontext.Manager, p agentcontext.Pusher) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- m.RunPush(ctx, p, agentcontext.PushOptions{Logger: testutil.Logger(t)})
	}()
	t.Cleanup(func() {
		cancel()
		err := <-done
		if err != nil {
			require.ErrorIs(t, err, context.Canceled)
		}
	})
}

func TestRunPush_FirstPushIsInitial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v1"), 0o600))

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	p := newFakePusher()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()

	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger: testutil.Logger(t).Named("push"),
		})
	}()

	// Wait for the first push.
	select {
	case <-p.pushStarted:
	case <-time.After(testutil.WaitShort):
		t.Fatalf("expected initial push")
	}

	requests := p.snapshot()
	require.Len(t, requests, 1)
	require.True(t, requests[0].Initial, "first push must be initial")
	require.Equal(t, uint64(1), requests[0].Version)

	cancel()
	require.ErrorIs(t, <-pushDone, context.Canceled)
}

func TestRunPush_SubsequentPushOnChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v1"), 0o600))

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	p := newFakePusher()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()

	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger: testutil.Logger(t).Named("push"),
		})
	}()

	// Initial push.
	<-p.pushStarted

	// Trigger a resync via Resync.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v2"), 0o600))
	_, err := m.Resync(ctx)
	require.NoError(t, err)

	// Second push.
	select {
	case <-p.pushStarted:
	case <-time.After(testutil.WaitShort):
		t.Fatalf("expected second push after resync")
	}

	requests := p.snapshot()
	require.GreaterOrEqual(t, len(requests), 2)
	require.False(t, requests[1].Initial, "subsequent pushes must not be Initial")
	require.NotEqual(t, requests[0].AggregateHash, requests[1].AggregateHash,
		"second push must reflect the v2 content, not a duplicate of the first snapshot")
	require.Greater(t, requests[1].Version, requests[0].Version,
		"version must advance between snapshots")

	cancel()
	require.ErrorIs(t, <-pushDone, context.Canceled)
}

func TestRunPush_StopsOnUnimplemented(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newTestManager(t, agentcontext.ManagerOptions{})
		p := newFakePusher()
		p.err = agentcontext.ErrPushUnimplemented
		release := make(chan struct{})
		p.release = release
		// Keep the first push in flight until a resync is waiting.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		pushDone := make(chan error, 1)
		go func() {
			pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{Logger: testutil.Logger(t)})
		}()
		<-p.pushStarted
		resyncDone := startResync(ctx, m)
		synctest.Wait()

		// An unavailable push stops the loop and wakes its resync waiter.
		release <- struct{}{}
		require.NoError(t, <-pushDone, "Unimplemented must stop the loop cleanly")
		synctest.Wait()
		select {
		case result := <-resyncDone:
			require.ErrorIs(t, result.err, agentcontext.ErrPushUnavailable)
		default:
			t.Fatal("an unimplemented push did not wake the resync waiter")
		}
		_, err := m.Resync(ctx)
		require.ErrorIs(t, err, agentcontext.ErrPushUnavailable)

		// A later successful push clears the unavailable state.
		startTestPush(t, m, newFakePusher())
		synctest.Wait()
		_, err = m.Resync(ctx)
		require.NoError(t, err, "a successful push must restore availability")
	})
}

func TestRunPush_RetriesTransientError(t *testing.T) {
	t.Parallel()
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTimer()
	defer trap.Close()

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	p := newFakePusher()
	p.errOnce = xerrors.New("transient")

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()
	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger:         testutil.Logger(t).Named("push"),
			InitialBackoff: time.Second,
			Clock:          mClock,
		})
	}()

	// First push hits transient and arms the retry timer. Wait for
	// the timer creation, then advance the clock past the backoff.
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	mClock.Advance(time.Second).MustWait(ctx)

	select {
	case <-p.pushStarted:
	case <-time.After(testutil.WaitShort):
		t.Fatalf("expected push after transient error")
	}
	require.GreaterOrEqual(t, len(p.snapshot()), 2)

	cancel()
	<-pushDone
}

// TestRunPush_ClosesOnManagerClose verifies that calling
// Manager.Close terminates an in-flight RunPush even when the
// caller's context is still live. Without this guarantee the
// agent shutdown would leak a push goroutine until the
// surrounding ctx expired.
func TestRunPush_ClosesOnManagerClose(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	p := newFakePusher()
	ctx := testutil.Context(t, testutil.WaitShort)
	done := make(chan error, 1)
	go func() {
		done <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger: testutil.Logger(t).Named("push"),
		})
	}()

	// Wait for the initial push so the loop is parked on the
	// change channel, then close the Manager and assert that
	// RunPush returns promptly with a nil error.
	select {
	case <-p.pushStarted:
	case <-ctx.Done():
		t.Fatalf("initial push never landed: %v", ctx.Err())
	}
	require.NoError(t, m.Close())

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatalf("RunPush did not return after Manager.Close: %v", ctx.Err())
	}
}

// TestRunPush_RejectedResponseProceeds verifies the contract
// that an Accepted=false response is not retried: pushWithRetry
// returns success and RunPush parks on the next change instead
// of re-sending the same snapshot. A regression that added
// retry-on-reject logic would loop here and fail the test.
func TestRunPush_RejectedResponseProceeds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v1"), 0o600))

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	p := newFakePusher()
	p.resp = &agentcontext.PushResponse{Accepted: false}

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()
	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger: testutil.Logger(t).Named("push"),
		})
	}()

	// Initial push delivered and accepted=false; loop must park
	// on changes, not retry the same payload.
	select {
	case <-p.pushStarted:
	case <-ctx.Done():
		t.Fatalf("initial push never landed: %v", ctx.Err())
	}

	// Trigger a content change so a second push lands. Without
	// the change, the loop should remain parked.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v2"), 0o600))
	_, err := m.Resync(ctx)
	require.NoError(t, err)

	select {
	case <-p.pushStarted:
	case <-ctx.Done():
		t.Fatalf("second push never landed after change: %v", ctx.Err())
	}

	requests := p.snapshot()
	require.GreaterOrEqual(t, len(requests), 2,
		"exactly one push per snapshot; rejection must not double-fire")
	require.NotEqual(t, requests[0].AggregateHash, requests[1].AggregateHash)

	cancel()
	require.ErrorIs(t, <-pushDone, context.Canceled)
}

// TestRunPush_WaitsForReady verifies the push loop ships nothing while
// the Manager is gated and ships the complete inventory once SetReady
// fires, so coderd never sees pre-startup partial state.
func TestRunPush_WaitsForReady(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Content exists from the start, but the Manager is gated: nothing
	// is pushed until SetReady.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules"), 0o600))

	m := newPendingTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
	})

	p := newFakePusher()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	defer cancel()

	pushDone := make(chan error, 1)
	go func() {
		pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{
			Logger: testutil.Logger(t).Named("push"),
		})
	}()

	// While gated, no push must be sent even though AGENTS.md exists.
	select {
	case <-p.pushStarted:
		t.Fatal("push loop shipped a snapshot before SetReady")
	case <-time.After(testutil.IntervalMedium):
	}
	require.Empty(t, p.snapshot(), "no push must happen before the gate releases")

	// Startup completes; the gate releases and the first real snapshot
	// is pushed with Initial=true.
	m.SetReady()

	select {
	case <-p.pushStarted:
	case <-time.After(testutil.WaitShort):
		t.Fatal("expected a push after SetReady")
	}

	requests := p.snapshot()
	require.NotEmpty(t, requests)
	first := requests[0]
	require.True(t, first.Initial, "first push after the gate releases must be Initial")
	require.NotEmpty(t, first.Resources, "first push must carry the resolved inventory")
	require.Empty(t, first.SnapshotError, "first push must not carry a transient snapshot error")

	cancel()
	require.ErrorIs(t, <-pushDone, context.Canceled)
}

func TestRunPush_NilPusherErrors(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})
	err := m.RunPush(context.Background(), nil, agentcontext.PushOptions{
		Logger: testutil.Logger(t).Named("push"),
	})
	require.Error(t, err)
}
