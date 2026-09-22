package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

func TestCommandHeaderProviderJWTRefresh(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	provider, outputPath := newTestHeaderProvider(ctx, t, clock)
	first := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Minute)))
	later := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Hour)))
	initial := "Authorization=Bearer " + first + "\nX-Token=" + later + "\nX-Removed=old\n"
	require.NoError(t, os.WriteFile(outputPath, []byte(initial), 0o600))

	headers, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer "+first, headers.Get("Authorization"))
	require.Equal(t, "unchanged", headers.Get("X-Static"))
	updated := "Authorization=Bearer " + later + "\nX-Value=updated\n"
	require.NoError(t, os.WriteFile(outputPath, []byte(updated), 0o600))

	clock.Advance(time.Minute - jwtExpirationSkew - time.Second).MustWait(ctx)
	cached, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, headers, cached)

	clock.Advance(time.Second).MustWait(ctx)
	refreshed, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer " + later}, refreshed.Values("Authorization"))
	require.Equal(t, "unchanged", refreshed.Get("X-Static"))
	require.Equal(t, "updated", refreshed.Get("X-Value"))
	require.Empty(t, refreshed.Get("X-Removed"))
	require.Empty(t, refreshed.Get("X-Token"))
}

func TestCommandHeaderProviderStaticOutput(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	provider, outputPath := newTestHeaderProvider(ctx, t, clock)
	initial := "X-Value=static\nX-Token=" + headerTestJWT(t, nil) + "\n"
	require.NoError(t, os.WriteFile(outputPath, []byte(initial), 0o600))
	headers, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "static", headers.Get("X-Value"))

	require.NoError(t, os.WriteFile(outputPath, []byte("X-Value=changed\n"), 0o600))
	clock.Advance(24 * time.Hour).MustWait(ctx)
	cached, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, headers, cached)
}

func TestCommandHeaderProviderCommandFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	provider, outputPath := newTestHeaderProvider(ctx, t, clock)
	initial := "Authorization=Bearer " + headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Minute))) + "\n"
	require.NoError(t, os.WriteFile(outputPath, []byte(initial), 0o600))
	_, err := provider.Headers(ctx)
	require.NoError(t, err)
	clock.Advance(time.Minute).MustWait(ctx)
	require.NoError(t, os.Remove(outputPath))

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

	require.NoError(t, os.WriteFile(outputPath, []byte("X-Value=recovered\n"), 0o600))
	recovered, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "recovered", recovered.Get("X-Value"))
	require.Empty(t, recovered.Get("Authorization"))
}

func TestCommandHeaderProviderWithinClockSkew(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	provider, outputPath := newTestHeaderProvider(ctx, t, clock)
	shortLived := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(jwtExpirationSkew/2)))
	require.NoError(t, os.WriteFile(outputPath, []byte("Authorization=Bearer "+shortLived+"\n"), 0o600))
	headers, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer "+shortLived, headers.Get("Authorization"))

	replacement := headerTestJWT(t, jwt.NewNumericDate(clock.Now().Add(time.Hour)))
	require.NoError(t, os.WriteFile(outputPath, []byte("Authorization=Bearer "+replacement+"\n"), 0o600))
	// The current token is already within the skew window, so the next call
	// refreshes without advancing the clock.
	refreshed, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer "+replacement, refreshed.Get("Authorization"))

	require.NoError(t, os.WriteFile(outputPath, []byte("X-Value=changed\n"), 0o600))
	cached, err := provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, refreshed, cached)
}

func newTestHeaderProvider(ctx context.Context, t *testing.T, clock quartz.Clock) (*commandHeaderProvider, string) {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "headers")
	command := fmt.Sprintf("cat %q", outputPath)
	if runtime.GOOS == "windows" {
		command = `type "` + outputPath + `"`
	}
	return &commandHeaderProvider{
		ctx:       ctx,
		serverURL: &url.URL{Scheme: "https", Host: "coder.example.com"},
		static:    []string{"X-Static=unchanged"},
		command:   command,
		clock:     clock,
	}, outputPath
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
