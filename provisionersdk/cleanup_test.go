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
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

const workDirectory = "/tmp/coder/provisioner-34/work"

func TestStaleSessions(t *testing.T) {
	t.Parallel()

	prepare := func() (afero.Fs, time.Time, slog.Logger) {
		fs := afero.NewMemMapFs()
		now := time.Date(2023, time.June, 3, 4, 5, 6, 0, time.UTC)
		logger := slogtest.Make(t, nil).
			Leveled(slog.LevelDebug).
			Named("cleanup-test")
		return fs, now, logger
	}

	t.Run("all sessions are stale", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs, now, logger := prepare()

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

		fs, now, logger := prepare()

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

		fs, now, logger := prepare()

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

func addSessionFolder(t *testing.T, fs afero.Fs, sessionName string, accessTime time.Time) {
	err := fs.MkdirAll(filepath.Join(workDirectory, sessionName), 0o755)
	require.NoError(t, err, "can't create session folder")
	fs.Chtimes(filepath.Join(workDirectory, sessionName), accessTime, accessTime)
	require.NoError(t, err, "can't set times")
}
