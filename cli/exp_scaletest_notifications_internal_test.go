//go:build !slim

package cli

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
)

const testTokenName = "scaletest-notifications-test"

// createScaletestUser creates a user whose name matches the scaletest pattern, so
// it is eligible for reuse by selectExistingUsers.
func createScaletestUser(ctx context.Context, t *testing.T, client *codersdk.Client, orgID uuid.UUID, id string, roles ...string) codersdk.User {
	t.Helper()

	pu := &preparedUser{origin: originCreated, id: id}
	require.NoError(t, createAndLoginUser(ctx, client, orgID, pu))
	require.True(t, strings.HasPrefix(pu.user.Username, loadtestutil.ScaleTestPrefix+"-"),
		"created user must carry the scaletest username prefix reuse requires")
	if len(roles) > 0 {
		updated, err := client.UpdateUserRoles(ctx, pu.user.ID.String(), codersdk.UpdateRoles{Roles: roles})
		require.NoError(t, err)
		return updated
	}
	return pu.user
}

func TestSelectExistingUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)

	const poolSize = 5
	for i := range poolSize {
		createScaletestUser(ctx, t, client, firstUser.OrganizationID, strconv.Itoa(i))
	}

	// Ineligible users that the filters must skip. Without these the role and
	// service-account branches never execute and the assertions below are vacuous.
	excludedAdmin := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "admin", codersdk.RoleTemplateAdmin)
	excludedOwner := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "owner", codersdk.RoleOwner)
	// Service accounts are a Premium feature and cannot be created against this
	// AGPL test server, so that exclusion is covered by the filter alone.
	// A non-scaletest user must be skipped even though it is otherwise eligible.
	_, excludedRealUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	require.False(t, loadtestutil.IsScaleTestUser(excludedRealUser.Username, excludedRealUser.Email))

	const userCount = 4
	selected, err := selectExistingUsers(ctx, client, firstUser.UserID, userCount)
	require.NoError(t, err)
	require.Len(t, selected, userCount)

	for _, u := range selected {
		require.NotEqual(t, firstUser.UserID, u.ID, "the caller is excluded")
		require.NotEqual(t, excludedAdmin.ID, u.ID, "existing template admins are excluded")
		require.NotEqual(t, excludedOwner.ID, u.ID, "owners are excluded")
		require.NotEqual(t, excludedRealUser.ID, u.ID, "non-scaletest users are excluded")
		require.True(t, loadtestutil.IsScaleTestUser(u.Username, u.Email))
		require.False(t, userHasRole(u, codersdk.RoleOwner))
		require.False(t, userHasRole(u, codersdk.RoleTemplateAdmin))
	}
}

func TestSelectExistingUsersFailsFast(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)

	// Plenty of users exist, but none of them are scaletest users, so none are
	// eligible for reuse.
	for range 3 {
		coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	}
	createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0")

	_, err := selectExistingUsers(ctx, client, firstUser.UserID, 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires 5 eligible users but only 1 are present")
	require.Contains(t, err.Error(), "--reuse-users")
}

func TestPrepareAndRestoreUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	// One reused user already holds a site role, which must survive the
	// promote/restore round trip.
	auditor := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "auditor", rbac.RoleAuditor().Name)

	const poolSize = 4
	for i := range poolSize - 1 {
		createScaletestUser(ctx, t, client, firstUser.OrganizationID, strconv.Itoa(i))
	}

	selected, err := selectExistingUsers(ctx, client, firstUser.UserID, poolSize)
	require.NoError(t, err)

	candidates := make([]preparedUser, len(selected))
	var sawAuditor bool
	for i, u := range selected {
		isAdmin := u.ID == auditor.ID || i == 0
		if u.ID == auditor.ID {
			sawAuditor = true
			require.True(t, userHasRole(u, codersdk.RoleAuditor))
		}
		candidates[i] = preparedUser{origin: originReused, user: u, isAdmin: isAdmin}
	}
	require.True(t, sawAuditor, "auditor should be among the selected users")

	err = forEachUser(ctx, candidates, 2, func(ctx context.Context, pu *preparedUser) error {
		return prepareUser(ctx, client, testTokenName, time.Hour, pu)
	})
	require.NoError(t, err)

	for _, pu := range candidates {
		require.NotEmpty(t, pu.sessionToken)
		require.NotEmpty(t, pu.tokenID)

		// The minted token must authenticate as the prepared user.
		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.sessionToken))
		me, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, pu.user.ID, me.ID)

		// The token is named after the run so orphans can be found, and expires.
		keys, err := client.Tokens(ctx, pu.user.ID.String(), codersdk.TokensFilter{})
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, testTokenName, keys[0].TokenName)
		require.WithinDuration(t, time.Now().Add(time.Hour), keys[0].ExpiresAt, time.Minute)

		got, err := client.User(ctx, pu.user.ID.String())
		require.NoError(t, err)
		require.Equal(t, pu.isAdmin, userHasRole(got, codersdk.RoleTemplateAdmin))
		if pu.user.ID == auditor.ID {
			require.True(t, userHasRole(got, codersdk.RoleAuditor), "promotion preserves existing roles")
		}
	}

	// A role granted while the run is in flight must survive cleanup, which is the
	// reason cleanup subtracts template-admin instead of replaying a snapshot.
	midRun := candidates[0]
	current, err := currentRoleNames(ctx, client, midRun.user.ID)
	require.NoError(t, err)
	_, err = client.UpdateUserRoles(ctx, midRun.user.ID.String(), codersdk.UpdateRoles{
		Roles: append(current, rbac.RoleUserAdmin().Name),
	})
	require.NoError(t, err)

	err = forEachUserBestEffort(ctx, candidates, 2, func(ctx context.Context, pu *preparedUser) error {
		return restoreUser(ctx, metrics, client, pu)
	})
	require.NoError(t, err)

	for _, pu := range candidates {
		got, err := client.User(ctx, pu.user.ID.String())
		require.NoError(t, err)
		require.False(t, userHasRole(got, codersdk.RoleTemplateAdmin), "template admin is removed")

		if pu.user.ID == auditor.ID {
			require.True(t, userHasRole(got, codersdk.RoleAuditor), "pre-existing roles are kept")
		}
		if pu.user.ID == midRun.user.ID {
			require.True(t, userHasRole(got, codersdk.RoleUserAdmin), "roles granted mid-run are kept")
		}

		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.sessionToken))
		_, err = userClient.User(ctx, codersdk.Me)
		require.Error(t, err)
	}

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, poolSize+1)
}

// TestRestoreUserWithoutRoleMakesNoWrite covers a user setup designated as an
// admin but never reached, which is what a mid-setup abort leaves behind. Cleanup
// must leave its roles alone rather than writing a role set it never changed.
func TestRestoreUserWithoutRoleMakesNoWrite(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	user := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0", rbac.RoleAuditor().Name)
	candidate := preparedUser{origin: originReused, user: user, isAdmin: true}

	require.NoError(t, restoreUser(ctx, metrics, client, &candidate))

	got, err := client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.True(t, userHasRole(got, codersdk.RoleAuditor), "existing roles are untouched")
	require.False(t, userHasRole(got, codersdk.RoleTemplateAdmin))
}

// TestRestoreUserIgnoresUserItNeverPromoted covers the other direction: a user this
// run never designated as an admin must keep a template-admin role granted by
// anyone else, since removing it would be an unaudited privilege change this test
// has no business making.
func TestRestoreUserIgnoresUserItNeverPromoted(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	user := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0")
	// Granted by someone else while the run was in flight.
	_, err := client.UpdateUserRoles(ctx, user.ID.String(), codersdk.UpdateRoles{
		Roles: []string{codersdk.RoleTemplateAdmin},
	})
	require.NoError(t, err)

	candidate := preparedUser{origin: originReused, user: user, isAdmin: false}
	require.NoError(t, restoreUser(ctx, metrics, client, &candidate))

	got, err := client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.True(t, userHasRole(got, codersdk.RoleTemplateAdmin),
		"a role this run never granted must survive cleanup")
}

