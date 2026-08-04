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

// expectReportThroughLatestBuild queues the statements that run before the
// generation is validated: the immutable identity read, the publication lock,
// the fenced status lock, and the fresh latest-build read.
func expectReportThroughLatestBuild(mock sqlmock.Sqlmock, fixture acquireFixture, acquisitionVersion int64, latestBuildID uuid.UUID, transition string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetWorkspaceAgentSubagentExecutionForAcquisition :one")).
		WithArgs(fixture.buildID, fixture.declID, fixture.parentID).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "child_agent_id"}).
			AddRow(fixture.workspaceID, fixture.childID))
	mock.ExpectExec(regexp.QuoteMeta("-- name: AcquireWorkspaceBuildPublicationLock :exec")).
		WithArgs(fixture.workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: LockWorkspaceAgentSubagentExecutionStatusForReport :one")).
		WithArgs(fixture.buildID, fixture.declID, acquisitionVersion).
		WillReturnRows(sqlmock.NewRows([]string{"status", "status_changed_at", "acquisition_version"}).
			AddRow("running", fixture.now, acquisitionVersion))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetLatestWorkspaceBuildGeneration :one")).
		WithArgs(fixture.workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transition"}).
			AddRow(latestBuildID, transition))
}

// expectReportedStatusRow queues the mutation and the row it returns.
func expectReportedStatusRow(mock sqlmock.Sqlmock, fixture acquireFixture, acquisitionVersion int64, status string) {
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetWorkspaceAgentSubagentExecutionAcquisitionParent :one")).
		WithArgs(fixture.parentID, fixture.buildID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "resource_id"}).
			AddRow(fixture.parentID, fixture.resourceID))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: LockWorkspaceAgentSubagentExecutionChildForReport :one")).
		WithArgs(fixture.childID, uuid.NullUUID{UUID: fixture.parentID, Valid: true}, fixture.resourceID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(fixture.childID))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: MarkWorkspaceAgentSubagentExecutionReported :one")).
		WithArgs(status, fixture.now, "driver log tail", fixture.buildID, fixture.declID, acquisitionVersion).
		WillReturnRows(sqlmock.NewRows([]string{
			"workspace_build_id",
			"declaration_id",
			"status",
			"created_at",
			"updated_at",
			"status_changed_at",
			"last_acquired_at",
			"last_reported_at",
			"restart_count",
			"last_error",
			"acquisition_version",
		}).AddRow(
			fixture.buildID,
			fixture.declID,
			status,
			fixture.now,
			fixture.now,
			fixture.now,
			fixture.now,
			fixture.now,
			int32(0),
			"driver log tail",
			acquisitionVersion,
		))
	mock.ExpectCommit()
}

func reportParams(fixture acquireFixture, acquisitionVersion int64, status database.SubagentExecutionStatus) database.ReportWorkspaceAgentSubagentExecutionStatusParams {
	return database.ReportWorkspaceAgentSubagentExecutionStatusParams{
		WorkspaceBuildID:   fixture.buildID,
		DeclarationID:      fixture.declID,
		ParentAgentID:      fixture.parentID,
		AcquisitionVersion: acquisitionVersion,
		Status:             status,
		LastError:          "driver log tail",
		Now:                fixture.now,
	}
}

