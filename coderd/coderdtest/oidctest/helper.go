package oidctest

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// LoginHelper helps with logging in a user and refreshing their oauth tokens.
// It is mainly because refreshing oauth tokens is a bit tricky and requires
// some database manipulation.
type LoginHelper struct {
	fake   *FakeIDP
	client *codersdk.Client
}

func NewLoginHelper(client *codersdk.Client, fake *FakeIDP) *LoginHelper {
	if client == nil {
		panic("client must not be nil")
	}
	if fake == nil {
		panic("fake must not be nil")
	}
	return &LoginHelper{
		fake:   fake,
		client: client,
	}
}

// Login just helps by making an unauthenticated client and logging in with
// the given claims. All Logins should be unauthenticated, so this is a
// convenience method.
func (h *LoginHelper) Login(t *testing.T, idTokenClaims jwt.MapClaims) (*codersdk.Client, *http.Response) {
	t.Helper()
	unauthenticatedClient := codersdk.New(h.client.URL)

	return h.fake.Login(t, unauthenticatedClient, idTokenClaims)
}

// AttemptLogin does not assert a successful login.
func (h *LoginHelper) AttemptLogin(t *testing.T, idTokenClaims jwt.MapClaims) (*codersdk.Client, *http.Response) {
	t.Helper()
	unauthenticatedClient := codersdk.New(h.client.URL)

	return h.fake.AttemptLogin(t, unauthenticatedClient, idTokenClaims)
}

// ExpireOauthToken expires the oauth token for the given user.
func (*LoginHelper) ExpireOauthToken(t *testing.T, db database.Store, user *codersdk.Client) database.UserLink {
	t.Helper()

	//nolint:gocritic // Testing
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitMedium))

	id, _, err := httpmw.SplitAPIToken(user.SessionToken())
	require.NoError(t, err)

	// We need to get the OIDC link and update it in the database to force
	// it to be expired.
	key, err := db.GetAPIKeyByID(ctx, id)
	require.NoError(t, err, "get api key")

	link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    key.UserID,
		LoginType: database.LoginTypeOIDC,
	})
	require.NoError(t, err, "get user link")

	// Expire the oauth link for the given user.
	updated, err := db.UpdateUserLink(ctx, database.UpdateUserLinkParams{
		OAuthAccessToken:       link.OAuthAccessToken,
		OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
		OAuthRefreshToken:      link.OAuthRefreshToken,
		OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
		OAuthExpiry:            time.Now().Add(time.Hour * -1),
		UserID:                 link.UserID,
		LoginType:              link.LoginType,
		Claims:                 database.UserLinkClaims{},
	})
	require.NoError(t, err, "expire user link")

	return updated
}

// ForceRefresh forces the client to refresh its oauth token. It does this by
// expiring the oauth token, then doing an authenticated call. This will force
// the API Key middleware to refresh the oauth token.
//
// A unit test assertion makes sure the refresh token is used.
func (h *LoginHelper) ForceRefresh(t *testing.T, db database.Store, user *codersdk.Client, idToken jwt.MapClaims) {
	t.Helper()

	link := h.ExpireOauthToken(t, db, user)
	// Updates the claims that the IDP will return. By default, it always
	// uses the original claims for the original oauth token.
	h.fake.UpdateRefreshClaims(link.OAuthRefreshToken, idToken)

	t.Cleanup(func() {
		require.True(t, h.fake.RefreshUsed(link.OAuthRefreshToken), "refresh token must be used, but has not. Did you forget to call the returned function from this call?")
	})

	// Do any authenticated call to force the refresh
	_, err := user.User(testutil.Context(t, testutil.WaitShort), "me")
	require.NoError(t, err, "user must be able to be fetched")
}

// OAuth2GetCode emulates a user clicking "allow" on the IDP page. When doing
// unit tests, it's easier to skip this step sometimes. It does make an actual
// request to the IDP, so it should be equivalent to doing this "manually" with
// actual requests.
func OAuth2GetCode(rawAuthURL string, doRequest func(req *http.Request) (*http.Response, error)) (string, error) {
	authURL, err := url.Parse(rawAuthURL)
	if err != nil {
		return "", xerrors.Errorf("failed to parse auth URL: %w", err)
	}

	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawAuthURL, nil)
	if err != nil {
		return "", xerrors.Errorf("failed to create auth request: %w", err)
	}

	expCode := http.StatusTemporaryRedirect
	resp, err := doRequest(r)
	if err != nil {
		return "", xerrors.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expCode {
		return "", codersdk.ReadBodyAsError(resp)
	}

	to := resp.Header.Get("Location")
	if to == "" {
		return "", xerrors.Errorf("expected redirect location")
	}

	toURL, err := url.Parse(to)
	if err != nil {
		return "", xerrors.Errorf("failed to parse redirect location: %w", err)
	}

	code := toURL.Query().Get("code")
	if code == "" {
		return "", xerrors.Errorf("expected code in redirect location")
	}

	state := authURL.Query().Get("state")
	newState := toURL.Query().Get("state")
	if newState != state {
		return "", xerrors.Errorf("expected state %q, got %q", state, newState)
	}
	return code, nil
}