// TestRestoreUserRemovesRoleCommittedDespiteError is the case a client-side record
// of the promotion cannot cover: the role update commits on the server and the
// client still sees an error. Cleanup must reconcile the account from the server's
// state, or a real account keeps template-admin.
func TestRestoreUserRemovesRoleCommittedDespiteError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	user := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0")

	// The role update reaches the server and commits; only its response is broken,
	// which is what a reset connection or a post-write deadline looks like here.
	brokenClient := brokenResponseClient(t, client, http.MethodPut, "/roles")

	pu := &preparedUser{origin: originReused, user: user, isAdmin: true}
	err := prepareUser(ctx, brokenClient, testTokenName, time.Hour, pu)
	require.ErrorContains(t, err, "promote to template admin")

	// The promotion really happened despite the error the client saw.
	got, err := client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.True(t, userHasRole(got, codersdk.RoleTemplateAdmin),
		"the server committed the promotion")

	// Cleanup reads that state and undoes it, even though this run recorded no
	// successful promotion.
	require.NoError(t, restoreUser(ctx, metrics, client, pu))

	got, err = client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.False(t, userHasRole(got, codersdk.RoleTemplateAdmin),
		"cleanup removes a role the server holds regardless of what the client recorded")
}

func TestPrepareUsersFatalOnFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// A user that does not exist makes token minting fail, which must be fatal.
	candidate := preparedUser{
		user: codersdk.User{ReducedUser: codersdk.ReducedUser{MinimalUser: codersdk.MinimalUser{ID: uuid.New()}}},
	}

	err := prepareUser(ctx, client, testTokenName, time.Hour, &candidate)
	require.ErrorContains(t, err, "create token")
	require.Empty(t, candidate.sessionToken)
}

func TestCreateAndDeleteUsers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	const userCount = 4
	const adminCount = 2
	candidates := make([]preparedUser, userCount)
	for i := range candidates {
		candidates[i] = preparedUser{origin: originCreated, isAdmin: i < adminCount, id: strconv.Itoa(i)}
	}

	err := forEachUser(ctx, candidates, 2, func(ctx context.Context, pu *preparedUser) error {
		return createAndLoginUser(ctx, client, firstUser.OrganizationID, pu)
	})
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, userCount+1)

	for i, pu := range candidates {
		require.NotEqual(t, uuid.Nil, pu.user.ID)
		require.NotEmpty(t, pu.sessionToken)
		// Created users must be reusable by a later run.
		require.True(t, loadtestutil.IsScaleTestUser(pu.user.Username, pu.user.Email))

		userClient := codersdk.New(client.URL, codersdk.WithSessionToken(pu.sessionToken))
		me, err := userClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, pu.user.ID, me.ID)

		got, err := client.User(ctx, pu.user.ID.String())
		require.NoError(t, err)
		require.Equal(t, i < adminCount, userHasRole(got, codersdk.RoleTemplateAdmin))
	}

	err = forEachUserBestEffort(ctx, candidates, 2, func(ctx context.Context, pu *preparedUser) error {
		return deleteUser(ctx, metrics, client, pu)
	})
	require.NoError(t, err)

	users, err = client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, firstUser.UserID, users.Users[0].ID)
}