// TestReportWorkspaceAgentSubagentExecutionStatusStatementOrder pins the
// statement sequence of the report transaction. It is deliberately the
// acquisition's order, publication guard, then execution status, then the exact
// child row, so the two paths cannot deadlock against each other. Taking the
// guard and the status lock before the latest build, parent, and child are read
// is also what makes those reads observe fresh READ COMMITTED snapshots.
func TestReportWorkspaceAgentSubagentExecutionStatusStatementOrder(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()
	const version = int64(3)

	expectReportThroughLatestBuild(mock, fixture, version, fixture.buildID, "start")
	expectReportedStatusRow(mock, fixture, version, "stopping")

	reported, err := database.New(sqlDB).ReportWorkspaceAgentSubagentExecutionStatus(context.Background(),
		reportParams(fixture, version, database.SubagentExecutionStatusStopping))
	require.NoError(t, err)
	require.Equal(t, fixture.buildID, reported.WorkspaceBuildID)
	require.Equal(t, fixture.declID, reported.DeclarationID)
	require.Equal(t, "stopping", reported.Status)
	require.EqualValues(t, version, reported.AcquisitionVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReportWorkspaceAgentSubagentExecutionStatusStoppedGenerationOrder asserts
// that the shutdown exception resolves the preceding start build in its own
// statement, after the latest build is known and before the parent and child are
// revalidated, so the lock order stays identical to the acquisition's.
func TestReportWorkspaceAgentSubagentExecutionStatusStoppedGenerationOrder(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()
	stopBuildID := uuid.New()
	const version = int64(3)

	expectReportThroughLatestBuild(mock, fixture, version, stopBuildID, "stop")
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetPrecedingStartWorkspaceBuildGeneration :one")).
		WithArgs(stopBuildID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(fixture.buildID))
	expectReportedStatusRow(mock, fixture, version, "stopped")

	reported, err := database.New(sqlDB).ReportWorkspaceAgentSubagentExecutionStatus(context.Background(),
		reportParams(fixture, version, database.SubagentExecutionStatusStopped))
	require.NoError(t, err)
	require.Equal(t, "stopped", reported.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReportWorkspaceAgentSubagentExecutionStatusStaleGenerationRollsBack
// asserts that a reported build which a newer start build has replaced aborts the
// transaction before the parent, child, or status row are touched, and is
// reported as sql.ErrNoRows.
func TestReportWorkspaceAgentSubagentExecutionStatusStaleGenerationRollsBack(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()
	const version = int64(3)

	expectReportThroughLatestBuild(mock, fixture, version, uuid.New(), "start")
	mock.ExpectRollback()

	reported, err := database.New(sqlDB).ReportWorkspaceAgentSubagentExecutionStatus(context.Background(),
		reportParams(fixture, version, database.SubagentExecutionStatusRunning))
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Equal(t, database.ReportWorkspaceAgentSubagentExecutionStatusRow{}, reported)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestReportWorkspaceAgentSubagentExecutionStatusRefusedTransitionRollsBack
// asserts that a status the stored status does not permit aborts the transaction
// as soon as the locked status is known, without reading the generation, the
// parent, or the child.
func TestReportWorkspaceAgentSubagentExecutionStatusRefusedTransitionRollsBack(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fixture := newAcquireFixture()
	const version = int64(3)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("-- name: GetWorkspaceAgentSubagentExecutionForAcquisition :one")).
		WithArgs(fixture.buildID, fixture.declID, fixture.parentID).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "child_agent_id"}).
			AddRow(fixture.workspaceID, fixture.childID))
	mock.ExpectExec(regexp.QuoteMeta("-- name: AcquireWorkspaceBuildPublicationLock :exec")).
		WithArgs(fixture.workspaceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("-- name: LockWorkspaceAgentSubagentExecutionStatusForReport :one")).
		WithArgs(fixture.buildID, fixture.declID, version).
		WillReturnRows(sqlmock.NewRows([]string{"status", "status_changed_at", "acquisition_version"}).
			AddRow("stopped", fixture.now, version))
	mock.ExpectRollback()

	// 'stopped' is terminal for a launcher, so it cannot go back to running.
	reported, err := database.New(sqlDB).ReportWorkspaceAgentSubagentExecutionStatus(context.Background(),
		reportParams(fixture, version, database.SubagentExecutionStatusRunning))
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Equal(t, database.ReportWorkspaceAgentSubagentExecutionStatusRow{}, reported)
	require.NoError(t, mock.ExpectationsWereMet())
}
