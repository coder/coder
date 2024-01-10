package promoauth_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ptestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/testutil"
)

func TestInstrument(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	idp := oidctest.NewFakeIDP(t, oidctest.WithServing())
	reg := prometheus.NewRegistry()
	count := func() int {
		return ptestutil.CollectAndCount(reg, "coderd_oauth2_external_requests_total")
	}

	factory := promoauth.NewFactory(reg)
	const id = "test"
	cfg := externalauth.Config{
		InstrumentedOAuth2Config: factory.New(id, idp.OIDCConfig(t, []string{})),
		ID:                       "test",
		ValidateURL:              must[*url.URL](t)(idp.IssuerURL().Parse("/oauth2/userinfo")).String(),
	}

	// 0 Requests before we start
	require.Equal(t, count(), 0)

	// Exchange should trigger a request
	code := idp.CreateAuthCode(t, "foo")
	token, err := cfg.Exchange(ctx, code)
	require.NoError(t, err)
	require.Equal(t, count(), 1)

	// Force a refresh
	token.Expiry = time.Now().Add(time.Hour * -1)
	src := cfg.TokenSource(ctx, token)
	refreshed, err := src.Token()
	require.NoError(t, err)
	require.NotEqual(t, token.AccessToken, refreshed.AccessToken, "token refreshed")
	require.Equal(t, count(), 2)

	// Try a validate
	valid, _, err := cfg.ValidateToken(ctx, refreshed.AccessToken)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, count(), 3)

	// Verify the default client was not broken. This check is added because we
	// extend the http.DefaultTransport. If a `.Clone()` is not done, this can be
	// mis-used. It is cheap to run this quick check.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		must[*url.URL](t)(idp.IssuerURL().Parse("/.well-known/openid-configuration")).String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, count(), 3)
}

func must[V any](t *testing.T) func(v V, err error) V {
	return func(v V, err error) V {
		t.Helper()
		require.NoError(t, err)
		return v
	}
}
