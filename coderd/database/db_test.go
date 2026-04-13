package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	slog "cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
)

func TestSerializedRetry(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	called := 0
	txOpts := &database.TxOptions{Isolation: sql.LevelSerializable}
	err := db.InTx(func(tx database.Store) error {
		// Test nested error
		return tx.InTx(func(tx database.Store) error {
			// The easiest way to mock a serialization failure is to
			// return a serialization failure error.
			called++
			return &pq.Error{
				Code:    "40001",
				Message: "serialization_failure",
			}
		}, txOpts)
	}, txOpts)
	require.Error(t, err, "should fail")
	// The double "execute transaction: execute transaction" is from the nested transactions.
	// Just want to make sure we don't try 9 times.
	require.Equal(t, err.Error(), "transaction failed after 3 attempts: execute transaction: execute transaction: pq: serialization_failure", "error message")
	require.Equal(t, called, 3, "should retry 3 times")
}

func TestNestedInTx(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	uid := uuid.New()
	sqlDB := testSQLDB(t)
	err := migrations.Up(sqlDB)
	require.NoError(t, err, "migrations")

	db := database.New(sqlDB, slog.Logger{})
	err = db.InTx(func(outer database.Store) error {
		return outer.InTx(func(inner database.Store) error {
			//nolint:gocritic
			require.Equal(t, outer, inner, "should be same transaction") // intxcheck:ignore // intentional: test asserts nested InTx returns same store

			_, err := inner.InsertUser(context.Background(), database.InsertUserParams{
				ID:             uid,
				Email:          "coder@coder.com",
				Username:       "coder",
				HashedPassword: []byte{},
				CreatedAt:      dbtime.Now(),
				UpdatedAt:      dbtime.Now(),
				RBACRoles:      []string{},
				LoginType:      database.LoginTypeGithub,
			})
			return err
		}, nil)
	}, nil)
	require.NoError(t, err, "outer tx: %w", err)

	user, err := db.GetUserByID(context.Background(), uid)
	require.NoError(t, err, "user exists")
	require.Equal(t, uid, user.ID, "user id expected")
}

func TestInTx_CapturesRollbackError(t *testing.T) {
	t.Parallel()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := database.New(sqlDB, slog.Logger{})

	callbackErr := xerrors.New("callback failed")
	rollbackErr := xerrors.New("rollback failed")

	mock.ExpectBegin()
	mock.ExpectRollback().WillReturnError(rollbackErr)

	err = db.InTx(func(_ database.Store) error {
		return callbackErr
	}, nil)
	require.EqualError(t, err, "defer (rollback failed): execute transaction: callback failed")
	require.ErrorIs(t, err, callbackErr,
		"returned error should still match the callback error when rollback fails")
	require.NotErrorIs(t, err, rollbackErr,
		"rollback failure should be reported in the message, not wrapped in the error chain")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNestedInTxStricterIsolationDefaultParent(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	// Outer uses default isolation, inner requests RepeatableRead.
	// After normalization default becomes ReadCommitted, so the
	// inner level is stricter and an error should be returned.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	}, nil)
	require.ErrorIs(t, err, database.ErrNestedTransactionIsolationMismatch)
}

func TestNestedInTxStricterIsolationBothExplicit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	// Outer uses RepeatableRead, inner requests Serializable.
	// Both are explicit and the inner is stricter, so an error
	// should be returned.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelSerializable})
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	require.ErrorIs(t, err, database.ErrNestedTransactionIsolationMismatch)
}

func TestNestedInTxSameIsolationNoError(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	// Both use the same isolation level. No error should occur.
	opts := &database.TxOptions{Isolation: sql.LevelRepeatableRead}
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, opts)
	}, opts)
	require.NoError(t, err)
}

func TestNestedInTxWeakerIsolationNoError(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	// Outer uses Serializable, inner requests RepeatableRead (weaker).
	// No error should occur because the inner gets more isolation
	// than needed.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	}, &database.TxOptions{Isolation: sql.LevelSerializable})
	require.NoError(t, err)
}

func TestNestedInTxDefaultVsReadCommittedNoError(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	// Outer uses default (nil opts), inner requests ReadCommitted.
	// After normalization both map to ReadCommitted, so no error
	// should occur.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelReadCommitted})
	}, nil)
	require.NoError(t, err)
}

func TestInTransaction_TopLevel(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})
	require.False(t, db.InTransaction(),
		"top-level Store should not report itself in a transaction")
}

func TestInTransaction_Nested(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, slog.Logger{})

	require.False(t, db.InTransaction())
	var innerVal bool
	err := db.InTx(func(tx database.Store) error {
		innerVal = tx.InTransaction()
		return nil
	}, nil)
	require.NoError(t, err)
	require.True(t, innerVal,
		"Store passed into InTx closure should report itself in a transaction")
}

func testSQLDB(t testing.TB) *sql.DB {
	t.Helper()

	connection, err := dbtestutil.Open(t)
	require.NoError(t, err)

	db, err := sql.Open("postgres", connection)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
