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
		InTx(gomock.Any(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead}).
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
		InTx(gomock.Any(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(1).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	mDB.EXPECT().
		InTx(gomock.Any(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead}).
		After(firstUpdate).
		Times(1).
		Return(nil)

	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.NoError(t, err)
}

func TestReadModifyUpdate_HardError(t *testing.T) {
	t.Parallel()

	mDB := dbmock.NewMockStore(gomock.NewController(t))

	mDB.EXPECT().
		InTx(gomock.Any(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead}).
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
		InTx(gomock.Any(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead}).
		Times(5).
		Return(&pq.Error{Code: pq.ErrorCode("40001")})
	err := database.ReadModifyUpdate(mDB, func(tx database.Store) error {
		return nil
	})
	require.ErrorContains(t, err, "too many errors")
}
