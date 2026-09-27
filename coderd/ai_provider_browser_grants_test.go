package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// All key material below is throwaway and invalid. These tests never touch
// live credentials, the keychain, or a real subscription provider.

// scriptedBrowserExchanger serves one scripted token endpoint per test.
type scriptedBrowserExchanger struct {
	exchangeGrant coderd.AIDeviceTokenGrant
}

func (f *scriptedBrowserExchanger) ExchangeAuthorizationCode(_ context.Context, _, _ string) (coderd.AIDeviceTokenGrant, error) {
	return f.exchangeGrant, nil
}

func useScriptedBrowserFlow(api *coderd.API, fake *scriptedBrowserExchanger) {
	api.AIBrowserGrants.ClientFactory(func(_ database.AIProviderType, _ string) coderd.AIBrowserGrantExchanger {
		return fake
	})
}

// browserGrantState pulls the state back out of the authorize URL, the
// same round-trip a real provider redirect performs.
func browserGrantState(t testing.TB, authorizeURL string) string {
	t.Helper()
	parsed, err := url.Parse(authorizeURL)
	require.NoError(t, err)
	require.Equal(t, "auth.openai.com", parsed.Host)
	require.Equal(t, "/oauth/authorize", parsed.Path)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state, "authorize URL must carry state")
	return state
}

