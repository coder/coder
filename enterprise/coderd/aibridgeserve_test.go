package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/aibridged"
	aibridgedproto "github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	entcoderd "github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

type versionOverridingRoundTripper struct {
	baseTransport      http.RoundTripper
	overrideAPIVersion string
}

func (f versionOverridingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	query := req.URL.Query()
	query.Del(aibridgedproto.VersionQueryParam)
	if f.overrideAPIVersion != "" {
		query.Set(aibridgedproto.VersionQueryParam, f.overrideAPIVersion)
	}
	req.URL.RawQuery = query.Encode()
	return f.baseTransport.RoundTrip(req)
}

func dialAIGatewayServe(ctx context.Context, t *testing.T, client *codersdk.Client, key string) (aibridged.DRPCClient, error) {
	return dialAIGatewayServeWithVersion(ctx, t, client, key, nil)
}

func dialAIGatewayServeWithVersion(ctx context.Context, t *testing.T, client *codersdk.Client, key string, version *string) (aibridged.DRPCClient, error) {
	t.Helper()

	transport := client.HTTPClient.Transport
	if version != nil {
		transport = versionOverridingRoundTripper{
			baseTransport:      transport,
			overrideAPIVersion: *version,
		}
	}

	dc, err := aibridged.NewWebsocketDialer(client.URL, transport, key)(ctx)
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
		name             string
		key              string
		version          string
		wantStatus       int
		wantMessage      string
		forbidErrMessage string
	}{
		{
			name:             "MissingKey",
			key:              "",
			version:          aibridgedproto.CurrentVersion.String(),
			wantStatus:       http.StatusUnauthorized,
			wantMessage:      "AI Gateway key required.",
			forbidErrMessage: "Try logging in",
		},
		{
			name:             "InvalidKey",
			key:              "not-a-real-key",
			version:          aibridgedproto.CurrentVersion.String(),
			wantStatus:       http.StatusUnauthorized,
			wantMessage:      "AI Gateway key invalid.",
			forbidErrMessage: "Try logging in",
		},
		{
			name:             "RevokedKey",
			key:              revoked.Key,
			version:          aibridgedproto.CurrentVersion.String(),
			wantStatus:       http.StatusUnauthorized,
			wantMessage:      "AI Gateway key invalid.",
			forbidErrMessage: "Try logging in",
		},
		{
			name:        "IncompatibleVersion",
			key:         validKey,
			version:     "999.0",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Incompatible or unparsable version",
		},
		{
			name:        "MissingVersion",
			key:         validKey,
			version:     "",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Incompatible or unparsable version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := dialAIGatewayServeWithVersion(t.Context(), t, client, tc.key, &tc.version)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, tc.wantStatus, sdkErr.StatusCode())
			require.Contains(t, sdkErr.Error(), tc.wantMessage)
			if tc.forbidErrMessage != "" {
				require.NotContains(t, sdkErr.Error(), tc.forbidErrMessage)
			}
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

	t.Run("DeletedKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		session := setupActiveAIGatewayServeSession(ctx, t)

		//nolint:gocritic // Owner role is needed for gateway key management.
		require.NoError(t, session.client.DeleteAIGatewayKey(ctx, session.created.ID))
		requireAIGatewayServeSessionClosed(t, session)
	})

	t.Run("RevokedEntitlement", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		session := setupActiveAIGatewayServeSession(ctx, t)

		licenses, err := session.client.Licenses(ctx)
		require.NoError(t, err)
		for _, license := range licenses {
			require.NoError(t, session.client.DeleteLicense(ctx, license.ID))
		}
		require.Eventually(t, func() bool {
			return !session.api.Entitlements.Enabled(codersdk.FeatureAIBridge)
		}, testutil.WaitShort, testutil.IntervalFast)

		requireAIGatewayServeSessionClosed(t, session)
	})
}

type activeAIGatewayServeSession struct {
	client  *codersdk.Client
	api     *entcoderd.API
	created codersdk.CreateAIGatewayKeyResponse
	tick    chan time.Time
	dc      aibridged.DRPCClient
}

func setupActiveAIGatewayServeSession(ctx context.Context, t *testing.T) activeAIGatewayServeSession {
	t.Helper()

	tick := make(chan time.Time, 1)
	opts := aibridgeOpts(t)
	opts.Options.NewTicker = func(time.Duration) (<-chan time.Time, func()) {
		return tick, func() {}
	}

	client, _, api, _ := coderdenttest.NewWithAPI(t, opts)

	//nolint:gocritic // Owner role is needed for gateway key management.
	created, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "key-name"})
	require.NoError(t, err)

	dc, err := dialAIGatewayServe(ctx, t, client, created.Key)
	require.NoError(t, err)

	return activeAIGatewayServeSession{
		client:  client,
		api:     api,
		created: created,
		tick:    tick,
		dc:      dc,
	}
}

func requireAIGatewayServeSessionClosed(t *testing.T, s activeAIGatewayServeSession) {
	t.Helper()

	s.tick <- time.Now() // trigger gateway key / license check.
	require.Eventually(t, func() bool {
		select {
		case <-s.dc.DRPCConn().Closed():
			return true
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast)
}
