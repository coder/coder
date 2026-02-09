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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
	defer ac.unblock()
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

	ac.unblock()
	testutil.TryReceive(testCtx, t, closed)
	testutil.TryReceive(testCtx, t, ac.done)
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
			a.unblock()
			testutil.TryReceive(ctx, t, a.done) // ensure we don't race with context cancellation
		}
	}()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		uut.close(nil)
	}()
	trap.MustWait(ctx).MustRelease(ctx)
	// top starts right away, but it hangs
	testutil.TryReceive(ctx, t, ac[2].started)
	// timer pops and we start the middle one
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	testutil.TryReceive(ctx, t, ac[1].started)

	// middle one finishes
	ac[1].unblock()
	// bottom starts, but also hangs
	testutil.TryReceive(ctx, t, ac[0].started)

	// timer has to pop twice to time out.
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	mClock.Advance(gracefulShutdownTimeout).MustWait(ctx)
	testutil.TryReceive(ctx, t, closed)
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

	conn, channels, requests, err := ssh.NewClientConn(&testutil.ReaderWriterConn{
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

	// We're not connected to a real shell
	err = session.Run("")
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
	t           *testing.T
	ctx         context.Context
	started     chan struct{}
	done        chan struct{}
	isUnblocked chan struct{}
	unblockOnce sync.Once
}

func (c *asyncCloser) Close() error {
	close(c.started)
	defer close(c.done)
	select {
	case <-c.ctx.Done():
		c.t.Error("timed out")
		return c.ctx.Err()
	case <-c.isUnblocked:
		return nil
	}
}

func (c *asyncCloser) unblock() {
	c.unblockOnce.Do(func() { close(c.isUnblocked) })
}

func newAsyncCloser(ctx context.Context, t *testing.T) *asyncCloser {
	return &asyncCloser{
		t:           t,
		ctx:         ctx,
		isUnblocked: make(chan struct{}),
		started:     make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func Test_getWorkspaceAgent(t *testing.T) {
	t.Parallel()

	createWorkspaceWithAgents := func(agents []codersdk.WorkspaceAgent) codersdk.Workspace {
		return codersdk.Workspace{
			Name: "test-workspace",
			LatestBuild: codersdk.WorkspaceBuild{
				Resources: []codersdk.WorkspaceResource{
					{
						Agents: agents,
					},
				},
			},
		}
	}

	createAgent := func(name string) codersdk.WorkspaceAgent {
		return codersdk.WorkspaceAgent{
			ID:   uuid.New(),
			Name: name,
		}
	}

	t.Run("SingleAgent_NoNameSpecified", func(t *testing.T) {
		t.Parallel()
		agent := createAgent("main")
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{agent})

		result, _, err := getWorkspaceAgent(workspace, "")
		require.NoError(t, err)
		assert.Equal(t, agent.ID, result.ID)
		assert.Equal(t, "main", result.Name)
	})

	t.Run("MultipleAgents_NoNameSpecified", func(t *testing.T) {
		t.Parallel()
		agent1 := createAgent("main1")
		agent2 := createAgent("main2")
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{agent1, agent2})

		_, _, err := getWorkspaceAgent(workspace, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multiple agents found")
		assert.Contains(t, err.Error(), "available agents: [main1 main2]")
	})

	t.Run("AgentNameSpecified_Found", func(t *testing.T) {
		t.Parallel()
		agent1 := createAgent("main1")
		agent2 := createAgent("main2")
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{agent1, agent2})

		result, other, err := getWorkspaceAgent(workspace, "main1")
		require.NoError(t, err)
		assert.Equal(t, agent1.ID, result.ID)
		assert.Equal(t, "main1", result.Name)
		assert.Len(t, other, 1)
		assert.Equal(t, agent2.ID, other[0].ID)
		assert.Equal(t, "main2", other[0].Name)
	})

	t.Run("AgentNameSpecified_NotFound", func(t *testing.T) {
		t.Parallel()
		agent1 := createAgent("main1")
		agent2 := createAgent("main2")
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{agent1, agent2})

		_, _, err := getWorkspaceAgent(workspace, "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `agent not found by name "nonexistent"`)
		assert.Contains(t, err.Error(), "available agents: [main1 main2]")
	})

	t.Run("NoAgents", func(t *testing.T) {
		t.Parallel()
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{})

		_, _, err := getWorkspaceAgent(workspace, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `workspace "test-workspace" has no agents`)
	})

	t.Run("AvailableAgentNames_SortedCorrectly", func(t *testing.T) {
		t.Parallel()
		// Define agents in non-alphabetical order.
		agent2 := createAgent("zod")
		agent1 := createAgent("clark")
		agent3 := createAgent("krypton")
		workspace := createWorkspaceWithAgents([]codersdk.WorkspaceAgent{agent2, agent1, agent3})

		_, _, err := getWorkspaceAgent(workspace, "nonexistent")
		require.Error(t, err)
		// Available agents should be sorted alphabetically.
		assert.Contains(t, err.Error(), "available agents: [clark krypton zod]")
	})
}