func TestUserAIBrowserGrants(t *testing.T) {
	t.Parallel()

	t.Run("AuthorizeThenPersistThroughServerPath", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		provider := createChatGPTProvider(t, adminClient)
		useScriptedBrowserFlow(api, &scriptedBrowserExchanger{
			exchangeGrant: scriptedTokenGrant(t, "acct-browser-outer"),
		})

		configs, err := memberClient.ListUserAIProviderKeyConfigs(ctx, "me")
		require.NoError(t, err)
		cfg := findKeyConfig(configs, provider.ID)
		require.NotNil(t, cfg)
		require.True(t, cfg.BrowserFlowSupported, "chatgpt provider must advertise the browser door")
		require.True(t, cfg.DeviceFlowSupported, "browser door lands next to the device door")
		require.False(t, cfg.HasUserAPIKey)

		grant, err := memberClient.InitiateUserAIBrowserGrant(ctx, "me", provider.ID)
		require.NoError(t, err)
		require.NotEmpty(t, grant.AuthorizeURL)
		require.NotContains(t, grant.AuthorizeURL, "code_verifier=", "verifier stays server-side")
		require.GreaterOrEqual(t, grant.ExpiresIn, 890)
		require.LessOrEqual(t, grant.ExpiresIn, 15*60)
		require.False(t, grant.StoresAccessTokenOnly)
		require.True(t, grant.RefreshSupported)
		require.NotEmpty(t, grant.ReauthMessage)

		parsed, err := url.Parse(grant.AuthorizeURL)
		require.NoError(t, err)
		query := parsed.Query()
		require.Equal(t, "code", query.Get("response_type"))
		require.Equal(t, "app_EMoamEEZ73f0CkXaXp7hrann", query.Get("client_id"))
		require.Equal(t, "http://localhost:1455/auth/callback", query.Get("redirect_uri"))
		require.Equal(t, "openid profile email offline_access", query.Get("scope"))
		require.NotEmpty(t, query.Get("code_challenge"), "PKCE challenge travels in the URL")
		require.Equal(t, "S256", query.Get("code_challenge_method"))
		require.NotEmpty(t, query.Get("state"))

		// The full callback URL pastes back, state-checked server-side.
		callback := "http://localhost:1455/auth/callback?code=test-auth-code&state=" + browserGrantState(t, grant.AuthorizeURL)
		authorized, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: callback})
		require.NoError(t, err)
		require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, authorized.Status)
		require.False(t, authorized.StoresAccessTokenOnly)
		require.True(t, authorized.RefreshSupported)
		require.NotEmpty(t, authorized.ReauthMessage)
		authorizedJSON, err := json.Marshal(authorized) // #nosec G117 -- test asserts the refresh token is absent from this payload.
		require.NoError(t, err)
		require.NotContains(t, string(authorizedJSON), "test-device-refresh-token", "refresh token never leaves the server")

		// Server-side custody: the credential reached the user key row on
		// exchange with no dashboard PUT, shaped exactly like a
		// device-code approval.
		configs, err = memberClient.ListUserAIProviderKeyConfigs(ctx, "me")
		require.NoError(t, err)
		cfg = findKeyConfig(configs, provider.ID)
		require.NotNil(t, cfg)
		require.True(t, cfg.HasUserAPIKey)
		require.NotNil(t, cfg.OAuthExpiry, "expiry visible on the keys page")
		require.WithinDuration(t, time.Now().Add(time.Hour), *cfg.OAuthExpiry, 5*time.Minute)
		require.True(t, cfg.RefreshSupported)
		require.False(t, cfg.ReauthRequired)

		memberUser, err := memberClientRaw.User(ctx, "me")
		require.NoError(t, err)
		row, err := api.Database.GetUserAIProviderKeyByProviderID(dbauthz.AsSystemRestricted(ctx), database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       memberUser.ID,
			AIProviderID: provider.ID,
		})
		require.NoError(t, err)
		require.Equal(t, scriptedAccessJWT(t, "acct-browser-outer"), row.APIKey)
		require.True(t, row.OAuthRefreshToken.Valid, "refresh token persisted server-side")
		require.Equal(t, "test-device-refresh-token", row.OAuthRefreshToken.String)
		require.True(t, row.OAuthExpiry.Valid, "expiry persisted from expires_in")
		require.WithinDuration(t, time.Now().Add(time.Hour), row.OAuthExpiry.Time, 5*time.Minute)
		require.True(t, row.AccountID.Valid)
		require.Equal(t, "acct-browser-outer", row.AccountID.String)
	})

	t.Run("StateMismatchAndMissingCode", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		provider := createChatGPTProvider(t, adminClient)
		useScriptedBrowserFlow(api, &scriptedBrowserExchanger{
			exchangeGrant: scriptedTokenGrant(t, "acct-browser-outer"),
		})

		grant, err := memberClient.InitiateUserAIBrowserGrant(ctx, "me", provider.ID)
		require.NoError(t, err)

		stale := "http://localhost:1455/auth/callback?code=test-auth-code&state=stale-state"
		err = func() error {
			_, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: stale})
			return err
		}()
		requireSDKError(t, err, http.StatusBadRequest)

		err = func() error {
			_, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: "   "})
			return err
		}()
		requireSDKError(t, err, http.StatusBadRequest)

		// The grant stays pending through bad pastes; the newest
		// callback still completes it.
		callback := "http://localhost:1455/auth/callback?code=test-auth-code&state=" + browserGrantState(t, grant.AuthorizeURL)
		authorized, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: callback})
		require.NoError(t, err)
		require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, authorized.Status)
	})

	t.Run("WrongUserIsolation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, memberUser := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)

		provider := createChatGPTProvider(t, adminClient)
		useScriptedBrowserFlow(api, &scriptedBrowserExchanger{
			exchangeGrant: scriptedTokenGrant(t, "acct-browser-outer"),
		})

		// Another user cannot mint into this user's slot.
		_, err := otherClient.InitiateUserAIBrowserGrant(ctx, memberUser.ID.String(), provider.ID)
		requireSDKError(t, err, http.StatusNotFound)

		grant, err := memberClient.InitiateUserAIBrowserGrant(ctx, "me", provider.ID)
		require.NoError(t, err)

		// Another user cannot exchange or cancel the grant.
		_, err = otherClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: "test-auth-code"})
		requireSDKError(t, err, http.StatusNotFound)
		requireSDKError(t, otherClient.CancelUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID), http.StatusNotFound)

		// The owner's grant is untouched by the probes.
		authorized, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: "test-auth-code"})
		require.NoError(t, err)
		require.Equal(t, codersdk.AIDeviceGrantStatusAuthorized, authorized.Status)
	})

	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		provider := createChatGPTProvider(t, adminClient)
		useScriptedBrowserFlow(api, &scriptedBrowserExchanger{
			exchangeGrant: scriptedTokenGrant(t, "acct-browser-outer"),
		})

		grant, err := memberClient.InitiateUserAIBrowserGrant(ctx, "me", provider.ID)
		require.NoError(t, err)
		require.NoError(t, memberClient.CancelUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID))

		canceled, err := memberClient.ExchangeUserAIBrowserGrant(ctx, "me", provider.ID, grant.GrantID, codersdk.AIBrowserGrantExchangeRequest{Input: "test-auth-code"})
		require.NoError(t, err, "exchange on a settled grant returns its snapshot")
		require.Equal(t, codersdk.AIDeviceGrantStatusCanceled, canceled.Status)

		requireSDKError(t,
			memberClient.CancelUserAIBrowserGrant(ctx, "me", provider.ID, uuid.New()),
			http.StatusNotFound,
		)
	})

	t.Run("UnsupportedProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, api := newChatClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		plain, err := adminClient.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAI,
			Name:    "plain-api-keys",
			Enabled: true,
			BaseURL: "https://api.openai.example.com/v1",
		})
		require.NoError(t, err)
		useScriptedBrowserFlow(api, &scriptedBrowserExchanger{})

		configs, err := memberClient.ListUserAIProviderKeyConfigs(ctx, "me")
		require.NoError(t, err)
		require.False(t, findKeyConfig(configs, plain.ID).BrowserFlowSupported)

		sdkErr := requireSDKError(t, func() error {
			_, err := memberClient.InitiateUserAIBrowserGrant(ctx, "me", plain.ID)
			return err
		}(), http.StatusUnprocessableEntity)
		require.Equal(t, "Browser sign-in is not supported for this provider.", sdkErr.Message)
	})
}
