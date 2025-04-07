// Package agentssh_test provides tests for basic functinoality of the agentssh
// package, more test coverage can be found in the `agent` and `cli` package(s).
package agentssh_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestNewServer_ServeClient(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := testutil.Logger(t)
	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	defer s.Close()
	err = s.UpdateHostSigner(42)
	assert.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := s.Serve(ln)
		assert.Error(t, err) // Server is closed.
	}()

	c := sshClient(t, ln.Addr().String())

	var b bytes.Buffer
	sess, err := c.NewSession()
	require.NoError(t, err)
	sess.Stdout = &b
	err = sess.Start("echo hello")
	require.NoError(t, err)

	err = sess.Wait()
	require.NoError(t, err)

	require.Equal(t, "hello", strings.TrimSpace(b.String()))

	err = s.Close()
	require.NoError(t, err)
	<-done
}

func TestNewServer_ExecuteShebang(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("bash doesn't exist on Windows")
	}

	ctx := context.Background()
	logger := testutil.Logger(t)
	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		cmd, err := s.CreateCommand(ctx, `#!/bin/bash
		echo test`, nil, nil)
		require.NoError(t, err)
		output, err := cmd.AsExec().CombinedOutput()
		require.NoError(t, err)
		require.Equal(t, "test\n", string(output))
	})
	t.Run("Args", func(t *testing.T) {
		t.Parallel()
		cmd, err := s.CreateCommand(ctx, `#!/usr/bin/env bash
		echo test`, nil, nil)
		require.NoError(t, err)
		output, err := cmd.AsExec().CombinedOutput()
		require.NoError(t, err)
		require.Equal(t, "test\n", string(output))
	})
	t.Run("CustomEnvInfoer", func(t *testing.T) {
		t.Parallel()
		ei := &fakeEnvInfoer{
			CurrentUserFn: func() (u *user.User, err error) {
				return nil, assert.AnError
			},
		}
		_, err := s.CreateCommand(ctx, `whatever`, nil, ei)
		require.ErrorIs(t, err, assert.AnError)
	})
}

type fakeEnvInfoer struct {
	CurrentUserFn func() (*user.User, error)
	EnvironFn     func() []string
	UserHomeDirFn func() (string, error)
	UserShellFn   func(string) (string, error)
}

func (f *fakeEnvInfoer) User() (u *user.User, err error) {
	return f.CurrentUserFn()
}

func (f *fakeEnvInfoer) Environ() []string {
	return f.EnvironFn()
}

func (f *fakeEnvInfoer) HomeDir() (string, error) {
	return f.UserHomeDirFn()
}

func (f *fakeEnvInfoer) Shell(u string) (string, error) {
	return f.UserShellFn(u)
}

func (*fakeEnvInfoer) ModifyCommand(cmd string, args ...string) (string, []string) {
	return cmd, args
}

func TestNewServer_CloseActiveConnections(t *testing.T) {
	t.Parallel()

	prepare := func(ctx context.Context, t *testing.T) (*agentssh.Server, func()) {
		t.Helper()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = s.Close()
		})
		err = s.UpdateHostSigner(42)
		assert.NoError(t, err)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		waitConns := make([]chan struct{}, 4)

		var wg sync.WaitGroup
		wg.Add(1 + len(waitConns))

		go func() {
			defer wg.Done()
			err := s.Serve(ln)
			assert.Error(t, err) // Server is closed.
		}()

		for i := 0; i < len(waitConns); i++ {
			waitConns[i] = make(chan struct{})
			go func(ch chan struct{}) {
				defer wg.Done()
				c := sshClient(t, ln.Addr().String())
				sess, err := c.NewSession()
				assert.NoError(t, err)
				pty := ptytest.New(t)
				sess.Stdin = pty.Input()
				sess.Stdout = pty.Output()
				sess.Stderr = pty.Output()

				// Every other session will request a PTY.
				if i%2 == 0 {
					err = sess.RequestPty("xterm", 80, 80, nil)
					assert.NoError(t, err)
				}
				// The 60 seconds here is intended to be longer than the
				// test. The shutdown should propagate.
				if runtime.GOOS == "windows" {
					// Best effort to at least partially test this in Windows.
					err = sess.Start("echo start\"ed\" && sleep 60")
				} else {
					err = sess.Start("/bin/bash -c 'trap \"sleep 60\" SIGTERM; echo start\"ed\"; sleep 60'")
				}
				assert.NoError(t, err)

				pty.ExpectMatchContext(ctx, "started")
				close(ch)

				err = sess.Wait()
				assert.Error(t, err)
			}(waitConns[i])
		}

		for _, ch := range waitConns {
			<-ch
		}

		return s, wg.Wait
	}

	t.Run("Close", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		s, wait := prepare(ctx, t)
		err := s.Close()
		require.NoError(t, err)
		wait()
	})

	t.Run("Shutdown", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		s, wait := prepare(ctx, t)
		err := s.Shutdown(ctx)
		require.NoError(t, err)
		wait()
	})

	t.Run("Shutdown Early", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		s, wait := prepare(ctx, t)
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		err := s.Shutdown(ctx)
		require.ErrorIs(t, err, context.Canceled)
		wait()
	})
}

