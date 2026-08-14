//go:build !slim

package cli

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// makeScaletestUser creates and authenticates a password-login user with the
// given username so it looks like an existing scaletest user to
// selectExistingUsers. It returns the user as seen by the admin client.
func makeScaletestUser(t *testing.T, client *codersdk.Client, orgID uuid.UUID, username string, roles ...rbac.RoleIdentifier) codersdk.User {
	t.Helper()
	_, user := coderdtest.CreateAnotherUserMutators(t, client, orgID, roles, func(r *codersdk.CreateUserRequestWithOrgs) {
		r.Username = username
		r.Email = username + "@scaletest.local"
	})
	return user
}

func TestSelectExistingUsers(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	for i := 0; i < 3; i++ {
		makeScaletestUser(t, client, first.OrganizationID, fmt.Sprintf("scaletest-notif-%d", i))
	}
	// Ineligible: wrong username prefix, and an existing template admin.
	makeScaletestUser(t, client, first.OrganizationID, "regular-user-0")
	makeScaletestUser(t, client, first.OrganizationID, "scaletest-admin-0", rbac.RoleTemplateAdmin())

	selected, err := selectExistingUsers(ctx, client, first.UserID, 3)
	require.NoError(t, err)
	require.Len(t, selected, 3)
	for _, u := range selected {
		require.True(t, strings.HasPrefix(u.Username, loadtestutil.ScaleTestPrefix+"-"), "selected %q", u.Username)
		require.NotEqual(t, first.UserID, u.ID, "must not select the caller")
		require.False(t, userHasRole(u, codersdk.RoleTemplateAdmin), "must not select an existing template admin")
	}
}

func TestSelectExistingUsersFailsFast(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	makeScaletestUser(t, client, first.OrganizationID, "scaletest-notif-0")

	_, err := selectExistingUsers(ctx, client, first.UserID, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 3 eligible users")
}

func TestPrepareAndRestoreReuseUser(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := makeScaletestUser(t, client, first.OrganizationID, "scaletest-notif-prep")
	u := &reuseUser{user: user, promoted: true}

	tokenName := fmt.Sprintf("%s-notifications", loadtestutil.ScaleTestPrefix)
	require.NoError(t, prepareReuseUser(ctx, client, tokenName, time.Hour, u))

	// Token minted and parseable, and its ID matches what we recorded.
	require.NotEmpty(t, u.sessionToken)
	require.NotEmpty(t, u.tokenID)
	gotID, _, err := httpmw.SplitAPIToken(u.sessionToken)
	require.NoError(t, err)
	require.Equal(t, u.tokenID, gotID)

	// Role promoted server-side.
	refreshed, err := client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.True(t, userHasRole(refreshed, codersdk.RoleTemplateAdmin))

	// The minted token authenticates as the reused user.
	userClient := codersdk.New(client.URL)
	userClient.SetSessionToken(u.sessionToken)
	me, err := userClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Equal(t, user.ID, me.ID)

	// Restore: token revoked and role removed. Refresh so restore sees the role.
	u.user = refreshed
	require.NoError(t, restoreReuseUser(ctx, client, u))

	refreshed, err = client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.False(t, userHasRole(refreshed, codersdk.RoleTemplateAdmin))

	_, err = userClient.User(ctx, codersdk.Me)
	require.Error(t, err, "revoked token must no longer authenticate")
}

func TestRestoreUsersBestEffort(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	good := makeScaletestUser(t, client, first.OrganizationID, "scaletest-notif-good")
	bad := makeScaletestUser(t, client, first.OrganizationID, "scaletest-notif-bad")

	tokenName := fmt.Sprintf("%s-notifications", loadtestutil.ScaleTestPrefix)
	goodU := &reuseUser{user: good, promoted: true}
	require.NoError(t, prepareReuseUser(ctx, client, tokenName, time.Hour, goodU))
	// Refresh so restore sees the promoted role.
	goodRefreshed, err := client.User(ctx, good.ID.String())
	require.NoError(t, err)
	goodU.user = goodRefreshed

	// The bad user is promoted but carries a bogus token ID, so revocation fails.
	_, err = client.UpdateUserRoles(ctx, bad.ID.String(), codersdk.UpdateRoles{
		Roles: append(currentRoleNames(bad), codersdk.RoleTemplateAdmin),
	})
	require.NoError(t, err)
	badRefreshed, err := client.User(ctx, bad.ID.String())
	require.NoError(t, err)
	badU := &reuseUser{user: badRefreshed, promoted: true, tokenID: "0123456789"}

	metrics := notifications.NewMetrics(prometheus.NewRegistry())
	err = restoreUsers(ctx, io.Discard, client, metrics, []reuseUser{*goodU, *badU}, 2)
	require.Error(t, err, "a failing token revocation should surface an error")

	// The good user is fully restored despite the bad one failing.
	goodRefreshed, gerr := client.User(ctx, good.ID.String())
	require.NoError(t, gerr)
	require.False(t, userHasRole(goodRefreshed, codersdk.RoleTemplateAdmin))
}

// chanWriter turns each Write into a message on a channel so a test can read
// progress output deterministically without racing the reporter goroutine.
type chanWriter struct {
	ch chan string
}

func (c *chanWriter) Write(p []byte) (int, error) {
	c.ch <- string(p)
	return len(p), nil
}

func TestProgressWatch(t *testing.T) {
	t.Parallel()

	t.Run("ReportsOnTick", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("progress")
		defer trap.Close()

		cw := &chanWriter{ch: make(chan string, 4)}
		var completed atomic.Int64
		p := progress{w: cw, clock: mClock, verb: "Prepared"}
		stop := p.watch(ctx, 5, &completed)
		defer stop()

		// Wait until the reporter has created its ticker, then release it so the
		// advance below fires a tick deterministically.
		call := trap.MustWait(ctx)
		call.Release(ctx)

		completed.Store(2)
		_, w := mClock.AdvanceNext()
		w.MustWait(ctx)

		line := <-cw.ch
		require.Equal(t, "  Prepared 2/5 users, 15s elapsed\n", line)
	})

	t.Run("NilWriterIsNoOp", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		var completed atomic.Int64
		// Zero value has a nil writer: watch must not start a reporter.
		stop := progress{}.watch(ctx, 5, &completed)
		stop()
	})

	t.Run("ZeroTotalIsNoOp", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		cw := &chanWriter{ch: make(chan string, 1)}
		var completed atomic.Int64
		stop := newProgress(cw, "Prepared").watch(ctx, 0, &completed)
		stop()
		require.Empty(t, cw.ch)
	})
}
