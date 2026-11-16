package coderd_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentrelay"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/x/workspacerelay"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// relayWorkspace is a workspace with a daemon and the session token a build
// would have given its owner.
type relayWorkspace struct {
	workspace    database.WorkspaceTable
	agent        database.WorkspaceAgent
	daemon       *agentsdk.Client
	sessionToken string
}

func newRelayWorkspace(t *testing.T, client *codersdk.Client, db database.Store, orgID, ownerID uuid.UUID) relayWorkspace {
	t.Helper()
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
	}).WithAgent().Do()
	_, token := dbgen.APIKey(t, db, database.APIKey{
		UserID:    ownerID,
		LoginType: database.LoginTypePassword,
		TokenName: provisionerdserver.WorkspaceSessionTokenName(ownerID, build.Workspace.ID),
	})
	return relayWorkspace{
		workspace:    build.Workspace,
		agent:        build.Agents[0],
		daemon:       agentsdk.New(client.URL, agentsdk.WithFixedToken(build.AgentToken)),
		sessionToken: token,
	}
}

// relay starts the relay of a workspace's daemon, as the daemon does.
func (w relayWorkspace) relay(t *testing.T, client *codersdk.Client) *agentrelay.Relay {
	t.Helper()
	relay, err := agentrelay.New(agentrelay.Options{
		Logger:       testutil.Logger(t).Named("relay"),
		ControlPlane: client.URL,
		Lease:        w.daemon.WorkspaceRelayLease,
		Release:      w.daemon.ReleaseWorkspaceRelay,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = relay.Close() })
	relay.Ensure(testutil.Context(t, testutil.WaitShort))
	return relay
}

func clientAt(t *testing.T, rawURL, token string) *codersdk.Client {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	client := codersdk.New(parsed)
	client.SetSessionToken(token)
	return client
}

// withLease sends a lease with every request, as somebody who got hold of
// one would.
type withLease struct {
	lease string
	next  http.RoundTripper
}

func (t withLease) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(workspacerelay.LeaseHeader, t.lease)
	return t.next.RoundTrip(r)
}

func signedLeaseRequest(t *testing.T) (agentsdk.WorkspaceRelayLeaseRequest, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return signLeaseRequest(publicKey, privateKey, time.Now()), privateKey
}

func signLeaseRequest(publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, at time.Time) agentsdk.WorkspaceRelayLeaseRequest {
	return agentsdk.WorkspaceRelayLeaseRequest{
		PublicKey: publicKey,
		Timestamp: at,
		Signature: ed25519.Sign(privateKey, workspacerelay.SigningInput(at)),
	}
}

