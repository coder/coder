//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/database/postgres"
)

func TestSerializedRetry(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	db := database.New(sqlDB)

	called := 0
	txOpts := &sql.TxOptions{Isolation: sql.LevelSerializable}
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
			require.Equal(t, outer, inner, "should be same transaction")

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

func testSQLDB(t testing.TB) *sql.DB {
	t.Helper()

	connection, closeFn, err := postgres.Open()
	require.NoError(t, err)
	t.Cleanup(closeFn)

	db, err := sql.Open("postgres", connection)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
