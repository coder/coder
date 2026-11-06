package agentcontext_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
	errOnce error
	signal  chan struct{}
	release <-chan struct{}
}

func newFakePusher() *fakePusher {
	return &fakePusher{
		resp:   &agentcontext.PushResponse{Accepted: true},
		signal: make(chan struct{}, 16),
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
	case p.signal <- struct{}{}:
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

	for _, tc := range []struct {
		name      string
		changed   bool
		supersede bool
	}{
		{name: "Unchanged"},
		{name: "Changed", changed: true},
		{name: "Superseded", changed: true, supersede: true},
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
				release := make(chan struct{})
				p.release = release
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				pushDone := make(chan error, 1)
				go func() {
					pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{Logger: testutil.Logger(t)})
				}()
				<-p.signal

				if tc.changed {
					mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "second")
				}
				done := make(chan agentcontext.Snapshot, 1)
				go func() {
					snap, err := m.Resync(ctx)
					assert.NoError(t, err)
					done <- snap
				}()
				synctest.Wait()
				require.Empty(t, done, "resync must wait for its push")

				release <- struct{}{}
				<-p.signal
				synctest.Wait()
				require.Empty(t, done, "an older acknowledgement must not release resync")

				laterDone := make(chan error, 1)
				if tc.supersede {
					go func() {
						_, err := m.Resync(ctx)
						laterDone <- err
					}()
					synctest.Wait()
				}
				release <- struct{}{}
				snap := <-done
				require.Equal(t, uint64(2), snap.Version)
				require.Equal(t, p.snapshot()[1].AggregateHash, snap.AggregateHash)
				if tc.supersede {
					<-p.signal
					release <- struct{}{}
					require.NoError(t, <-laterDone)
				}
				cancel()
				require.ErrorIs(t, <-pushDone, context.Canceled)
			})
		})
	}
}

func TestManager_ResyncWaitsForNewerResolve(t *testing.T) {
	t.Parallel()
	for _, cancelNewer := range []bool{false, true} {
		t.Run(fmt.Sprintf("CancelNewer=%t", cancelNewer), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				started := make(chan int, 2)
				releases := []chan struct{}{nil, make(chan struct{}), make(chan struct{})}
				var calls atomic.Int32
				m := newTestManager(t, agentcontext.ManagerOptions{
					MCPCatalog: func() []agentcontext.MCPServerStatus {
						call := int(calls.Add(1)) - 1
						if call > 0 {
							started <- call
							select {
							case <-releases[call]:
							case <-t.Context().Done():
							}
						}
						return []agentcontext.MCPServerStatus{{Name: fmt.Sprintf("pass-%d", call)}}
					},
				})
				startTestPush(t, m)
				type result struct {
					snap agentcontext.Snapshot
					err  error
				}
				older := make(chan result, 1)
				newer := make(chan result, 1)
				go func() {
					snap, err := m.Resync(t.Context())
					older <- result{snap: snap, err: err}
				}()
				require.Equal(t, 1, <-started)
				newerCtx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go func() {
					snap, err := m.Resync(newerCtx)
					newer <- result{snap: snap, err: err}
				}()
				require.Equal(t, 2, <-started)
				close(releases[1])
				synctest.Wait()
				require.Empty(t, older, "the published snapshot predates both resync passes")

				if cancelNewer {
					cancel()
					synctest.Wait()
					require.Len(t, newer, 1, "caller cancellation must not wait for collection")
				}
				close(releases[2])
				newerResult := <-newer
				if cancelNewer {
					require.ErrorIs(t, newerResult.err, context.Canceled)
				} else {
					require.NoError(t, newerResult.err)
					require.Equal(t, uint64(2), newerResult.snap.Version)
				}
				olderResult := <-older
				require.NoError(t, olderResult.err, "a canceled newer caller must not strand a live resync")
				require.Equal(t, uint64(2), olderResult.snap.Version)
				require.NotNil(t, findMCPServerResource(olderResult.snap, "pass-2"))
			})
		})
	}
}

func TestManager_ResyncPushWaitErrors(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"NoPushLoop", "Canceled", "Closed"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTimer()
			defer trap.Close()
			m := newTestManager(t, agentcontext.ManagerOptions{Clock: clock})
			ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
			defer cancel()

			done := make(chan error, 1)
			go func() {
				_, err := m.Resync(ctx)
				done <- err
			}()
			waitCtx := testutil.Context(t, testutil.WaitLong)
			trap.MustWait(waitCtx).MustRelease(waitCtx)
			want := context.DeadlineExceeded
			switch mode {
			case "Canceled":
				cancel()
				want = context.Canceled
			case "Closed":
				require.NoError(t, m.Close())
				want = agentcontext.ErrManagerClosed
			default:
				_, advanced := clock.AdvanceNext()
				advanced.MustWait(waitCtx)
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, want)
				if mode == "NoPushLoop" {
					require.ErrorContains(t, err, "no acknowledged context snapshot within 30s")
					require.ErrorContains(t, err, "disconnected from coderd or context push may be failing")
				}
			case <-waitCtx.Done():
				t.Fatal("resync did not stop waiting")
			}
		})
	}
}

func startTestPush(t *testing.T, m *agentcontext.Manager) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- m.RunPush(ctx, newFakePusher(), agentcontext.PushOptions{Logger: testutil.Logger(t)})
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
	case <-p.signal:
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
	<-p.signal

	// Trigger a resync via Resync.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v2"), 0o600))
	_, err := m.Resync(ctx)
	require.NoError(t, err)

	// Second push.
	select {
	case <-p.signal:
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
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		pushDone := make(chan error, 1)
		go func() {
			pushDone <- m.RunPush(ctx, p, agentcontext.PushOptions{Logger: testutil.Logger(t)})
		}()
		<-p.signal
		resyncDone := make(chan error, 1)
		go func() {
			_, err := m.Resync(ctx)
			resyncDone <- err
		}()
		synctest.Wait()
		release <- struct{}{}
		require.NoError(t, <-pushDone, "Unimplemented must stop the loop cleanly")
		synctest.Wait()
		select {
		case err := <-resyncDone:
			require.ErrorIs(t, err, agentcontext.ErrPushUnavailable)
		default:
			t.Fatal("an unimplemented push did not wake the resync waiter")
		}
		_, err := m.Resync(ctx)
		require.ErrorIs(t, err, agentcontext.ErrPushUnavailable)

		startTestPush(t, m)
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
	case <-p.signal:
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
	case <-p.signal:
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
	case <-p.signal:
	case <-ctx.Done():
		t.Fatalf("initial push never landed: %v", ctx.Err())
	}

	// Trigger a content change so a second push lands. Without
	// the change, the loop should remain parked.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v2"), 0o600))
	_, err := m.Resync(ctx)
	require.NoError(t, err)

	select {
	case <-p.signal:
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
	case <-p.signal:
		t.Fatal("push loop shipped a snapshot before SetReady")
	case <-time.After(testutil.IntervalMedium):
	}
	require.Empty(t, p.snapshot(), "no push must happen before the gate releases")

	// Startup completes; the gate releases and the first real snapshot
	// is pushed with Initial=true.
	m.SetReady()

	select {
	case <-p.signal:
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