// failingPathClient returns a client whose requests reach the real deployment
// except those to failPath, which fail. Used to make one step of a multi-step
// helper fail after earlier steps have already taken effect server-side.
func failingPathClient(t *testing.T, target *codersdk.Client, failPath string) *codersdk.Client {
	t.Helper()

	proxy := httputil.NewSingleHostReverseProxy(target.URL)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == failPath {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"injected failure"}`))
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	proxyURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	client := codersdk.New(proxyURL, codersdk.WithSessionToken(target.SessionToken()))
	return client
}

// brokenResponseClient returns a client whose requests reach the real deployment
// and take effect, but whose response to method+pathSuffix is replaced with an
// error. That reproduces a write that commits server-side while the client sees a
// failure, which no client-side record of the outcome can represent.
func brokenResponseClient(t *testing.T, target *codersdk.Client, method, pathSuffix string) *codersdk.Client {
	t.Helper()

	proxy := httputil.NewSingleHostReverseProxy(target.URL)
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request.Method == method && strings.HasSuffix(resp.Request.URL.Path, pathSuffix) {
			return xerrors.New("injected response failure")
		}
		return nil
	}
	srv := httptest.NewServer(proxy)
	t.Cleanup(srv.Close)

	proxyURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return codersdk.New(proxyURL, codersdk.WithSessionToken(target.SessionToken()))
}

// TestCreateAndLoginUserCapturesPartialUser covers why createAndLoginUser assigns
// pu.user before checking the error: creation can succeed while a later step
// fails, and cleanup needs the ID to reclaim the user.
func TestCreateAndLoginUserCapturesPartialUser(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	// Creation succeeds; the login that follows it fails.
	failingClient := failingPathClient(t, client, "/api/v2/users/login")

	pu := &preparedUser{origin: originCreated, id: "partial"}
	err := createAndLoginUser(ctx, failingClient, firstUser.OrganizationID, pu)
	require.Error(t, err)
	require.Empty(t, pu.sessionToken, "login did not complete")

	// The mechanism under test: the created user survives the failure, so cleanup
	// can still find it.
	require.NotEqual(t, uuid.Nil, pu.user.ID)
	require.True(t, loadtestutil.IsScaleTestUser(pu.user.Username, pu.user.Email))

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 2, "the user was really created")

	require.NoError(t, deleteUser(ctx, metrics, client, pu))

	users, err = client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1, "the partially created user is reclaimed")

	// A candidate with no ID must be a no-op rather than an error.
	require.NoError(t, deleteUser(ctx, metrics, client, &preparedUser{origin: originCreated, id: "never"}))
}

// TestRestoreUserContinuesAfterTokenFailure covers restoreUser's internal
// continue-on-error: a failed token revoke must not stop the demotion, since
// leaving template-admin on a user is the worse of the two residues.
func TestRestoreUserContinuesAfterTokenFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	user := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0")
	pu := &preparedUser{origin: originReused, user: user, isAdmin: true}
	require.NoError(t, prepareUser(ctx, client, testTokenName, time.Hour, pu))

	// A token ID that does not exist makes the revoke fail while leaving the
	// promotion in place.
	pu.tokenID = "0123456789"

	err := restoreUser(ctx, metrics, client, pu)
	require.Error(t, err, "the revoke failure is reported")

	// The demotion still happened despite that failure.
	got, err := client.User(ctx, pu.user.ID.String())
	require.NoError(t, err)
	require.False(t, userHasRole(got, codersdk.RoleTemplateAdmin))
}

func makeTestUsers(n int) []preparedUser {
	users := make([]preparedUser, n)
	for i := range users {
		users[i].user = codersdk.User{
			ReducedUser: codersdk.ReducedUser{
				MinimalUser: codersdk.MinimalUser{ID: uuid.New()},
			},
		}
	}
	return users
}

func TestForEachUser(t *testing.T) {
	t.Parallel()

	t.Run("CoversEveryUserOnceWithinLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		users := makeTestUsers(10)
		const limit = 3
		var (
			mu       sync.Mutex
			seen     = map[uuid.UUID]int{}
			inFlight atomic.Int64
			maxSeen  atomic.Int64
		)
		// Block until the limit is reached before returning, so a raised limit really
		// does show up as more calls in flight. Without this every call can finish
		// before the next starts and the assertion holds for any limit.
		atLimit := make(chan struct{})
		var once sync.Once
		err := forEachUser(ctx, users, limit, func(_ context.Context, pu *preparedUser) error {
			n := inFlight.Add(1)
			if n == int64(limit) {
				once.Do(func() { close(atLimit) })
			}
			<-atLimit
			for {
				old := maxSeen.Load()
				if n <= old || maxSeen.CompareAndSwap(old, n) {
					break
				}
			}
			defer inFlight.Add(-1)

			mu.Lock()
			seen[pu.user.ID]++
			mu.Unlock()
			return nil
		})
		require.NoError(t, err)
		require.Len(t, seen, 10)
		for _, count := range seen {
			require.Equal(t, 1, count, "each user is processed exactly once")
		}
		require.LessOrEqual(t, maxSeen.Load(), int64(limit), "never exceeds the concurrency limit")
	})

	t.Run("EmptySliceSkipsFn", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		called := false
		err := forEachUser(ctx, nil, 3, func(context.Context, *preparedUser) error {
			called = true
			return nil
		})
		require.NoError(t, err)
		require.False(t, called)
	})

	t.Run("CancelsSiblingsOnError", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		users := makeTestUsers(2)
		sentinel := xerrors.New("boom")
		var siblingCanceled atomic.Bool
		siblingRunning := make(chan struct{})
		// Both users run concurrently under the limit. The second only fails once the
		// first is known to be inside fn, so the first is guaranteed to observe the
		// cancellation instead of being skipped before it starts.
		err := forEachUser(ctx, users, 2, func(ctx context.Context, pu *preparedUser) error {
			if pu.user.ID == users[0].user.ID {
				close(siblingRunning)
				select {
				case <-ctx.Done():
					siblingCanceled.Store(true)
					return ctx.Err()
				case <-time.After(testutil.WaitShort):
					return nil
				}
			}
			<-siblingRunning
			return sentinel
		})
		require.ErrorIs(t, err, sentinel)
		require.True(t, siblingCanceled.Load())
	})
}

func TestForEachUserBestEffort(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	users := makeTestUsers(4)
	errA := xerrors.New("err-a")
	errB := xerrors.New("err-b")
	var (
		mu             sync.Mutex
		processed      int
		observedCancel bool
	)
	// Every user is processed and its error joined; a failure never cancels the
	// others, which is what stops one failed demotion abandoning the rest.
	err := forEachUserBestEffort(ctx, users, 2, func(ctx context.Context, pu *preparedUser) error {
		mu.Lock()
		processed++
		if ctx.Err() != nil {
			observedCancel = true
		}
		mu.Unlock()
		if pu.user.ID == users[0].user.ID {
			return errA
		}
		return errB
	})
	require.Equal(t, 4, processed)
	require.False(t, observedCancel, "a failing call must not cancel siblings")
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

// TestTriggerRecorder covers the coupling between the notifications the runners
// wait for and the actions the trigger actually performs. Drift between the two
// used to be silent: a runner waited for a notification nothing sent, and the run
// burned its whole budget before failing with delivery errors.
func TestTriggerRecorder(t *testing.T) {
	t.Parallel()

	expectedID := uuid.New()

	t.Run("RecordsAndVerifies", func(t *testing.T) {
		t.Parallel()

		ch := make(chan time.Time, 1)
		rec := newTriggerRecorder(map[uuid.UUID]chan time.Time{expectedID: ch})

		before := time.Now()
		require.NoError(t, rec.record(expectedID))
		require.NoError(t, rec.verifyAll())

		// The trigger time is readable and the channel closed, so the latency
		// computation cannot miss it.
		got, ok := <-ch
		require.True(t, ok)
		require.False(t, got.Before(before))
		_, stillOpen := <-ch
		require.False(t, stillOpen, "channel is closed after recording")
	})

	t.Run("ExpectedNotificationWithNoAction", func(t *testing.T) {
		t.Parallel()

		// A second expected notification that no action triggers must be reported,
		// not silently waited on by the runners.
		unsentID := uuid.New()
		rec := newTriggerRecorder(map[uuid.UUID]chan time.Time{
			expectedID: make(chan time.Time, 1),
			unsentID:   make(chan time.Time, 1),
		})
		require.NoError(t, rec.record(expectedID))

		err := rec.verifyAll()
		require.ErrorContains(t, err, unsentID.String())
		require.ErrorContains(t, err, "no action triggers expected notification")
	})

	t.Run("TriggeredNotificationNobodyExpects", func(t *testing.T) {
		t.Parallel()

		rec := newTriggerRecorder(map[uuid.UUID]chan time.Time{})
		err := rec.record(expectedID)
		require.ErrorContains(t, err, "no runner is waiting for")
	})

	t.Run("DoubleRecordIsRejected", func(t *testing.T) {
		t.Parallel()

		rec := newTriggerRecorder(map[uuid.UUID]chan time.Time{expectedID: make(chan time.Time, 1)})
		require.NoError(t, rec.record(expectedID))
		// Recording twice would panic on the closed channel, so it is rejected.
		require.ErrorContains(t, rec.record(expectedID), "triggered more than once")
	})
}

// TestUserSinksRefuseWrongOrigin covers the guards that keep the two cleanup paths
// from crossing: deleting a reused account is unrecoverable, and restoring a
// created one means the caller mixed up the paths.
func TestUserSinksRefuseWrongOrigin(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	user := createScaletestUser(ctx, t, client, firstUser.OrganizationID, "0")

	reused := &preparedUser{origin: originReused, user: user, isAdmin: true}
	require.ErrorContains(t, deleteUser(ctx, metrics, client, reused), "did not create")

	created := &preparedUser{origin: originCreated, user: user, isAdmin: true}
	require.ErrorContains(t, restoreUser(ctx, metrics, client, created), "this run created")

	// The refusal is real: the user still exists.
	got, err := client.User(ctx, user.ID.String())
	require.NoError(t, err)
	require.Equal(t, user.ID, got.ID)
}

// createEmptyTemplate creates a template with the given name so a test can assert
// what the sweep does and does not delete.
func createEmptyTemplate(ctx context.Context, t *testing.T, client *codersdk.Client, orgID uuid.UUID, name string) codersdk.Template {
	t.Helper()

	version := coderdtest.CreateTemplateVersion(t, client, orgID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	return coderdtest.CreateTemplate(t, client, orgID, version.ID, func(req *codersdk.CreateTemplateRequest) {
		req.Name = name
	})
}

// TestSweepStaleTemplates covers the sweep that stops trigger templates
// accumulating when runs are killed between creating and deleting one. Its guards
// decide what gets deleted from a live deployment, so each one is exercised here.
func TestSweepStaleTemplates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	firstUser := coderdtest.CreateFirstUser(t, client)

	run := &scaletestRun{
		logger:       testutil.Logger(t),
		client:       client,
		orgID:        firstUser.OrganizationID,
		templateName: notificationsPrefix + "thisrun",
	}

	stale := createEmptyTemplate(ctx, t, client, firstUser.OrganizationID, notificationsPrefix+"earlier")
	mine := createEmptyTemplate(ctx, t, client, firstUser.OrganizationID, run.templateName)
	unrelated := createEmptyTemplate(ctx, t, client, firstUser.OrganizationID, "production-template")
	// Contains the prefix but does not start with it. The server-side filter is a
	// substring match, so this comes back from the query and only the client-side
	// prefix check keeps it alive.
	lookalike := createEmptyTemplate(ctx, t, client, firstUser.OrganizationID, "x-"+notificationsPrefix+"c")

	require.NoError(t, run.sweepStaleTemplates(ctx))

	remaining, err := client.TemplatesByOrganization(ctx, firstUser.OrganizationID)
	require.NoError(t, err)

	names := make([]string, 0, len(remaining))
	for _, tpl := range remaining {
		names = append(names, tpl.Name)
	}
	require.NotContains(t, names, stale.Name, "a template left by an earlier run is deleted")
	require.Contains(t, names, mine.Name, "this run's own template is kept")
	require.Contains(t, names, unrelated.Name, "templates without the prefix are never touched")
	require.Contains(t, names, lookalike.Name, "a name containing the prefix is not a name starting with it")
}
