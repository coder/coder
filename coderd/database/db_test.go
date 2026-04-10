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
	"github.com/coder/coder/v2/testutil"
)

func TestSerializedRetry(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB)

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

	db := database.New(sqlDB)
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

	db := database.New(sqlDB)

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

	sink := testutil.NewFakeSink(t)
	logger := slog.Make(sink).Leveled(slog.LevelDebug)

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, database.WithLogger(logger))

	// Outer uses default isolation, inner requests RepeatableRead.
	// The inner level is stricter, so a Critical log should fire.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	}, nil)
	require.NoError(t, err)

	entries := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelCritical
	})
	require.Len(t, entries, 1, "expected exactly one Critical log entry")
	require.Contains(t, entries[0].Message, "nested transaction requested stricter isolation level")

	var parentVal, requestedVal string
	for _, f := range entries[0].Fields {
		switch f.Name {
		case "parent_isolation":
			parentVal, _ = f.Value.(string)
		case "requested_isolation":
			requestedVal, _ = f.Value.(string)
		}
	}
	require.Equal(t, sql.LevelDefault.String(), parentVal)
	require.Equal(t, sql.LevelRepeatableRead.String(), requestedVal)
}

func TestNestedInTxStricterIsolationBothExplicit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sink := testutil.NewFakeSink(t)
	logger := slog.Make(sink).Leveled(slog.LevelDebug)

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, database.WithLogger(logger))

	// Outer uses RepeatableRead, inner requests Serializable.
	// Both are explicit and the inner is stricter, so a Critical
	// log should fire.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelSerializable})
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	require.NoError(t, err)

	entries := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelCritical
	})
	require.Len(t, entries, 1, "expected exactly one Critical log entry")
	require.Contains(t, entries[0].Message, "nested transaction requested stricter isolation level")

	var parentVal, requestedVal string
	for _, f := range entries[0].Fields {
		switch f.Name {
		case "parent_isolation":
			parentVal, _ = f.Value.(string)
		case "requested_isolation":
			requestedVal, _ = f.Value.(string)
		}
	}
	require.Equal(t, sql.LevelRepeatableRead.String(), parentVal)
	require.Equal(t, sql.LevelSerializable.String(), requestedVal)
}

func TestNestedInTxSameIsolationNoLog(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sink := testutil.NewFakeSink(t)
	logger := slog.Make(sink).Leveled(slog.LevelDebug)

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, database.WithLogger(logger))

	// Both use the same isolation level. No log should fire.
	opts := &database.TxOptions{Isolation: sql.LevelRepeatableRead}
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, opts)
	}, opts)
	require.NoError(t, err)

	entries := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelCritical
	})
	require.Empty(t, entries, "should not log when isolation levels match")
}

func TestNestedInTxWeakerIsolationNoLog(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sink := testutil.NewFakeSink(t)
	logger := slog.Make(sink).Leveled(slog.LevelDebug)

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB, database.WithLogger(logger))

	// Outer uses Serializable, inner requests RepeatableRead (weaker).
	// No log should fire because the inner gets more isolation than needed.
	err := db.InTx(func(outer database.Store) error {
		return outer.InTx(func(_ database.Store) error {
			return nil
		}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	}, &database.TxOptions{Isolation: sql.LevelSerializable})
	require.NoError(t, err)

	entries := sink.Entries(func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelCritical
	})
	require.Empty(t, entries, "should not log when inner isolation is weaker than outer")
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
