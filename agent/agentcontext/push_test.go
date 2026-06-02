package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
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
}

func newFakePusher() *fakePusher {
	return &fakePusher{
		resp:   &agentcontext.PushResponse{Accepted: true},
		signal: make(chan struct{}, 16),
	}
}

func (p *fakePusher) PushContextState(_ context.Context, req *agentcontext.PushRequest) (*agentcontext.PushResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.errOnce != nil {
		err := p.errOnce
		p.errOnce = nil
		return nil, err
	}
	select {
	case p.signal <- struct{}{}:
	default:
	}
	return p.resp, p.err
}

func (p *fakePusher) snapshot() []*agentcontext.PushRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*agentcontext.PushRequest, len(p.requests))
	copy(out, p.requests)
	return out
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

	cancel()
	require.ErrorIs(t, <-pushDone, context.Canceled)
}

func TestRunPush_StopsOnUnimplemented(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	p := newFakePusher()
	p.err = agentcontext.ErrPushUnimplemented

	ctx := testutil.Context(t, testutil.WaitShort)
	err := m.RunPush(ctx, p, agentcontext.PushOptions{
		Logger: testutil.Logger(t).Named("push"),
	})
	require.NoError(t, err, "Unimplemented must stop the loop cleanly")
}

func TestRunPush_RetriesTransientError(t *testing.T) {
	t.Parallel()
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
			InitialBackoff: 10 * time.Millisecond,
		})
	}()

	// First push hits transient, second succeeds.
	select {
	case <-p.signal:
	case <-time.After(testutil.WaitShort):
		t.Fatalf("expected push after transient error")
	}
	require.GreaterOrEqual(t, len(p.snapshot()), 2)

	cancel()
	<-pushDone
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
