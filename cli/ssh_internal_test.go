package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
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
	logger := testutil.Logger(t)
	uut := newCloserStack(ctx, logger, quartz.NewMock(t))
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

func TestCloserStack_Empty(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	uut := newCloserStack(ctx, logger, quartz.NewMock(t))

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		uut.close(nil)
	}()
	testutil.TryReceive(ctx, t, closed)
}

func TestCloserStack_Context(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger := testutil.Logger(t)
	uut := newCloserStack(ctx, logger, quartz.NewMock(t))
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
	uut := newCloserStack(ctx, logger, quartz.NewMock(t))
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
	uut := newCloserStack(ctx, logger, quartz.NewMock(t))
	ac := newAsyncCloser(testCtx, t)
	defer ac.complete()
	err := uut.push("async", ac)
	require.NoError(t, err)
	cancel()
	testutil.TryReceive(testCtx, t, ac.started)

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

	ac.complete()
	testutil.TryReceive(testCtx, t, closed)
}

func TestCloserStack_Timeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().TickerFunc("closerStack")
	defer trap.Close()
	uut := newCloserStack(ctx, logger, mClock)
	var ac [3]*asyncCloser
	for i := range ac {
		ac[i] = newAsyncCloser(ctx, t)
		err := uut.push(fmt.Sprintf("async %d", i), ac[i])
		require.NoError(t, err)
	}
	defer func() {
		for _, a := range ac {
			a.complete()
		}
	}()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		uut.close(nil)
	}()
	trap.MustWait(ctx).Release()
	// top starts right away, but it hangs
	testutil.TryReceive(ctx, t, ac[2].started)
	// timer pops and we start the middle one
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	testutil.TryReceive(ctx, t, ac[1].started)

	// middle one finishes
	ac[1].complete()
	// bottom starts, but also hangs
	testutil.TryReceive(ctx, t, ac[0].started)

	// timer has to pop twice to time out.
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	testutil.TryReceive(ctx, t, closed)
}

func TestCoderConnectPTY(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	stack := newCloserStack(ctx, logger, quartz.NewMock(t))

	server := newSSHServer("127.0.0.1:0")
	ln, err := net.Listen("tcp", server.server.Addr)
	require.NoError(t, err)

	go func() {
		_ = server.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	ptty := ptytest.New(t)
	ptyDone := make(chan struct{})
	go func() {
		err := runCoderConnectPTY(ctx, ln.Addr().String(), ptty.Output(), ptty.Input(), ptty.Output(), stack)
		assert.NoError(t, err)
		close(ptyDone)
	}()
	ptty.ExpectMatch("Connected!")
	// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
	ptty.WriteLine("exit")
	<-ptyDone
}

func TestCoderConnectStdio(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	stack := newCloserStack(ctx, logger, quartz.NewMock(t))

	clientOutput, clientInput := io.Pipe()
	serverOutput, serverInput := io.Pipe()
	defer func() {
		for _, c := range []io.Closer{clientOutput, clientInput, serverOutput, serverInput} {
			_ = c.Close()
		}
	}()

	server := newSSHServer("127.0.0.1:0")
	ln, err := net.Listen("tcp", server.server.Addr)
	require.NoError(t, err)

	go func() {
		_ = server.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})

	stdioDone := make(chan struct{})
	go func() {
		err = runCoderConnectStdio(ctx, ln.Addr().String(), clientOutput, serverInput, stack)
		assert.NoError(t, err)
		close(stdioDone)
	}()

	conn, channels, requests, err := ssh.NewClientConn(&cliutil.StdioConn{
		Reader: serverOutput,
		Writer: clientInput,
	}, "", &ssh.ClientConfig{
		// #nosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	defer conn.Close()

	sshClient := ssh.NewClient(conn, channels, requests)
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()

	// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
	err = session.Run("exit")
	require.NoError(t, err)
	err = sshClient.Close()
	require.NoError(t, err)
	_ = clientOutput.Close()

	<-stdioDone
}

type sshServer struct {
	server *gliderssh.Server
}

func newSSHServer(addr string) *sshServer {
	return &sshServer{
		server: &gliderssh.Server{
			Addr: addr,
			Handler: func(s gliderssh.Session) {
				_, _ = io.WriteString(s.Stderr(), "Connected!")
			},
		},
	}
}

func (s *sshServer) Serve(ln net.Listener) error {
	return s.server.Serve(ln)
}

func (s *sshServer) Close() error {
	return s.server.Close()
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
	t             *testing.T
	ctx           context.Context
	started       chan struct{}
	isComplete    chan struct{}
	comepleteOnce sync.Once
}

func (c *asyncCloser) Close() error {
	close(c.started)
	select {
	case <-c.ctx.Done():
		c.t.Error("timed out")
		return c.ctx.Err()
	case <-c.isComplete:
		return nil
	}
}

func (c *asyncCloser) complete() {
	c.comepleteOnce.Do(func() { close(c.isComplete) })
}

func newAsyncCloser(ctx context.Context, t *testing.T) *asyncCloser {
	return &asyncCloser{
		t:          t,
		ctx:        ctx,
		isComplete: make(chan struct{}),
		started:    make(chan struct{}),
	}
}