func TestWorkspaceRelay(t *testing.T) {
	t.Parallel()

	t.Run("Enforce", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			WorkspaceRelayMode: workspacerelay.ModeEnforce,
		})
		db := api.Database
		admin := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		first := newRelayWorkspace(t, client, db, admin.OrganizationID, admin.UserID)
		second := newRelayWorkspace(t, client, db, admin.OrganizationID, admin.UserID)

		// The session token of a workspace does not work on its own,
		// wherever it is used from.
		_, err := clientAt(t, client.URL.String(), first.sessionToken).User(ctx, codersdk.Me)
		refused := requireStatus(t, err, http.StatusUnauthorized)
		require.Contains(t, refused.Message, "only accepted through that workspace's daemon")

		// It works through the workspace's daemon.
		relay := first.relay(t, client)
		require.NotEmpty(t, relay.URL())
		me, err := clientAt(t, relay.URL(), first.sessionToken).User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, admin.UserID, me.ID)

		// The daemon of one workspace vouches for nothing in another.
		_, err = clientAt(t, relay.URL(), second.sessionToken).User(ctx, codersdk.Me)
		requireStatus(t, err, http.StatusUnauthorized)

		// Other credentials are as they were: a user's session, and a
		// token a user made, whatever the user called it.
		_, err = client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		_, userToken := dbgen.APIKey(t, db, database.APIKey{
			UserID:    admin.UserID,
			LoginType: database.LoginTypeToken,
			TokenName: provisionerdserver.WorkspaceSessionTokenName(admin.UserID, first.workspace.ID),
		})
		_, err = clientAt(t, client.URL.String(), userToken).User(ctx, codersdk.Me)
		require.NoError(t, err)

		// A lease that has run out is worth nothing.
		expired, _, err := workspacerelay.IssueLease(ctx, api.AppSigningKeyCache, time.Now().Add(-time.Hour), time.Minute,
			first.workspace.ID, first.agent.ID, 1)
		require.NoError(t, err)
		stale := clientAt(t, client.URL.String(), first.sessionToken)
		stale.HTTPClient.Transport = withLease{lease: expired, next: http.DefaultTransport}
		_, err = stale.User(ctx, codersdk.Me)
		requireStatus(t, err, http.StatusUnauthorized)

		// AI Gateway authenticates tokens by itself. The relay applies
		// there as well, and the lease is not passed on.
		var reached, leasePassedOn bool
		gateway := api.WorkspaceRelayAIGateway()(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			reached = true
			leasePassedOn = r.Header.Get(workspacerelay.LeaseHeader) != ""
			rw.WriteHeader(http.StatusNoContent)
		}))
		bare := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v2/aibridge/anthropic/v1/messages", nil)
		bare.Header.Set("Authorization", "Bearer "+first.sessionToken)
		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, bare)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.False(t, reached)

		valid, _, err := workspacerelay.IssueLease(ctx, api.AppSigningKeyCache, time.Now(), time.Minute,
			first.workspace.ID, first.agent.ID, 1)
		require.NoError(t, err)
		relayed := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v2/aibridge/anthropic/v1/messages", nil)
		relayed.Header.Set("Authorization", "Bearer "+first.sessionToken)
		relayed.Header.Set(workspacerelay.LeaseHeader, valid)
		rec = httptest.NewRecorder()
		gateway.ServeHTTP(rec, relayed)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.True(t, reached)
		require.False(t, leasePassedOn)
	})

	t.Run("OneDaemonInstance", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			WorkspaceRelayMode: workspacerelay.ModeEnforce,
		})
		admin := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newRelayWorkspace(t, client, db, admin.OrganizationID, admin.UserID)

		// The daemon obtains a lease and renews it.
		holder, holderKey := signedLeaseRequest(t)
		lease, err := ws.daemon.WorkspaceRelayLease(ctx, holder)
		require.NoError(t, err)
		require.EqualValues(t, 1, lease.Generation)
		require.True(t, lease.Enforced)
		lease, err = ws.daemon.WorkspaceRelayLease(ctx, signLeaseRequest(holder.PublicKey, holderKey, time.Now()))
		require.NoError(t, err)
		require.EqualValues(t, 1, lease.Generation)

		// Somebody with a copy of the daemon's token, and a key of their
		// own, is refused while the daemon is alive.
		intruder, intruderKey := signedLeaseRequest(t)
		_, err = ws.daemon.WorkspaceRelayLease(ctx, intruder)
		requireStatus(t, err, http.StatusConflict)

		// A request that is not signed by the key it names is refused, and
		// so is one that was signed long ago.
		forged := holder
		forged.Signature = intruder.Signature
		_, err = ws.daemon.WorkspaceRelayLease(ctx, forged)
		requireStatus(t, err, http.StatusBadRequest)
		_, err = ws.daemon.WorkspaceRelayLease(ctx, signLeaseRequest(holder.PublicKey, holderKey, time.Now().Add(-time.Hour)))
		requireStatus(t, err, http.StatusBadRequest)

		// Once the holder has stopped renewing for as long as a lease
		// lasts, another instance takes over. Here the holder's last
		// renewal is moved into the past.
		//nolint:gocritic // The test reaches into the store to age the holder's renewal.
		_, err = db.ClaimWorkspaceAgentRelay(dbauthz.AsSystemRestricted(ctx), database.ClaimWorkspaceAgentRelayParams{
			WorkspaceAgentID: ws.agent.ID,
			PublicKey:        holder.PublicKey,
			Now:              time.Now().Add(-2 * workspacerelay.DefaultLeaseTTL),
			LapsedBefore:     time.Now().Add(-3 * workspacerelay.DefaultLeaseTTL),
		})
		require.NoError(t, err)
		lease, err = ws.daemon.WorkspaceRelayLease(ctx, signLeaseRequest(intruder.PublicKey, intruderKey, time.Now()))
		require.NoError(t, err)
		require.EqualValues(t, 2, lease.Generation)

		// The instance that was replaced can no longer renew.
		_, err = ws.daemon.WorkspaceRelayLease(ctx, signLeaseRequest(holder.PublicKey, holderKey, time.Now()))
		requireStatus(t, err, http.StatusConflict)

		// A daemon that stops gives the relay up, and the next instance
		// does not wait.
		require.NoError(t, ws.daemon.ReleaseWorkspaceRelay(ctx, signLeaseRequest(intruder.PublicKey, intruderKey, time.Now())))
		lease, err = ws.daemon.WorkspaceRelayLease(ctx, signLeaseRequest(holder.PublicKey, holderKey, time.Now()))
		require.NoError(t, err)
		require.EqualValues(t, 3, lease.Generation)
	})

	t.Run("Log", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			WorkspaceRelayMode: workspacerelay.ModeLog,
		})
		admin := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newRelayWorkspace(t, client, db, admin.OrganizationID, admin.UserID)

		// Nothing is refused. Daemons relay already, so that an operator
		// can see what does not go through them.
		_, err := clientAt(t, client.URL.String(), ws.sessionToken).User(ctx, codersdk.Me)
		require.NoError(t, err)
		relay := ws.relay(t, client)
		require.NotEmpty(t, relay.URL())
		_, err = clientAt(t, relay.URL(), ws.sessionToken).User(ctx, codersdk.Me)
		require.NoError(t, err)
	})

	t.Run("Off", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newRelayWorkspace(t, client, db, admin.OrganizationID, admin.UserID)

		_, err := clientAt(t, client.URL.String(), ws.sessionToken).User(ctx, codersdk.Me)
		require.NoError(t, err)
		// The daemon gets no lease, and gives processes no address.
		request, _ := signedLeaseRequest(t)
		_, err = ws.daemon.WorkspaceRelayLease(ctx, request)
		requireStatus(t, err, http.StatusNotFound)
		require.Empty(t, ws.relay(t, client).URL())
	})
}
