package database_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
)

func TestNestedInTx(t *testing.T) {
	uid := uuid.New()
	sqlDB := testSQLDB(t)
	err := database.MigrateUp(sqlDB)
	require.NoError(t, err, "migrations")

	db := database.New(sqlDB)
	err = db.InTx(func(outer database.Store) error {
		return outer.InTx(func(inner database.Store) error {
			require.Equal(t, outer, inner, "should be same transaction")

			_, err := inner.InsertUser(context.Background(), database.InsertUserParams{
				ID:             uid,
				Email:          "coder@coder.com",
				Username:       "coder",
				HashedPassword: []byte{},
				CreatedAt:      database.Now(),
				UpdatedAt:      database.Now(),
				RBACRoles:      []string{},
			})
			return err
		})
	})
	require.NoError(t, err, "outer tx: %w", err)

	user, err := db.GetUserByID(context.Background(), uid)
	require.NoError(t, err, "user exists")
	require.Equal(t, uid, user.ID, "user id expected")
}
