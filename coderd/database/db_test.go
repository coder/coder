//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/migrations"
	"github.com/coder/coder/coderd/database/postgres"
)

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
				CreatedAt:      database.Now(),
				UpdatedAt:      database.Now(),
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
