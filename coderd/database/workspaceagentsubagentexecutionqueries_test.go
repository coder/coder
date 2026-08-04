package database_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// acquireFixture holds the identifiers shared by the acquisition statements.
type acquireFixture struct {
	workspaceID uuid.UUID
	buildID     uuid.UUID
	declID      uuid.UUID
	parentID    uuid.UUID
	resourceID  uuid.UUID
	childID     uuid.UUID
	childToken  uuid.UUID
	now         time.Time
}

func newAcquireFixture() acquireFixture {
	return acquireFixture{
		workspaceID: uuid.New(),
		buildID:     uuid.New(),
		declID:      uuid.New(),
		parentID:    uuid.New(),
		resourceID:  uuid.New(),
		childID:     uuid.New(),
		childToken:  uuid.New(),
		now:         time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

// expectAcquireThroughLatestBuild queues the statements that run before the
// generation is validated: the immutable identity read, the publication lock,
// the status lock, and the fresh latest-build read.
func expectAcquireThroughLatestBuild(mock sqlmock.Sqlmock, fixture acquireFixture, latestBuildID uuid.UUID, transition string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetWorkspaceAgentSubagentExecutionForAcquisition :one")).
		WithArgs(fixture.buildID, fixture.declID, fixture.parentID).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "child_agent_id"}).
			AddRow(fixture.workspaceID, fixture.childID))
	mock.ExpectExec(regexp.QuoteMeta("-- name: AcquireWorkspaceBuildPublicationLock :exec")).
		WithArgs(fixture.workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: LockWorkspaceAgentSubagentExecutionStatusForAcquisition :one")).
		WithArgs(fixture.buildID, fixture.declID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "status_changed_at", "restart_count", "acquisition_version"}).
			AddRow("failed", fixture.now, int32(0), int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetLatestWorkspaceBuildGeneration :one")).
		WithArgs(fixture.workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transition"}).
			AddRow(latestBuildID, transition))
}

// TestAcquireWorkspaceAgentSubagentExecutionStatementOrder pins the statement
// sequence of the acquisition transaction. The order is the correctness
// contract: the publication lock and the status lock are taken before the
// latest build, parent, and child are read, so those reads observe fresh READ
// COMMITTED snapshots that cannot predate a concurrently published build.
func TestAcquireWorkspaceAgentSubagentExecutionStatementOrder(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()

	expectAcquireThroughLatestBuild(mock, fixture, fixture.buildID, "start")
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetWorkspaceAgentSubagentExecutionAcquisitionParent :one")).
		WithArgs(fixture.parentID, fixture.buildID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "resource_id"}).
			AddRow(fixture.parentID, fixture.resourceID))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: LockWorkspaceAgentSubagentExecutionChildForAcquisition :one")).
		WithArgs(fixture.childID, uuid.NullUUID{UUID: fixture.parentID, Valid: true}, fixture.resourceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "auth_token"}).
			AddRow(fixture.childID, fixture.childToken))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: MarkWorkspaceAgentSubagentExecutionAcquired :one")).
		WithArgs(fixture.now, fixture.buildID, fixture.declID, fixture.childID).
		WillReturnRows(sqlmock.NewRows([]string{"child_agent_id", "auth_token", "acquisition_version"}).
			AddRow(fixture.childID, fixture.childToken, int64(2)))
	mock.ExpectCommit()

	acquired, err := database.New(sqlDB).AcquireWorkspaceAgentSubagentExecution(context.Background(), database.AcquireWorkspaceAgentSubagentExecutionParams{
		WorkspaceBuildID: fixture.buildID,
		DeclarationID:    fixture.declID,
		ParentAgentID:    fixture.parentID,
		Now:              fixture.now,
	})
	require.NoError(t, err)
	require.Equal(t, fixture.childID, acquired.ChildAgentID)
	require.Equal(t, fixture.childToken, acquired.AuthToken)
	require.EqualValues(t, 2, acquired.AcquisitionVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAcquireWorkspaceAgentSubagentExecutionStaleGenerationRollsBack asserts
// that a requested build which is no longer the workspace's latest generation
// aborts the transaction before the parent, child, or status row are touched,
// and is reported as sql.ErrNoRows.
func TestAcquireWorkspaceAgentSubagentExecutionStaleGenerationRollsBack(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()

	expectAcquireThroughLatestBuild(mock, fixture, uuid.New(), "start")
	mock.ExpectRollback()

	acquired, err := database.New(sqlDB).AcquireWorkspaceAgentSubagentExecution(context.Background(), database.AcquireWorkspaceAgentSubagentExecutionParams{
		WorkspaceBuildID: fixture.buildID,
		DeclarationID:    fixture.declID,
		ParentAgentID:    fixture.parentID,
		Now:              fixture.now,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Equal(t, database.AcquireWorkspaceAgentSubagentExecutionRow{}, acquired)
	require.NoError(t, mock.ExpectationsWereMet())
}
