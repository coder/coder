package promoauth_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ptestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/testutil"
)

func TestMaintainDefault(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	idp := oidctest.NewFakeIDP(t, oidctest.WithServing())
	reg := prometheus.NewRegistry()
	count := func() int {
		return ptestutil.CollectAndCount(reg, "coderd_oauth2_external_requests_total")
	}

	factory := promoauth.NewFactory(reg)
	cfg := factory.New("test", idp.OAuthConfig())

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

	// Verify the default client was not broken. This check is added because we
	// extend the http.DefaultTransport. If a `.Clone()` is not done, this can be
	// mis-used. It is cheap to run this quick check.
	_, err = http.DefaultClient.Get("https://coder.com")
	require.NoError(t, err)
	require.Equal(t, count(), 2)
}
