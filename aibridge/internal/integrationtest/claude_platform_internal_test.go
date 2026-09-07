package integrationtest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/awssig"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
)

const claudePlatformWorkspaceID = "wrkspc_config"

// claudePlatformCfg returns a Claude Platform for AWS config pointing at the
// given URL. The base URL override keeps both bridged and passthrough routes
// on the mock upstream while the signing scope stays region-based.
func claudePlatformCfg(url string, authMode config.ClaudePlatformAuthMode) *config.AWSClaudePlatform {
	cfg := &config.AWSClaudePlatform{
		AuthMode:    authMode,
		Region:      "us-west-2",
		WorkspaceID: claudePlatformWorkspaceID,
		BaseURL:     url,
	}
	if authMode == config.ClaudePlatformAuthModeIAM {
		cfg.AccessKey = "test-access-key"
		cfg.AccessKeySecret = "test-secret-key"
	}
	return cfg
}

// TestClaudePlatformIntegration covers Anthropic's AWS-hosted Messages API.
// Unlike Bedrock it speaks the native Messages wire format, so only routing and
// authentication differ from a direct Anthropic provider.
func TestClaudePlatformIntegration(t *testing.T) {
	t.Parallel()

	t.Run("invalid config", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		cpCfg := claudePlatformCfg("http://unused", config.ClaudePlatformAuthModeIAM)
		cpCfg.Region = ""

		_, err := provider.NewAnthropic(ctx, config.Anthropic{}, nil, cpCfg)
		require.ErrorContains(t, err, "region required")
	})

	// IAM mode signs the bridged request for the aws-external-anthropic
	// service and forwards the client's model unchanged.
	t.Run("iam/v1/messages", func(t *testing.T) {
		for _, streaming := range []bool{true, false} {
			t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
				upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

				wantModel := gjson.GetBytes(fix.Request(), "model").String()
				require.NotEmpty(t, wantModel)

				cpCfg := claudePlatformCfg(upstream.URL, config.ClaudePlatformAuthModeIAM)
				// No key pool: IAM mode authenticates by signing.
				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
					withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, cpCfg)),
				)

				reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
				require.NoError(t, err)
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				received := upstream.ReceivedRequests()
				require.Len(t, received, 1)

				// Native passthrough: standard Messages path, model unchanged.
				require.Equal(t, "/v1/messages", received[0].Path)
				require.Equal(t, wantModel, gjson.GetBytes(received[0].Body, "model").String(),
					"model should be forwarded unchanged")

				authHeader := received[0].Header.Get("Authorization")
				require.True(t, strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", authHeader)
				require.Contains(t, authHeader, "/aws-external-anthropic/aws4_request",
					"signature must be scoped to the aws-external-anthropic service")
				require.Contains(t, authHeader, "/us-west-2/",
					"signature must be scoped to the configured region, not the base URL host")

				require.Equal(t, claudePlatformWorkspaceID, received[0].Header.Get(intercept.HeaderAnthropicWorkspaceID))
				require.Empty(t, received[0].Header.Get(intercept.AuthHeaderXAPIKey),
					"IAM mode must not send an API key")

				// PRM attribution is a Bedrock-specific revenue agreement.
				require.NotContains(t, received[0].Header.Get("User-Agent"), awssig.PRMUserAgent,
					"Claude Platform must not send Bedrock PRM attribution")

				interceptions := bridgeServer.Recorder.RecordedInterceptions()
				require.Len(t, interceptions, 1)
				require.Equal(t, wantModel, interceptions[0].Model)
				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})

	// api_key mode authenticates with a workspace key from the provider's key
	// pool, exactly like a direct Anthropic provider, and signs nothing.
	t.Run("api_key/v1/messages", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
		upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

		cpCfg := claudePlatformCfg(upstream.URL, config.ClaudePlatformAuthModeAPIKey)
		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
			withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, anthropicCfg(upstream.URL, apiKey), cpCfg)),
		)

		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, fix.Request())
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		require.Equal(t, "/v1/messages", received[0].Path)
		require.Equal(t, apiKey, received[0].Header.Get(intercept.AuthHeaderXAPIKey))
		require.Empty(t, received[0].Header.Get("Authorization"), "api_key mode must not sign")
		require.Equal(t, claudePlatformWorkspaceID, received[0].Header.Get(intercept.HeaderAnthropicWorkspaceID))

		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})

	// A client-supplied key keeps precedence over IAM signing so BYOK keeps
	// working when the deployment is configured for signing.
	t.Run("byok wins over iam signing", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
		upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

		cpCfg := claudePlatformCfg(upstream.URL, config.ClaudePlatformAuthModeIAM)
		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
			withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, cpCfg)),
		)

		// The client also tries to pick its own workspace, which must be
		// overridden by provider configuration.
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, fix.Request(), http.Header{
			intercept.AuthHeaderXAPIKey:          {"user-key"},
			intercept.HeaderAnthropicWorkspaceID: {"wrkspc_from_client"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		require.Equal(t, "user-key", received[0].Header.Get(intercept.AuthHeaderXAPIKey),
			"BYOK key must be forwarded")
		require.Equal(t, claudePlatformWorkspaceID, received[0].Header.Get(intercept.HeaderAnthropicWorkspaceID),
			"provider configuration owns the workspace ID")

		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})

	// Passthrough routes bypass the SDK, so the provider signs them itself.
	t.Run("iam passthrough", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		upstream := testutil.NewMockUpstream(ctx, t, testutil.UpstreamResponse{
			Blocking: []byte(`{"data":[]}`),
		})

		cpCfg := claudePlatformCfg(upstream.URL, config.ClaudePlatformAuthModeIAM)
		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
			withCustomProvider(aibridgetest.NewClaudePlatformProvider(t, config.Anthropic{}, cpCfg)),
		)

		resp, err := bridgeServer.makeRequest(t, http.MethodGet, "/anthropic/v1/models", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		require.Equal(t, "/v1/models", received[0].Path)

		authHeader := received[0].Header.Get("Authorization")
		require.True(t, strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", authHeader)
		require.Contains(t, authHeader, "/aws-external-anthropic/aws4_request")
		require.Equal(t, claudePlatformWorkspaceID, received[0].Header.Get(intercept.HeaderAnthropicWorkspaceID))
	})
}
