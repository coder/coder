package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatAutomationsOwnerFilter checks the owner filter of the list:
// owner=me returns only the caller's automations, no filter returns every
// automation the caller can read, and a filter never shows an automation
// RBAC hides.
func TestChatAutomationsOwnerFilter(t *testing.T) {
	t.Parallel()
	env := newChatAutomationTestEnv(t, nil, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	admin, err := env.owner.User(ctx, codersdk.Me)
	require.NoError(t, err)
	member, err := env.member.User(ctx, codersdk.Me)
	require.NoError(t, err)

	adminOwned, err := env.owner.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
	require.NoError(t, err)
	memberOwned, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
	require.NoError(t, err)

	ids := func(t *testing.T, client *codersdk.ExperimentalClient, owner string) []uuid.UUID {
		t.Helper()
		list, err := client.ChatAutomations(ctx, env.orgID, codersdk.ChatAutomationsFilter{Owner: owner})
		require.NoError(t, err, "owner=%q", owner)
		got := make([]uuid.UUID, 0, len(list))
		for _, automation := range list {
			got = append(got, automation.ID)
		}
		return got
	}

	// Without a filter, the admin sees every automation they can read.
	require.ElementsMatch(t, []uuid.UUID{adminOwned.Automation.ID, memberOwned.Automation.ID}, ids(t, env.owner, ""))
	// owner=me, the admin's username, and the admin's ID all mean the admin.
	for _, owner := range []string{codersdk.Me, admin.Username, admin.ID.String()} {
		require.Equal(t, []uuid.UUID{adminOwned.Automation.ID}, ids(t, env.owner, owner), "owner=%q", owner)
	}
	// The admin can filter by another owner.
	require.Equal(t, []uuid.UUID{memberOwned.Automation.ID}, ids(t, env.owner, member.Username))

	// The member sees only their own, with or without a filter, and a
	// filter on the admin shows nothing because RBAC hides it.
	require.Equal(t, []uuid.UUID{memberOwned.Automation.ID}, ids(t, env.member, ""))
	require.Equal(t, []uuid.UUID{memberOwned.Automation.ID}, ids(t, env.member, codersdk.Me))
	require.Empty(t, ids(t, env.member, admin.Username))
	require.Empty(t, ids(t, env.member, admin.ID.String()))

	// An unknown username is a bad request that names the parameter.
	_, err = env.owner.ChatAutomations(ctx, env.orgID, codersdk.ChatAutomationsFilter{Owner: "no-such-user"})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Len(t, sdkErr.Validations, 1)
	require.Equal(t, "owner", sdkErr.Validations[0].Field)
}
