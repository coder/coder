package promoauth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/testutil"
)

func TestInstrument(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	idp := oidctest.NewFakeIDP(t, oidctest.WithServing())
	reg := prometheus.NewRegistry()
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(promhelp.RegistryDump(reg))
		}
	})

	const id = "test"
	labels := prometheus.Labels{
		"name":        id,
		"status_code": "200",
	}
	const metricname = "coderd_oauth2_external_requests_total"
	count := func(source string) int {
		labels["source"] = source
		return promhelp.CounterValue(t, reg, "coderd_oauth2_external_requests_total", labels)
	}

	factory := promoauth.NewFactory(reg)

	cfg := externalauth.Config{
		InstrumentedOAuth2Config: factory.New(id, idp.OIDCConfig(t, []string{})),
		ID:                       "test",
		ValidateURL:              must[*url.URL](t)(idp.IssuerURL().Parse("/oauth2/userinfo")).String(),
	}

	// 0 Requests before we start
	require.Nil(t, promhelp.MetricValue(t, reg, metricname, labels), "no metrics at start")

	noClientCtx := ctx
	// This should never be done, but promoauth should not break the default client
	// even if this happens. So intentionally do this to verify nothing breaks.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, http.DefaultClient)
	// Exchange should trigger a request
	code := idp.CreateAuthCode(t, "foo")
	_, err := cfg.Exchange(ctx, code)
	require.NoError(t, err)
	require.Equal(t, count("Exchange"), 1)

	// Do an exchange without a default http client as well to verify original
	// transport is not broken.
	code = idp.CreateAuthCode(t, "bar")
	token, err := cfg.Exchange(noClientCtx, code)
	require.NoError(t, err)
	require.Equal(t, count("Exchange"), 2)

	// Force a refresh
	token.Expiry = time.Now().Add(time.Hour * -1)
	src := cfg.TokenSource(ctx, token)
	refreshed, err := src.Token()
	require.NoError(t, err)
	require.NotEqual(t, token.AccessToken, refreshed.AccessToken, "token refreshed")
	require.Equal(t, count("TokenSource"), 1)

	// Try a validate
	valid, _, err := cfg.ValidateToken(ctx, refreshed)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, count("ValidateToken"), 1)

	// Verify the default client was not broken. This check is added because we
	// extend the http.DefaultTransport. If a `.Clone()` is not done, this can be
	// mis-used. It is cheap to run this quick check.
	snapshot := promhelp.RegistryDump(reg)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		must[*url.URL](t)(idp.IssuerURL().Parse("/.well-known/openid-configuration")).String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.NoError(t, promhelp.Compare(reg, snapshot), "http default client corrupted")
}

func TestGithubRateLimits(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cases := []struct {
		Name            string
		NoHeaders       bool
		Omit            []string
		ExpectNoMetrics bool
		Limit           int
		Remaining       int
		Used            int
		Reset           time.Time

		at time.Time
	}{
		{
			Name:            "NoHeaders",
			NoHeaders:       true,
			ExpectNoMetrics: true,
		},
		{
			Name:            "ZeroHeaders",
			ExpectNoMetrics: true,
		},
		{
			Name:      "OverLimit",
			Limit:     100,
			Remaining: 0,
			Used:      500,
			Reset:     now.Add(time.Hour),
			at:        now,
		},
		{
			Name:      "UnderLimit",
			Limit:     100,
			Remaining: 0,
			Used:      500,
			Reset:     now.Add(time.Hour),
			at:        now,
		},
		{
			Name:            "Partial",
			Omit:            []string{"x-ratelimit-remaining"},
			ExpectNoMetrics: true,
			Limit:           100,
			Remaining:       0,
			Used:            500,
			Reset:           now.Add(time.Hour),
			at:              now,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			idp := oidctest.NewFakeIDP(t, oidctest.WithMiddlewares(
				func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
						if !c.NoHeaders {
							rw.Header().Set("x-ratelimit-limit", fmt.Sprintf("%d", c.Limit))
							rw.Header().Set("x-ratelimit-remaining", fmt.Sprintf("%d", c.Remaining))
							rw.Header().Set("x-ratelimit-used", fmt.Sprintf("%d", c.Used))
							rw.Header().Set("x-ratelimit-resource", "core")
							rw.Header().Set("x-ratelimit-reset", fmt.Sprintf("%d", c.Reset.Unix()))
							for _, omit := range c.Omit {
								rw.Header().Del(omit)
							}
						}

						// Ignore the well known, required for setup
						if !strings.Contains(r.URL.String(), ".well-known") {
							// GitHub rate limits only are instrumented for unauthenticated calls. So emulate
							// that here. We cannot actually use invalid credentials because the fake IDP
							// will throw a test error, as it only expects things to work. And we want to use
							// a real idp to emulate the right http calls to be instrumented.
							rw.WriteHeader(http.StatusUnauthorized)
							return
						}
						next.ServeHTTP(rw, r)
					})
				}))

			factory := promoauth.NewFactory(reg)
			if !c.at.IsZero() {
				factory.Now = func() time.Time {
					return c.at
				}
			}

			cfg := factory.NewGithub("test", idp.OIDCConfig(t, []string{}))

			// Do a single oauth2 call
			ctx := testutil.Context(t, testutil.WaitShort)
			ctx = context.WithValue(ctx, oauth2.HTTPClient, idp.HTTPClient(nil))
			_, err := cfg.Exchange(ctx, "invalid")
			require.Error(t, err, "expected unauthorized exchange")

			// Verify
			labels := prometheus.Labels{
				"name":     "test",
				"resource": "core-unauthorized",
			}
			pass := true
			if !c.ExpectNoMetrics {
				pass = pass && assert.Equal(t, promhelp.GaugeValue(t, reg, "coderd_oauth2_external_requests_rate_limit_total", labels), c.Limit, "limit")
				pass = pass && assert.Equal(t, promhelp.GaugeValue(t, reg, "coderd_oauth2_external_requests_rate_limit_remaining", labels), c.Remaining, "remaining")
				pass = pass && assert.Equal(t, promhelp.GaugeValue(t, reg, "coderd_oauth2_external_requests_rate_limit_used", labels), c.Used, "used")
				if !c.at.IsZero() {
					until := c.Reset.Sub(c.at)
					// Float accuracy is not great, so we allow a delta of 2
					pass = pass && assert.InDelta(t, promhelp.GaugeValue(t, reg, "coderd_oauth2_external_requests_rate_limit_reset_in_seconds", labels), int(until.Seconds()), 2, "reset in")
				}
			} else {
				pass = pass && assert.Nil(t, promhelp.MetricValue(t, reg, "coderd_oauth2_external_requests_rate_limit_total", labels), "not exists")
			}

			// Helpful debugging
			if !pass {
				t.Log(promhelp.RegistryDump(reg))
			}
		})
	}
}

func must[V any](t *testing.T) func(v V, err error) V {
	return func(v V, err error) V {
		t.Helper()
		require.NoError(t, err)
		return v
	}
}
