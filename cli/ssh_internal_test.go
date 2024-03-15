package cli

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	fakeOwnerName     = "fake-owner-name"
	fakeServerURL     = "https://fake-foo-url"
	fakeWorkspaceName = "fake-workspace-name"
)

func TestVerifyWorkspaceOutdated(t *testing.T) {
	t.Parallel()

	serverURL, err := url.Parse(fakeServerURL)
	require.NoError(t, err)

	client := codersdk.Client{URL: serverURL}

	t.Run("Up-to-date", func(t *testing.T) {
		t.Parallel()

		workspace := codersdk.Workspace{Name: fakeWorkspaceName, OwnerName: fakeOwnerName}

		_, outdated := verifyWorkspaceOutdated(&client, workspace)

		assert.False(t, outdated, "workspace should be up-to-date")
	})
	t.Run("Outdated", func(t *testing.T) {
		t.Parallel()

		workspace := codersdk.Workspace{Name: fakeWorkspaceName, OwnerName: fakeOwnerName, Outdated: true}

		updateWorkspaceBanner, outdated := verifyWorkspaceOutdated(&client, workspace)

		assert.True(t, outdated, "workspace should be outdated")
		assert.NotEmpty(t, updateWorkspaceBanner, "workspace banner should be present")
	})
}

func TestBuildWorkspaceLink(t *testing.T) {
	t.Parallel()

	serverURL, err := url.Parse(fakeServerURL)
	require.NoError(t, err)

	workspace := codersdk.Workspace{Name: fakeWorkspaceName, OwnerName: fakeOwnerName}
	workspaceLink := buildWorkspaceLink(serverURL, workspace)

	assert.Equal(t, workspaceLink.String(), fakeServerURL+"/@"+fakeOwnerName+"/"+fakeWorkspaceName)
}

func TestCloserStack_Mainline(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := newCloserStack(ctx, logger)
	closes := new([]*fakeCloser)
	fc0 := &fakeCloser{closes: closes}
	fc1 := &fakeCloser{closes: closes}

	func() {
		defer uut.close(nil)
		err := uut.push("fc0", fc0)
		require.NoError(t, err)
		err = uut.push("fc1", fc1)
		require.NoError(t, err)
	}()
	// order reversed
	require.Equal(t, []*fakeCloser{fc1, fc0}, *closes)
}

func TestCloserStack_Context(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	uut := newCloserStack(ctx, logger)
	closes := new([]*fakeCloser)
	fc0 := &fakeCloser{closes: closes}
	fc1 := &fakeCloser{closes: closes}

	err := uut.push("fc0", fc0)
	require.NoError(t, err)
	err = uut.push("fc1", fc1)
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		uut.Lock()
		defer uut.Unlock()
		return uut.closed
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestCloserStack_PushAfterClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	uut := newCloserStack(ctx, logger)
	closes := new([]*fakeCloser)
	fc0 := &fakeCloser{closes: closes}
	fc1 := &fakeCloser{closes: closes}

	err := uut.push("fc0", fc0)
	require.NoError(t, err)

	exErr := xerrors.New("test")
	uut.close(exErr)
	require.Equal(t, []*fakeCloser{fc0}, *closes)

	err = uut.push("fc1", fc1)
	require.ErrorIs(t, err, exErr)
	require.Equal(t, []*fakeCloser{fc1, fc0}, *closes, "should close fc1")
}

func TestCloserStack_CloseAfterContext(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	uut := newCloserStack(ctx, logger)
	ac := &asyncCloser{
		t:        t,
		ctx:      testCtx,
		complete: make(chan struct{}),
		started:  make(chan struct{}),
	}
	err := uut.push("async", ac)
	require.NoError(t, err)
	cancel()
	testutil.RequireRecvCtx(testCtx, t, ac.started)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		uut.close(nil)
	}()

	// since the asyncCloser is still waiting, we shouldn't complete uut.close()
	select {
	case <-time.After(testutil.IntervalFast):
		// OK!
	case <-closed:
		t.Fatal("closed before stack was finished")
	}

	// complete the asyncCloser
	close(ac.complete)
	testutil.RequireRecvCtx(testCtx, t, closed)
}

type fakeCloser struct {
	closes *[]*fakeCloser
	err    error
}

func (c *fakeCloser) Close() error {
	*c.closes = append(*c.closes, c)
	return c.err
}

type asyncCloser struct {
	t        *testing.T
	ctx      context.Context
	started  chan struct{}
	complete chan struct{}
}

func (c *asyncCloser) Close() error {
	close(c.started)
	select {
	case <-c.ctx.Done():
		c.t.Error("timed out")
		return c.ctx.Err()
	case <-c.complete:
		return nil
	}
}
