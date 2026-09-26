package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/testutil"
)

// TestDeletedOwnerAuthorizationIsTerminal pins the deleted-owner
// contract of the chatd authorization helpers: fail closed, keep the
// httpmw.ErrUserDeleted sentinel chain intact, and mark the error
// terminal so the generation retry loop does not burn its attempts on
// a permanently unauthorizable owner.
func TestDeletedOwnerAuthorizationIsTerminal(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})

	setupCtx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, db.UpdateUserDeletedByID(setupCtx, user.ID))

	t.Run("ForcedMCPServerConfigs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := forcedMCPServerConfigsForOwner(ctx, db, org.ID, user.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, httpmw.ErrUserDeleted, "sentinel chain must survive wrapping")
		require.True(t, isTerminalGeneration(err), "deleted owner must stop the generation retry loop")
	})

	t.Run("CallerModelConfigContext", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := callerModelConfigContext(ctx, db, user.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, httpmw.ErrUserDeleted, "sentinel chain must survive wrapping")
		require.True(t, isTerminalGeneration(err), "deleted owner must stop the generation retry loop")
	})

	t.Run("UserSkillContext", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := userSkillContext(ctx, db, user.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, httpmw.ErrUserDeleted, "personal skills must not load for a deleted owner")
	})
}
