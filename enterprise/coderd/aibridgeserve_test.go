package coderd_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/aibridged"
	aibridgedproto "github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	entcoderd "github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
)

// manualDialAIGatewayServe dials /api/v2/ai-gateway/serve, authenticating with the given
// gateway key and API version. On a successful WebSocket upgrade it returns a
// yamux session and http.StatusSwitchingProtocols. Otherwise it returns a nil
// session and the HTTP status code coderd responded with.
func manualDialAIGatewayServe(ctx context.Context, t *testing.T, client *codersdk.Client, key string, version string) (*yamux.Session, int) {
	t.Helper()

	serverURL, err := client.URL.Parse("/api/v2/ai-gateway/serve")
	require.NoError(t, err)
	query := serverURL.Query()
	if version != "" {
		query.Set(aibridgedproto.VersionQueryParam, version)
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

// dialAIGatewayServe connects to /api/v2/ai-gateway/serve using the production NewWebsocketDialer.
func dialAIGatewayServe(ctx context.Context, t *testing.T, client *codersdk.Client, key string) (aibridged.DRPCClient, error) {
	t.Helper()

	dc, err := aibridged.NewWebsocketDialer(client.URL, client.HTTPClient.Transport, key)(ctx)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		_ = dc.DRPCConn().Close()
	})
	return dc, nil
}

func TestAIGatewayServeSuccess(t *testing.T) {
	t.Parallel()

	client, firstUser := coderdenttest.New(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	//nolint:gocritic // Owner role is needed for gateway key management.
	created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-success"})
	require.NoError(t, err)

	// Use NewWebsocketDialer that production code of standalone gateway uses
	dc, err := dialAIGatewayServe(ctx, t, client, created.Key)
	require.NoError(t, err)

	// Exercise one RPC from each service in the DRPCClient union to verify the
	// dialer wires every service and the serve mux registers them all.

	// DRPCAuthorizerClient
	resp, err := dc.IsAuthorized(ctx, &aibridgedproto.IsAuthorizedRequest{Key: client.SessionToken()})
	require.NoError(t, err)
	require.Equal(t, firstUser.UserID.String(), resp.GetOwnerId())

	// DRPCProviderConfiguratorClient
	_, err = dc.GetAIProviders(ctx, &aibridgedproto.GetAIProvidersRequest{})
	require.NoError(t, err)

	// DRPCMCPConfiguratorClient
	_, err = dc.GetMCPServerConfigs(ctx, &aibridgedproto.GetMCPServerConfigsRequest{UserId: firstUser.UserID.String()})
	require.NoError(t, err)

	// DRPCRecorderClient
	_, err = dc.RecordInterception(ctx, &aibridgedproto.RecordInterceptionRequest{
		Id:          uuid.NewString(),
		InitiatorId: firstUser.UserID.String(),
		ApiKeyId:    "serve-success-key",
		Provider:    "openai",
		Model:       "gpt-4",
		StartedAt:   timestamppb.Now(),
	})
	require.NoError(t, err)

	// Verify the session records liveness for the authenticating key.
	require.Eventually(t, func() bool {
		//nolint:gocritic // Owner role is needed for gateway key management.
		keys, err := client.ListAIGatewayKeys(ctx)
		if err != nil {
			return false
		}
		for _, k := range keys {
			if k.ID == created.ID {
				return k.LastHeartbeatAt != nil
			}
		}
		return false
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestAIGatewayServeKeyAndVersionValidationErr(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	//nolint:gocritic // Owner role is needed for gateway key management.
	created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-quick-failures"})
	require.NoError(t, err)
	validKey := created.Key

	//nolint:gocritic // Owner role is needed for gateway key management.
	revoked, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-revoked"})
	require.NoError(t, err)
	require.NoError(t, client.DeleteAIGatewayKey(ctx, revoked.ID))

	tests := []struct {
		name       string
		key        string
		version    string
		wantStatus int
	}{
		{
			name:       "MissingKey",
			key:        "",
			version:    aibridgedproto.CurrentVersion.String(),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "InvalidKey",
			key:        "not-a-real-key",
			version:    aibridgedproto.CurrentVersion.String(),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "RevokedKey",
			key:        revoked.Key,
			version:    aibridgedproto.CurrentVersion.String(),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "IncompatibleVersion",
			key:        validKey,
			version:    "999.0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingVersion",
			key:        validKey,
			version:    "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, status := manualDialAIGatewayServe(t.Context(), t, client, tc.key, tc.version)
			require.Equal(t, tc.wantStatus, status)
		})
	}
}

func TestAIGatewayServeMissingEntitlement(t *testing.T) {
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

	// The production dialer must surface the upgrade failure as a
	// *codersdk.Error so the standalone gateway's connect loop can detect the
	// 403 and stop retrying instead of looping forever.
	_, err := dialAIGatewayServe(ctx, t, client, "any-key")
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func TestAIGatewayServeTrackKeyUsageClosesActiveSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(context.Context, *codersdk.Client, *entcoderd.API, codersdk.CreateAIGatewayKeyResponse) error
	}{
		{
			name: "DeletedKey",
			mutate: func(ctx context.Context, client *codersdk.Client, _ *entcoderd.API, created codersdk.CreateAIGatewayKeyResponse) error {
				//nolint:gocritic // Owner role is needed for gateway key management.
				return client.DeleteAIGatewayKey(ctx, created.ID)
			},
		},
		{
			name: "RevokedEntitlement",
			mutate: func(_ context.Context, _ *codersdk.Client, api *entcoderd.API, _ codersdk.CreateAIGatewayKeyResponse) error {
				api.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
					entitlements.Features[codersdk.FeatureAIBridge] = codersdk.Feature{
						Entitlement: codersdk.EntitlementNotEntitled,
						Enabled:     false,
					}
				})
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tick := make(chan time.Time, 1)
			opts := aibridgeOpts(t)
			opts.Options.NewTicker = func(time.Duration) (<-chan time.Time, func()) {
				return tick, func() {}
			}

			client, _, api, _ := coderdenttest.NewWithAPI(t, opts)
			ctx := testutil.Context(t, testutil.WaitLong)

			//nolint:gocritic // Owner role is needed for gateway key management.
			created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "serve-active"})
			require.NoError(t, err)

			dc, err := dialAIGatewayServe(ctx, t, client, created.Key)
			require.NoError(t, err)

			require.NoError(t, tt.mutate(ctx, client, api, created))

			tick <- time.Now() // trigger aiGatewayTrackKeyUsage.
			require.Eventually(t, func() bool {
				select {
				case <-dc.DRPCConn().Closed():
					return true
				default:
					return false
				}
			}, testutil.WaitShort, testutil.IntervalFast)
		})
	}
}
