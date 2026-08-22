package database_test

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

// An API key is held by an actor, and an actor is not always a user. These
// tests exercise the case the holder pair exists for, which nothing else in
// the tree does: a key whose holder is not a row in users at all.
func TestAPIKeyHolder(t *testing.T) {
	t.Parallel()

	insert := func(t *testing.T, db database.Store, holder database.HolderID, kind database.HolderType) (database.APIKey, error) {
		t.Helper()
		return db.InsertAPIKey(testutil.Context(t, testutil.WaitShort), database.InsertAPIKeyParams{
			ID:              uuid.NewString(),
			HolderID:        holder,
			HolderType:      kind,
			HashedSecret:    []byte("hashed"),
			LastUsed:        dbtime.Now(),
			ExpiresAt:       dbtime.Now().Add(time.Hour),
			CreatedAt:       dbtime.Now(),
			UpdatedAt:       dbtime.Now(),
			LoginType:       database.LoginTypeToken,
			Scopes:          database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			LifetimeSeconds: int64(time.Hour.Seconds()),
			IPAddress: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.IPv4(127, 0, 0, 1),
					Mask: net.IPv4Mask(255, 255, 255, 255),
				},
				Valid: true,
			},
		})
	}

	// The point of the change. Before it, a key's holder was a foreign key into
	// users, so an identifier belonging to anything else could not be stored.
	t.Run("HolderNeedNotBeAUser", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Deliberately not created anywhere. An AI agent's identity lives in
		// its own ledger, and nothing in users corresponds to it.
		agent := database.HolderID(uuid.New())

		key, err := insert(t, db, agent, database.HolderTypeAIAgent)
		require.NoError(t, err, "a holder that is not a user should be storable")
		require.Equal(t, agent, key.HolderID)
		require.Equal(t, database.HolderTypeAIAgent, key.HolderType)

		got, err := db.GetAPIKeyByID(ctx, key.ID)
		require.NoError(t, err)
		require.Equal(t, agent, got.HolderID, "the holder should round trip unchanged")
		require.Equal(t, database.HolderTypeAIAgent, got.HolderType)
	})

	// The trigger refusing keys for deleted users consults users, and must not
	// do so for a holder that could never be one.
	t.Run("DeletedUserTriggerIgnoresNonUsers", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		require.NoError(t, db.UpdateUserDeletedByID(testutil.Context(t, testutil.WaitShort), user.ID))

		_, err := insert(t, db, database.HolderID(user.ID), database.HolderTypeUser)
		require.ErrorContains(t, err, "Cannot create API key for deleted user", "a deleted user may not be given a key")

		_, err = insert(t, db, database.HolderID(user.ID), database.HolderTypeAIAgent)
		require.NoError(t, err, "the same identifier held as an AI agent is not a deleted user")
	})

	// The set is closed by a CHECK rather than an enum, which is recorded as a
	// proof of concept cheat. This asserts the constraint is real either way.
	t.Run("HolderTypeIsClosed", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)

		_, err := insert(t, db, database.HolderID(uuid.New()), database.HolderType("sandbox"))
		require.ErrorContains(t, err, "api_keys_holder_type_check", "a holder type naming no identity table should be refused by the check constraint")

		_, err = insert(t, db, database.HolderID(uuid.New()), database.HolderType(""))
		require.ErrorContains(t, err, "api_keys_holder_type_check", "an unset holder type should be refused by the check constraint")
	})
}
