package database_test

import (
	"database/sql"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestReadModifyUpdate_OK(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(nil)
	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

func TestReadModifyUpdate_RetryOK(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	firstUpdate := mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		After(firstUpdate).
		Times(1).
		Return(nil)

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

// TestReadModifyUpdate_RetryDeadlockOK asserts that a deadlock, which
// PostgreSQL resolves by aborting one of the conflicting transactions, is
// retried on the same path as a serialization failure.
func TestReadModifyUpdate_RetryDeadlockOK(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	firstUpdate := mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("40P01")})
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		After(firstUpdate).
		Times(1).
		Return(nil)

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

// TestReadModifyUpdate_OtherPQErrorNotRetried asserts that only the conflict
// codes are retried, so an unrelated database error is returned immediately.
func TestReadModifyUpdate_OtherPQErrorNotRetried(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	// 23505 is unique_violation.
	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("23505")})

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	var pqe *pq.Error
	require.True(t, xerrors.As(err, &pqe))
	require.EqualValues(t, "23505", pqe.Code)
}

func TestReadModifyUpdate_HardError(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(xerrors.New("a bad thing happened"))

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.ErrorContains(t, err, "a bad thing happened")
}

func TestReadModifyUpdate_TooManyRetries(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().
		InTx(gomock.Any(), &database.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(5).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.ErrorContains(t, err, "too many errors")
}
