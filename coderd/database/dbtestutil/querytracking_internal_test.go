package dbtestutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestWrapWithQueryTracking(t *testing.T) {
	t.Parallel()

	t.Run("TracksQueries", func(t *testing.T) {
		t.Parallel()

		// Given: a db wrapped with query tracking.
		resetCh := make(chan struct{})
		mockDB := dbmock.NewMockStore(gomock.NewController(t))
		mockDB.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockDB.EXPECT().Wrappers().Return(nil).AnyTimes()
		logger := slogtest.Make(t, nil)
		db, cleanup := wrapWithQueryTracking(t, mockDB, logger, resetCh)

		// When: some database queries are executed.
		_, _ = db.GetUsers(t.Context(), database.GetUsersParams{})
		_, _ = db.GetUsers(t.Context(), database.GetUsersParams{})

		// Then: cleanup writes a report file.
		cleanup()

		reportPath := reportPathForTest(t)
		t.Cleanup(func() { _ = os.Remove(reportPath) })

		content, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "GetUsers")
		require.Contains(t, string(content), "2\tGetUsers")
	})

	t.Run("ResetExcludesSetupQueries", func(t *testing.T) {
		t.Parallel()

		// Given: a db wrapped with query tracking.
		resetCh := make(chan struct{})
		mockDB := dbmock.NewMockStore(gomock.NewController(t))
		mockDB.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockDB.EXPECT().GetAuthorizedUsers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockDB.EXPECT().Wrappers().Return(nil).AnyTimes()
		logger := slogtest.Make(t, nil)
		db, cleanup := wrapWithQueryTracking(t, mockDB, logger, resetCh)

		// When: setup queries run, then reset, then test queries run.
		_, _ = db.GetUsers(t.Context(), database.GetUsersParams{})
		_, _ = db.GetUsers(t.Context(), database.GetUsersParams{})
		resetCh <- struct{}{}

		// Only this query should be in the report.
		_, _ = db.GetAuthorizedUsers(t.Context(), database.GetUsersParams{}, nil)

		// Then: report only contains post-reset queries.
		cleanup()

		reportPath := reportPathForTest(t)
		t.Cleanup(func() { _ = os.Remove(reportPath) })

		content, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "1\tGetAuthorizedUsers")
		require.NotContains(t, string(content), "GetUsers")
	})

	t.Run("NilChannelTracksAll", func(t *testing.T) {
		t.Parallel()

		// Given: a db wrapped with query tracking and nil channel.
		mockDB := dbmock.NewMockStore(gomock.NewController(t))
		mockDB.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockDB.EXPECT().Wrappers().Return(nil).AnyTimes()
		logger := slogtest.Make(t, nil)
		db, cleanup := wrapWithQueryTracking(t, mockDB, logger, nil)

		// When: some database queries are executed.
		_, _ = db.GetUsers(t.Context(), database.GetUsersParams{})

		// Then: all queries are in the report.
		cleanup()

		reportPath := reportPathForTest(t)
		t.Cleanup(func() { _ = os.Remove(reportPath) })

		content, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		require.Contains(t, string(content), "GetUsers")
	})
}

func reportPathForTest(t testing.TB) string {
	t.Helper()
	outDir := os.Getenv("DBTRACKER_REPORT_DIR")
	if outDir == "" {
		var err error
		outDir, err = filepath.Abs(".")
		require.NoError(t, err)
	}
	testName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, t.Name())
	return filepath.Join(outDir, testName+".querytracking.tsv")
}
