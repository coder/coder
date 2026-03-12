package agentssh

import (
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// fakeSession implements ssh.Session for testing sessionCloseTracker.
// Only Exit and Close are exercised; other methods panic if called.
type fakeSession struct {
	gliderssh.Session
	exitCalled  bool
	exitCode    int
	closeCalled bool
}

func (f *fakeSession) Exit(code int) error {
	f.exitCalled = true
	f.exitCode = code
	return nil
}

func (f *fakeSession) Close() error {
	f.closeCalled = true
	return nil
}

func TestSessionCloseTracker_ExitBeforeClose(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	err := scr.Exit(42)
	require.NoError(t, err)
	err = scr.Close()
	require.NoError(t, err)

	// Exit's code takes priority over Close's default of 1.
	assert.Equal(t, 42, scr.exitCode())
	// closeReason should be empty because Exit was called.
	assert.Empty(t, scr.closeReason())
}

func TestSessionCloseTracker_CloseBeforeExit(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	// Close fires first (e.g. sftp.Server.Close() teardown).
	err := scr.Close()
	require.NoError(t, err)
	// Exit fires second with the real exit code.
	err = scr.Exit(0)
	require.NoError(t, err)

	// Exit always wins, even when Close ran first.
	assert.Equal(t, 0, scr.exitCode())
	// closeReason should be empty because Exit was called.
	assert.Empty(t, scr.closeReason())
}

func TestSessionCloseTracker_CloseOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	err := scr.Close()
	require.NoError(t, err)

	// Without Exit, Close sets code 1 and a default reason.
	assert.Equal(t, 1, scr.exitCode())
	assert.NotEmpty(t, scr.closeReason(), "should have a default reason when only Close is called")
}

func TestSessionCloseTracker_ExitOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	err := scr.Exit(7)
	require.NoError(t, err)

	assert.Equal(t, 7, scr.exitCode())
	assert.Empty(t, scr.closeReason())
}

func TestSessionCloseTracker_MultipleExitCalls(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	// Second Exit call should override the first.
	err := scr.Exit(1)
	require.NoError(t, err)
	err = scr.Exit(2)
	require.NoError(t, err)

	assert.Equal(t, 2, scr.exitCode())
}

func TestSessionCloseTracker_ZeroValue(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	// Before any calls, defaults to zero.
	assert.Equal(t, 0, scr.exitCode())
	assert.Empty(t, scr.closeReason())
}

func TestSessionCloseTracker_DelegatesUnderlying(t *testing.T) {
	t.Parallel()
	fake := &fakeSession{}
	scr := &sessionCloseTracker{Session: fake}

	err := scr.Exit(5)
	require.NoError(t, err)
	assert.True(t, fake.exitCalled, "should delegate Exit to underlying session")
	assert.Equal(t, 5, fake.exitCode)

	err = scr.Close()
	require.NoError(t, err)
	assert.True(t, fake.closeCalled, "should delegate Close to underlying session")
}

func TestSessionCloseTracker_ExitError(t *testing.T) {
	t.Parallel()

	// Simulate an underlying session that returns errors.
	failSession := &failingSession{}
	scr := &sessionCloseTracker{Session: failSession}

	err := scr.Exit(1)
	assert.Error(t, err)
	// The code should still be tracked even if the underlying Exit fails.
	assert.Equal(t, 1, scr.exitCode())
}

type failingSession struct {
	gliderssh.Session
}

func (*failingSession) Exit(_ int) error {
	return xerrors.New("exit failed")
}

func (*failingSession) Close() error {
	return xerrors.New("close failed")
}
