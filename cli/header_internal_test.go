package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestCommandHeaderProvider(t *testing.T) {
	t.Parallel()

	t.Run("JWTRefresh", func(t *testing.T) {
		t.Parallel()

		t.Run("CacheExpiry", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			first := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Minute)))
			later := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Hour)))
			// Avoid nested quotes: cmd.exe does not unquote arguments like sh.
			command := fmt.Sprintf("echo Authorization=Bearer %s&&echo X-Token=%s&&echo X-Removed=old", first, later)
			provider := newTestHeaderProvider(clock, http.Header{"X-Static": {"unchanged"}}, command)

			headers, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "Bearer "+first, headers.Get("Authorization"))
			require.Equal(t, "unchanged", headers.Get("X-Static"))
			provider.command = fmt.Sprintf("echo Authorization=Bearer %s&&echo X-Value=updated", later)

			clock.Advance(time.Minute - jwtExpirationSkew - time.Second).MustWait(ctx)
			cached, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, headers, cached)
			cached["Authorization"][0] = "mutated"
			cached["X-Static"][0] = "mutated"
			isolated, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "Bearer "+first, isolated.Get("Authorization"))
			require.Equal(t, "unchanged", isolated.Get("X-Static"))
			require.Equal(t, headers, isolated)

			clock.Advance(time.Second).MustWait(ctx)
			refreshed, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"Bearer " + later}, refreshed.Values("Authorization"))
			require.Equal(t, "unchanged", refreshed.Get("X-Static"))
			require.Equal(t, "updated", refreshed.Get("X-Value"))
			require.Empty(t, refreshed.Get("X-Removed"))
			require.Empty(t, refreshed.Get("X-Token"))
		})

		t.Run("WithinClockSkew", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			shortLived := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(jwtExpirationSkew/2)))
			provider := newTestHeaderProvider(clock, nil, "echo Authorization=Bearer "+shortLived)
			headers, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "Bearer "+shortLived, headers.Get("Authorization"))

			replacement := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Hour)))
			provider.command = "echo Authorization=Bearer " + replacement
			// The current token is already within the skew window, so the next call
			// refreshes without advancing the clock.
			refreshed, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "Bearer "+replacement, refreshed.Get("Authorization"))

			provider.command = "echo X-Value=changed"
			cached, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, refreshed, cached)
		})
	})

	t.Run("StaticOutput", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		command := "echo X-Value=static&&echo X-Token=" + headerTestJWT(t, nil)
		staticToken := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(-time.Minute)))
		provider := newTestHeaderProvider(clock, http.Header{"X-Static-Token": {staticToken}}, command)
		headers, err := provider.Headers(ctx)
		require.NoError(t, err)
		require.Equal(t, "static", headers.Get("X-Value"))
		require.Equal(t, staticToken, headers.Get("X-Static-Token"))

		provider.command = "echo X-Value=changed"
		clock.Advance(24 * time.Hour).MustWait(ctx)
		cached, err := provider.Headers(ctx)
		require.NoError(t, err)
		require.Equal(t, headers, cached)
	})

	t.Run("CommandFailure", func(t *testing.T) {
		t.Parallel()

		t.Run("CommandError", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			command := "echo Authorization=Bearer " + headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Minute)))
			provider := newTestHeaderProvider(clock, nil, command)
			_, err := provider.Headers(ctx)
			require.NoError(t, err)
			clock.Advance(time.Minute).MustWait(ctx)
			provider.command = "exit 1"

			sent := false
			transport := &codersdk.HeaderTransport{
				Provider: provider,
				Transport: roundTripper(func(*http.Request) (*http.Response, error) {
					sent = true
					return httptest.NewRecorder().Result(), nil
				}),
			}
			req := httptest.NewRequest(http.MethodGet, provider.serverURL.String(), nil).WithContext(ctx)
			res, err := transport.RoundTrip(req)
			if res != nil {
				require.NoError(t, res.Body.Close())
			}
			require.ErrorContains(t, err, "run header command")
			require.NotContains(t, err.Error(), provider.command)
			require.Nil(t, res)
			require.False(t, sent, "a failed refresh must not send stale headers")

			provider.command = "echo X-Value=recovered"
			recovered, err := provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "recovered", recovered.Get("X-Value"))
			require.Empty(t, recovered.Get("Authorization"))
		})

		t.Run("CanceledRefresh", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			token := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Minute)))
			provider := newTestHeaderProvider(clock, nil, "echo Authorization=Bearer "+token)
			_, err := provider.Headers(ctx)
			require.NoError(t, err)
			clock.Advance(time.Minute).MustWait(ctx)

			canceled, cancel := context.WithCancel(ctx)
			cancel()
			headers, err := provider.Headers(canceled)
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, headers)

			provider.command = "echo X-Value=recovered"
			headers, err = provider.Headers(ctx)
			require.NoError(t, err)
			require.Equal(t, "recovered", headers.Get("X-Value"))
		})
	})
}

func newTestHeaderProvider(clock quartz.Clock, static http.Header, command string) *commandHeaderProvider {
	return &commandHeaderProvider{
		serverURL:     &url.URL{Scheme: "https", Host: "coder.example.com"},
		staticHeaders: static,
		command:       command,
		clock:         clock,
	}
}

func headerTestJWT(t *testing.T, expiry *jwt.NumericDate) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.HS512,
		Key:       []byte(strings.Repeat("k", 64)),
	}, nil)
	require.NoError(t, err)
	value, err := jwt.Signed(signer).Claims(jwt.Claims{Expiry: expiry}).Serialize()
	require.NoError(t, err)
	return value
}
