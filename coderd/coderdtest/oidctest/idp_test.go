package oidctest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
)

// TestFakeIDPBasicFlow tests the basic flow of the fake IDP.
// It is done all in memory with no actual network requests.
// nolint:bodyclose
func TestFakeIDPBasicFlow(t *testing.T) {
	t.Parallel()

	fake := oidctest.NewFakeIDP(t,
		oidctest.WithLogging(t, nil),
	)

	var handler http.Handler
	srv := httptest.NewServer(http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	})))
	defer srv.Close()

	cfg := fake.OIDCConfig(t, nil)
	cli := fake.HTTPClient(nil)
	ctx := oidc.ClientContext(context.Background(), cli)

	const expectedState = "random-state"
	var token *oauth2.Token
	// This is the Coder callback using an actual network request.
	fake.SetCoderdCallbackHandler(func(w http.ResponseWriter, r *http.Request) {
		// Emulate OIDC flow
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		assert.Equal(t, expectedState, state, "state mismatch")

		oauthToken, err := cfg.Exchange(ctx, code)
		if assert.NoError(t, err, "failed to exchange code") {
			assert.NotEmpty(t, oauthToken.AccessToken, "access token is empty")
			assert.NotEmpty(t, oauthToken.RefreshToken, "refresh token is empty")
		}
		token = oauthToken
	})

	//nolint:bodyclose
	resp := fake.OIDCCallback(t, expectedState, jwt.MapClaims{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Test the user info
	_, err := cfg.Provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	require.NoError(t, err)

	// Now test it can refresh
	refreshed, err := cfg.TokenSource(ctx, &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(time.Minute * -1),
	}).Token()
	require.NoError(t, err, "failed to refresh token")
	require.NotEmpty(t, refreshed.AccessToken, "access token is empty on refresh")
}
