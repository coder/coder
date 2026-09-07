package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

func claudePlatformIAMCfg() *config.AWSClaudePlatform {
	return &config.AWSClaudePlatform{
		AuthMode:        config.ClaudePlatformAuthModeIAM,
		Region:          "us-west-2",
		WorkspaceID:     "wrkspc_config",
		AccessKey:       "test-access-key",
		AccessKeySecret: "test-secret-key",
	}
}

func claudePlatformAPIKeyCfg() *config.AWSClaudePlatform {
	return &config.AWSClaudePlatform{
		AuthMode:    config.ClaudePlatformAuthModeAPIKey,
		Region:      "us-west-2",
		WorkspaceID: "wrkspc_config",
	}
}

func newTestClaudePlatform(t testing.TB, cfg config.Anthropic, cpCfg *config.AWSClaudePlatform) *Anthropic {
	t.Helper()
	p, err := NewAnthropic(context.Background(), cfg, nil, cpCfg)
	require.NoError(t, err)
	return p
}

func TestNewAnthropic_ClaudePlatform(t *testing.T) {
	t.Parallel()

	t.Run("mutually exclusive with bedrock", func(t *testing.T) {
		t.Parallel()

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, &config.AWSBedrock{
			Region:         "us-west-2",
			Model:          "beddel",
			SmallFastModel: "modrock",
		}, claudePlatformIAMCfg())
		require.ErrorContains(t, err, "mutually exclusive")
	})

	t.Run("invalid config rejected", func(t *testing.T) {
		t.Parallel()

		cpCfg := claudePlatformIAMCfg()
		cpCfg.WorkspaceID = ""

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, nil, cpCfg)
		require.ErrorContains(t, err, "workspace id required")
	})

	t.Run("iam mode resolves credentials", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		require.NotNil(t, p.auth.ClaudePlatform)
		require.Nil(t, p.auth.Bedrock)
		require.NotNil(t, p.auth.ClaudePlatform.Creds)
	})

	t.Run("api key mode resolves no credentials", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformAPIKeyCfg())
		require.NotNil(t, p.auth.ClaudePlatform)
		// The workspace key comes from the key pool, so no AWS identity is
		// resolved and nothing is signed.
		require.Nil(t, p.auth.ClaudePlatform.Creds)
	})
}

func TestAnthropic_ClaudePlatformBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("regional default", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		require.Equal(t, "https://aws-external-anthropic.us-west-2.api.aws", p.BaseURL())
	})

	t.Run("override wins over anthropic base url", func(t *testing.T) {
		t.Parallel()

		cpCfg := claudePlatformIAMCfg()
		cpCfg.BaseURL = "https://proxy.internal"

		// The Claude Platform base URL is authoritative: the generic Anthropic
		// base URL is not consulted, so both bridged and passthrough routes
		// reach the same host.
		p := newTestClaudePlatform(t, config.Anthropic{BaseURL: "https://api.anthropic.com"}, cpCfg)
		require.Equal(t, "https://proxy.internal", p.BaseURL())
	})
}

func TestAnthropic_ClaudePlatformResolveCredential(t *testing.T) {
	t.Parallel()

	t.Run("iam mode signs when no key is present", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())

		cred, err := p.resolveCredential(httptest.NewRequest(http.MethodPost, routeMessages, nil))
		require.NoError(t, err)
		sig, ok := cred.(intercept.AWSSigV4)
		require.True(t, ok, "expected AWS SigV4 credential, got %T", cred)
		require.Equal(t, "test-access-key", sig.AccessKey)
	})

	t.Run("byok takes precedence over signing", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())

		req := httptest.NewRequest(http.MethodPost, routeMessages, nil)
		req.Header.Set(intercept.AuthHeaderXAPIKey, "user-key")

		cred, err := p.resolveCredential(req)
		require.NoError(t, err)
		require.Equal(t, intercept.BYOK{Secret: "user-key", Header: intercept.AuthHeaderXAPIKey}, cred)
	})

	t.Run("api key mode uses the key pool", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{
			KeyPool: testutil.SingleKeyPool(config.ProviderAnthropic, "workspace-key"),
		}, claudePlatformAPIKeyCfg())

		cred, err := p.resolveCredential(httptest.NewRequest(http.MethodPost, routeMessages, nil))
		require.NoError(t, err)
		require.IsType(t, &intercept.CentralizedPool{}, cred)
	})

	t.Run("api key mode without a pool has no credential", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformAPIKeyCfg())

		_, err := p.resolveCredential(httptest.NewRequest(http.MethodPost, routeMessages, nil))
		require.ErrorIs(t, err, ErrNoCredential)
	})
}

// captureTransport records the last request it saw and returns an empty 200.
type captureTransport struct {
	req *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestAnthropic_WrapPassthroughTransport(t *testing.T) {
	t.Parallel()

	t.Run("not wrapped for plain anthropic", func(t *testing.T) {
		t.Parallel()

		p := newTestAnthropic(t, config.Anthropic{}, nil)
		inner := &captureTransport{}
		require.Same(t, http.RoundTripper(inner), p.WrapPassthroughTransport(inner))
	})

	t.Run("not wrapped for bedrock", func(t *testing.T) {
		t.Parallel()

		p := newTestAnthropic(t, config.Anthropic{}, &config.AWSBedrock{
			Region:         "us-west-2",
			Model:          "beddel",
			SmallFastModel: "modrock",
		})
		inner := &captureTransport{}
		require.Same(t, http.RoundTripper(inner), p.WrapPassthroughTransport(inner))
	})

	t.Run("iam mode signs and sets the workspace header", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		inner := &captureTransport{}

		req := httptest.NewRequest(http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", nil)
		req.Header.Set(intercept.HeaderAnthropicWorkspaceID, "wrkspc_from_client")

		resp, err := p.WrapPassthroughTransport(inner).RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.NotNil(t, inner.req)
		require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID),
			"provider configuration must own the workspace ID")

		auth := inner.req.Header.Get(intercept.AuthHeaderAuthorization)
		require.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256"), "missing SigV4 auth: %q", auth)
		require.Contains(t, auth, "/aws-external-anthropic/aws4_request",
			"signature must be scoped to the aws-external-anthropic service")
	})

	t.Run("byok is forwarded unsigned", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformIAMCfg())
		inner := &captureTransport{}

		req := httptest.NewRequest(http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", nil)
		req.Header.Set(intercept.AuthHeaderXAPIKey, "user-key")

		resp, err := p.WrapPassthroughTransport(inner).RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.NotNil(t, inner.req)
		require.Equal(t, "user-key", inner.req.Header.Get(intercept.AuthHeaderXAPIKey))
		require.Empty(t, inner.req.Header.Get(intercept.AuthHeaderAuthorization),
			"a request that already carries a credential must not also be signed")
		require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID))
	})

	t.Run("api key mode only sets the workspace header", func(t *testing.T) {
		t.Parallel()

		p := newTestClaudePlatform(t, config.Anthropic{}, claudePlatformAPIKeyCfg())
		inner := &captureTransport{}

		req := httptest.NewRequest(http.MethodGet, "https://aws-external-anthropic.us-west-2.api.aws/v1/models", nil)

		resp, err := p.WrapPassthroughTransport(inner).RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.NotNil(t, inner.req)
		require.Equal(t, "wrkspc_config", inner.req.Header.Get(intercept.HeaderAnthropicWorkspaceID))
		require.Empty(t, inner.req.Header.Get(intercept.AuthHeaderAuthorization))
	})
}
