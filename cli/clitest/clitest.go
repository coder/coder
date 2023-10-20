package clitest

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// New creates a CLI instance with a configuration pointed to a
// temporary testing directory.
func New(t testing.TB, args ...string) (*clibase.Invocation, config.Root) {
	var root cli.RootCmd

	cmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	return NewWithCommand(t, cmd, args...)
}

type logWriter struct {
	prefix string
	log    slog.Logger
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	trimmed := strings.TrimSpace(string(p))
	if trimmed == "" {
		return len(p), nil
	}
	l.log.Info(
		context.Background(),
		l.prefix+": "+trimmed,
	)
	return len(p), nil
}

func NewWithCommand(
	t testing.TB, cmd *clibase.Cmd, args ...string,
) (*clibase.Invocation, config.Root) {
	configDir := config.Root(t.TempDir())
	logger := slogtest.Make(t, nil)
	i := &clibase.Invocation{
		Command: cmd,
		Args:    append([]string{"--global-config", string(configDir)}, args...),
		Stdin:   io.LimitReader(nil, 0),
		Stdout:  (&logWriter{prefix: "stdout", log: logger}),
		Stderr:  (&logWriter{prefix: "stderr", log: logger}),
	}
	t.Logf("invoking command: %s %s", cmd.Name(), strings.Join(i.Args, " "))

	// These can be overridden by the test.
	return i, configDir
}

// SetupConfig applies the URL and SessionToken of the client to the config.
func SetupConfig(t *testing.T, client *codersdk.Client, root config.Root) {
	err := root.Session().Write(client.SessionToken())
	require.NoError(t, err)
	err = root.URL().Write(client.URL.String())
	require.NoError(t, err)
}

// CreateTemplateVersionSource writes the echo provisioner responses into a
// new temporary testing directory.
func CreateTemplateVersionSource(t *testing.T, responses *echo.Responses) string {
	directory := t.TempDir()
	f, err := os.CreateTemp(directory, "*.tf")
	require.NoError(t, err)
	_ = f.Close()
	f, err = os.Create(filepath.Join(directory, ".terraform.lock.hcl"))
	require.NoError(t, err)
	_ = f.Close()
	data, err := echo.Tar(responses)
	require.NoError(t, err)
	extractTar(t, data, directory)
	return directory
}

func extractTar(t *testing.T, data []byte, directory string) {
	reader := tar.NewReader(bytes.NewBuffer(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if header.Name == "." || strings.Contains(header.Name, "..") {
			continue
		}
		// #nosec
		path := filepath.Join(directory, header.Name)
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0o600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, mode)
			require.NoError(t, err)
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			require.NoError(t, err)
			// Max file size of 10MB.
			_, err = io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			require.NoError(t, err)
			err = file.Close()
			require.NoError(t, err)
		}
	}
}

// Start runs the command in a goroutine and cleans it up when the test
// completed.
func Start(t *testing.T, inv *clibase.Invocation) {
	t.Helper()

	closeCh := make(chan struct{})
	// StartWithWaiter adds its own `t.Cleanup`, so we need to be sure it's added
	// before ours.
	waiter := StartWithWaiter(t, inv)
	t.Cleanup(func() {
		waiter.Cancel()
		<-closeCh
	})

	go func() {
		defer close(closeCh)
		err := waiter.Wait()
		switch {
		case errors.Is(err, context.Canceled):
			return
		default:
			assert.NoError(t, err)
		}
	}()
}

// Run runs the command and asserts that there is no error.
func Run(t *testing.T, inv *clibase.Invocation) {
	t.Helper()

	err := inv.Run()
	require.NoError(t, err)
}

type ErrorWaiter struct {
	waitOnce    sync.Once
	cachedError error
	cancelFunc  context.CancelFunc

	c <-chan error
	t *testing.T
}

func (w *ErrorWaiter) Cancel() {
	w.cancelFunc()
}

func (w *ErrorWaiter) Wait() error {
	w.waitOnce.Do(func() {
		var ok bool
		w.cachedError, ok = <-w.c
		if !ok {
			panic("unexpected channel close")
		}
	})
	return w.cachedError
}

func (w *ErrorWaiter) RequireSuccess() {
	require.NoError(w.t, w.Wait())
}

func (w *ErrorWaiter) RequireError() {
	require.Error(w.t, w.Wait())
}

func (w *ErrorWaiter) RequireContains(s string) {
	require.ErrorContains(w.t, w.Wait(), s)
}

func (w *ErrorWaiter) RequireIs(want error) {
	require.ErrorIs(w.t, w.Wait(), want)
}

func (w *ErrorWaiter) RequireAs(want interface{}) {
	require.ErrorAs(w.t, w.Wait(), want)
}

// StartWithWaiter runs the command in a goroutine but returns the error instead
// of asserting it. This is useful for testing error cases.
func StartWithWaiter(t *testing.T, inv *clibase.Invocation) *ErrorWaiter {
	t.Helper()

	var (
		ctx    = inv.Context()
		cancel func()

		cleaningUp atomic.Bool
		errCh      = make(chan error, 1)
		doneCh     = make(chan struct{})
	)
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(testutil.WaitMedium))
	} else {
		ctx, cancel = context.WithCancel(inv.Context())
	}

	inv = inv.WithContext(ctx)

	go func() {
		defer close(doneCh)
		defer close(errCh)
		err := inv.Run()
		if cleaningUp.Load() && errors.Is(err, context.DeadlineExceeded) {
			// If we're cleaning up, this error is likely related to the CLI
			// teardown process. E.g., the server could be slow to shut down
			// Postgres.
			t.Logf("command %q timed out during test cleanup", inv.Command.FullName())
		}
		// Whether or not this fails the test is left to the caller.
		t.Logf("command %q exited with error: %v", inv.Command.FullName(), err)
		errCh <- err
	}()

	// Don't exit test routine until server is done.
	t.Cleanup(func() {
		cancel()
		cleaningUp.Store(true)
		<-doneCh
	})
	return &ErrorWaiter{c: errCh, t: t, cancelFunc: cancel}
}
