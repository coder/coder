package provisionersdk_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

const workDirectory = "/tmp/coder/provisioner-34/work"

var now = time.Date(2023, time.June, 3, 4, 5, 6, 0, time.UTC)

func TestStaleSessions(t *testing.T) {
	t.Parallel()

	prepare := func() (afero.Fs, slog.Logger) {
		tempDir := t.TempDir()
		fs := afero.NewBasePathFs(afero.NewOsFs(), tempDir)
		logger := testutil.Logger(t).
			Leveled(slog.LevelDebug).
			Named("cleanup-test")
		return fs, logger
	}

	t.Run("all sessions are stale", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		first := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, first, now.Add(-7*24*time.Hour))
		second := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, second, now.Add(-8*24*time.Hour))
		third := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, third, now.Add(-9*24*time.Hour))

		// when
		provisionersdk.CleanStaleSessions(ctx, workDirectory, fs, now, logger)

		// then
		entries, err := afero.ReadDir(fs, workDirectory)
		require.NoError(t, err)
		require.Empty(t, entries, "all session leftovers should be removed")
	})

	t.Run("one session is stale", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		first := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, first, now.Add(-7*24*time.Hour))
		second := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, second, now.Add(-6*24*time.Hour))

		// when
		provisionersdk.CleanStaleSessions(ctx, workDirectory, fs, now, logger)

		// then
		entries, err := afero.ReadDir(fs, workDirectory)
		require.NoError(t, err)
		require.Len(t, entries, 1, "one session should be present")
		require.Equal(t, second, entries[0].Name(), 1)
	})

	t.Run("no stale sessions", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, logger := prepare()

		// given
		first := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, first, now.Add(-6*24*time.Hour))
		second := provisionersdk.SessionDir(uuid.NewString())
		addSessionFolder(t, fs, second, now.Add(-5*24*time.Hour))

		// when
		provisionersdk.CleanStaleSessions(ctx, workDirectory, fs, now, logger)

		// then
		entries, err := afero.ReadDir(fs, workDirectory)
		require.NoError(t, err)
		require.Len(t, entries, 2, "both sessions should be present")
	})
}

func addSessionFolder(t *testing.T, fs afero.Fs, sessionName string, modTime time.Time) {
	err := fs.MkdirAll(filepath.Join(workDirectory, sessionName), 0o755)
	require.NoError(t, err, "can't create session folder")
	require.NoError(t, fs.Chtimes(filepath.Join(workDirectory, sessionName), now, modTime), "can't chtime of session dir")
	require.NoError(t, err, "can't set times")
}
