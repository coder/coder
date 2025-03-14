package oidctest_test

import (
	"fmt"
	"errors"
	"context"
	"crypto"
	"net/http"
	"testing"

	"time"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"

	"github.com/coder/coder/v2/testutil"
)
// TestFakeIDPBasicFlow tests the basic flow of the fake IDP.
// It is done all in memory with no actual network requests.
// nolint:bodyclose

func TestFakeIDPBasicFlow(t *testing.T) {
	t.Parallel()
	fake := oidctest.NewFakeIDP(t,
		oidctest.WithLogging(t, nil),
	)
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
// TestIDPIssuerMismatch emulates a situation where the IDP issuer url does

// not match the one in the well-known config and claims.
// This can happen in some edge cases and in some azure configurations.
//
// This test just makes sure a fake IDP can set up this scenario.
func TestIDPIssuerMismatch(t *testing.T) {
	t.Parallel()
	const proxyURL = "https://proxy.com"
	const primaryURL = "https://primary.com"
	fake := oidctest.NewFakeIDP(t,
		oidctest.WithIssuer(proxyURL),

		oidctest.WithDefaultIDClaims(jwt.MapClaims{
			"iss": primaryURL,
		}),
		oidctest.WithHookWellKnown(func(r *http.Request, j *oidctest.ProviderJSON) error {
			// host should be proxy.com, but we return the primaryURL
			if r.Host != "proxy.com" {
				return fmt.Errorf("unexpected host: %s", r.Host)
			}

			j.Issuer = primaryURL
			return nil
		}),

		oidctest.WithLogging(t, nil),
	)
	ctx := testutil.Context(t, testutil.WaitMedium)
	// Do not use real network requests
	cli := fake.HTTPClient(nil)
	ctx = oidc.ClientContext(ctx, cli)
	// Allow the issuer mismatch
	verifierContext := oidc.InsecureIssuerURLContext(ctx, "this field does not matter")
	p, err := oidc.NewProvider(verifierContext, "https://proxy.com")
	require.NoError(t, err, "failed to create OIDC provider")
	oauthConfig := fake.OauthConfig(t, nil)
	cfg := &coderd.OIDCConfig{
		OAuth2Config: oauthConfig,
		Provider:     p,
		Verifier: oidc.NewVerifier(fake.WellknownConfig().Issuer, &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{fake.PublicKey()},

		}, &oidc.Config{
			SkipIssuerCheck: true,
			ClientID:        oauthConfig.ClientID,
			SupportedSigningAlgs: []string{
				"RS256",

			},
		}),
		UsernameField: "preferred_username",
		EmailField:    "email",
		AuthURLParams: map[string]string{"access_type": "offline"},

	}
	const expectedState = "random-state"
	var token *oauth2.Token
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
	resp := fake.OIDCCallback(t, expectedState, nil) // Use default claims
	require.Equal(t, http.StatusOK, resp.StatusCode)

	idToken, err := cfg.Verifier.Verify(ctx, token.Extra("id_token").(string))
	require.NoError(t, err)
	require.Equal(t, primaryURL, idToken.Issuer)

}