func TestNewServer_Signal(t *testing.T) {
	t.Parallel()

	t.Run("Stdout", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		logger := testutil.Logger(t)
		s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
		require.NoError(t, err)
		defer s.Close()
		err = s.UpdateHostSigner(42)
		assert.NoError(t, err)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			err := s.Serve(ln)
			assert.Error(t, err) // Server is closed.
		}()
		defer func() {
			err := s.Close()
			require.NoError(t, err)
			<-done
		}()

		c := sshClient(t, ln.Addr().String())

		sess, err := c.NewSession()
		require.NoError(t, err)
		r, err := sess.StdoutPipe()
		require.NoError(t, err)

		// Perform multiple sleeps since the interrupt signal doesn't propagate to
		// the process group, this lets us exit early.
		sleeps := strings.Repeat("sleep 1 && ", int(testutil.WaitMedium.Seconds()))
		err = sess.Start(fmt.Sprintf("echo hello && %s echo bye", sleeps))
		require.NoError(t, err)

		sc := bufio.NewScanner(r)
		for sc.Scan() {
			t.Log(sc.Text())
			if strings.Contains(sc.Text(), "hello") {
				break
			}
		}
		require.NoError(t, sc.Err())

		err = sess.Signal(ssh.SIGKILL)
		require.NoError(t, err)

		// Assumption, signal propagates and the command exists, closing stdout.
		for sc.Scan() {
			t.Log(sc.Text())
			require.NotContains(t, sc.Text(), "bye")
		}
		require.NoError(t, sc.Err())

		err = sess.Wait()
		exitErr := &ssh.ExitError{}
		require.ErrorAs(t, err, &exitErr)
		wantCode := 255
		if runtime.GOOS == "windows" {
			wantCode = 1
		}
		require.Equal(t, wantCode, exitErr.ExitStatus())
	})
	t.Run("PTY", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		logger := testutil.Logger(t)
		s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
		require.NoError(t, err)
		defer s.Close()
		err = s.UpdateHostSigner(42)
		assert.NoError(t, err)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			err := s.Serve(ln)
			assert.Error(t, err) // Server is closed.
		}()
		defer func() {
			err := s.Close()
			require.NoError(t, err)
			<-done
		}()

		c := sshClient(t, ln.Addr().String())

		pty := ptytest.New(t)

		sess, err := c.NewSession()
		require.NoError(t, err)
		r, err := sess.StdoutPipe()
		require.NoError(t, err)

		// Note, we request pty but don't use ptytest here because we can't
		// easily test for no text before EOF.
		sess.Stdin = pty.Input()
		sess.Stderr = pty.Output()

		err = sess.RequestPty("xterm", 80, 80, nil)
		require.NoError(t, err)

		// Perform multiple sleeps since the interrupt signal doesn't propagate to
		// the process group, this lets us exit early.
		sleeps := strings.Repeat("sleep 1 && ", int(testutil.WaitMedium.Seconds()))
		err = sess.Start(fmt.Sprintf("echo hello && %s echo bye", sleeps))
		require.NoError(t, err)

		sc := bufio.NewScanner(r)
		for sc.Scan() {
			t.Log(sc.Text())
			if strings.Contains(sc.Text(), "hello") {
				break
			}
		}
		require.NoError(t, sc.Err())

		err = sess.Signal(ssh.SIGKILL)
		require.NoError(t, err)

		// Assumption, signal propagates and the command exists, closing stdout.
		for sc.Scan() {
			t.Log(sc.Text())
			require.NotContains(t, sc.Text(), "bye")
		}
		require.NoError(t, sc.Err())

		err = sess.Wait()
		exitErr := &ssh.ExitError{}
		require.ErrorAs(t, err, &exitErr)
		wantCode := 255
		if runtime.GOOS == "windows" {
			wantCode = 1
		}
		require.Equal(t, wantCode, exitErr.ExitStatus())
	})
}

func sshClient(t *testing.T, addr string) *ssh.Client {
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	sshConn, channels, requests, err := ssh.NewClientConn(conn, "localhost:22", &ssh.ClientConfig{
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // This is a test.
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshConn.Close()
	})
	c := ssh.NewClient(sshConn, channels, requests)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}
