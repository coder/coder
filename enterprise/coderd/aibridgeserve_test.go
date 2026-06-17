package coderd_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"

	aibridgedproto "github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
)

// dialAIBridgeServe dials /api/v2/ai-gateway/serve, authenticating with the given
// gateway key and API version. On a successful WebSocket upgrade it returns a
// yamux session and http.StatusSwitchingProtocols. Otherwise it returns a nil
// session and the HTTP status code coderd responded with.
func dialAIBridgeServe(ctx context.Context, t *testing.T, client *codersdk.Client, key, version string) (*yamux.Session, int) {
	t.Helper()

	serverURL, err := client.URL.Parse("/api/v2/ai-gateway/serve")
	require.NoError(t, err)
	query := serverURL.Query()
	if version != "" {
		query.Set("version", version)
	}
	serverURL.RawQuery = query.Encode()

	headers := http.Header{}
	if key != "" {
		headers.Set(codersdk.AIGatewayKeyHeader, key)
	}

	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient:      &http.Client{Transport: client.HTTPClient.Transport},
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      headers,
	})
	if err != nil {
		statusCode := 0
		if res != nil {
			statusCode = res.StatusCode
			_ = res.Body.Close()
		}
		return nil, statusCode
	}
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	_, wsNetConn := codersdk.WebsocketNetConn(context.Background(), conn, websocket.MessageBinary)
	conn.SetReadLimit(drpcsdk.YamuxDefaultStreamWindowSize)
	session, err := yamux.Client(wsNetConn, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = session.Close()
		_ = wsNetConn.Close()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	return session, http.StatusSwitchingProtocols
}

func TestAIBridgeServe(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		client, firstUser := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-success"})
		require.NoError(t, err)

		session, status := dialAIBridgeServe(ctx, t, client, created.Key, aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusSwitchingProtocols, status)
		require.NotNil(t, session)

		// The Authorizer service should be served and authorize the owner's
		// session token, exercising a full DRPC round trip over the WebSocket.
		authorizer := aibridgedproto.NewDRPCAuthorizerClient(drpcsdk.MultiplexedConn(session))
		resp, err := authorizer.IsAuthorized(ctx, &aibridgedproto.IsAuthorizedRequest{
			Key: client.SessionToken(),
		})
		require.NoError(t, err)
		require.Equal(t, firstUser.UserID.String(), resp.GetOwnerId())

		// The session records liveness for the authenticating key.
		require.Eventually(t, func() bool {
			//nolint:gocritic // Owner role is irrelevant for gateway key management here.
			keys, err := client.ListAIGatewayKeys(ctx)
			if err != nil {
				return false
			}
			for _, k := range keys {
				if k.ID == created.ID {
					return k.LastUsedAt != nil
				}
			}
			return false
		}, testutil.WaitMedium, testutil.IntervalFast)
	})

	t.Run("MissingKey", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		_, status := dialAIBridgeServe(ctx, t, client, "", aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("InvalidKey", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		_, status := dialAIBridgeServe(ctx, t, client, "not-a-real-key", aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("RevokedKey", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-revoked"})
		require.NoError(t, err)
		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		require.NoError(t, client.DeleteAIGatewayKey(ctx, created.ID))

		_, status := dialAIBridgeServe(ctx, t, client, created.Key, aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("IncompatibleVersion", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-badversion"})
		require.NoError(t, err)

		_, status := dialAIBridgeServe(ctx, t, client, created.Key, "999.0")
		require.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("MissingVersion", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-missing-version"})
		require.NoError(t, err)

		_, status := dialAIBridgeServe(ctx, t, client, created.Key, "")
		require.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("DeletedKeyClosesActiveSession", func(t *testing.T) {
		t.Parallel()
		tick := make(chan time.Time, 1)
		opts := aibridgeOpts(t)
		opts.Options.NewTicker = func(time.Duration) (<-chan time.Time, func()) {
			return tick, func() {}
		}

		client, _ := coderdenttest.New(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-delete-active"})
		require.NoError(t, err)

		session, status := dialAIBridgeServe(ctx, t, client, created.Key, aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusSwitchingProtocols, status)
		require.NotNil(t, session)

		//nolint:gocritic // Owner role is irrelevant for gateway key management here.
		require.NoError(t, client.DeleteAIGatewayKey(ctx, created.ID))

		tick <- time.Now()
		require.Eventually(t, func() bool {
			select {
			case <-session.CloseChan():
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("MissingEntitlement", func(t *testing.T) {
		t.Parallel()
		// Enable the bridge config but do not grant the FeatureAIBridge license.
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		_, status := dialAIBridgeServe(ctx, t, client, "any-key", aibridgedproto.CurrentVersion.String())
		require.Equal(t, http.StatusForbidden, status)
	})
}
