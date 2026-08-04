package database_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func TestDeleteWorkspaceAgentChildRollsBackOnContextCleanupError(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	childID := uuid.New()
	parentID := uuid.New()
	cleanupErr := xerrors.New("cleanup failed")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("-- name: SoftDeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwned :one")).
		WithArgs(childID, parentID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta("-- name: DeleteWorkspaceAgentContextResourcesByAgentID :exec")).
		WithArgs(childID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("-- name: DeleteWorkspaceAgentContextSnapshotByAgentID :exec")).
		WithArgs(childID).
		WillReturnError(cleanupErr)
	mock.ExpectRollback()

	count, err := database.New(sqlDB).DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwned(context.Background(), database.DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwnedParams{
		ID:       childID,
		ParentID: parentID,
	})
	require.Zero(t, count)
	require.ErrorIs(t, err, cleanupErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
