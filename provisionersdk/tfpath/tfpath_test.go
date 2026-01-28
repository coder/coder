package tfpath_test

import (
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

func TestCleanStaleSessions(t *testing.T) {
	t.Parallel()

	t.Run("NonFatalRemoveFailure", func(t *testing.T) {
		t.Parallel()
		const parentDir = "parent"
		// Verify RemoveAll failure is not fatal
		ctx := testutil.Context(t, testutil.WaitShort)

		called := false
		mem := afero.NewMemMapFs()
		staleSession := tfpath.Session(parentDir, "stale")
		err := mem.MkdirAll(staleSession.WorkDirectory(), 0777)
		require.NoError(t, err)

		failingFs := &removeFailure{
			Fs: mem,
			removeAll: func(path string) error {
				called = true
				return xerrors.New("constant failure")
			},
		}

		future := time.Now().Add(time.Hour * 24 * 120)
		l := tfpath.Session(parentDir, "sess1")
		err = l.CleanStaleSessions(ctx, slogtest.Make(t, &slogtest.Options{
			IgnoreErrors: true,
		}), failingFs, future)
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("FatalRemoveFailure", func(t *testing.T) {
		// If the stale directory is the same one we plan to use, that is
		// an issue.
		t.Parallel()
		const parentDir = "parent"
		// Verify RemoveAll failure is not fatal
		ctx := testutil.Context(t, testutil.WaitShort)

		called := false
		mem := afero.NewMemMapFs()
		staleSession := tfpath.Session(parentDir, "stale")
		err := mem.MkdirAll(staleSession.WorkDirectory(), 0777)
		require.NoError(t, err)

		failingFs := &removeFailure{
			Fs: mem,
			removeAll: func(path string) error {
				called = true
				return xerrors.New("constant failure")
			},
		}

		future := time.Now().Add(time.Hour * 24 * 120)
		err = staleSession.CleanStaleSessions(ctx, slogtest.Make(t, &slogtest.Options{
			IgnoreErrors: true,
		}), failingFs, future)
		require.ErrorContains(t, err, "constant failure")
		require.True(t, called)
	})

}

type removeFailure struct {
	afero.Fs
	removeAll func(path string) error
}

func (rf *removeFailure) RemoveAll(path string) error {
	if rf.removeAll != nil {
		return rf.removeAll(path)
	}
	return rf.Fs.RemoveAll(path)
}
