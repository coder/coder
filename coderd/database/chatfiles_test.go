package database_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestDeleteOldChatFilesRechecksSelectedCandidates(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	before := time.Now().UTC()
	fileID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT cf.id FROM chat_files cf")).
		WithArgs(before, int32(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(fileID))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM chat_files cf")).
		WithArgs(sqlmock.AnyArg(), before).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	deleted, err := database.New(sqlDB).DeleteOldChatFiles(context.Background(), database.DeleteOldChatFilesParams{
		BeforeTime: before,
		LimitCount: 100,
	})
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}
