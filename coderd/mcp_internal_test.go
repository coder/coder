package coderd

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

// dbauthzTestStore wraps the test database with the same dbauthz layer
// used in production (coderd.go:370). Without it the test would not
// catch RBAC failures from the chatd subject; with it the test fails
// loudly if the elevation in OIDCAccessToken is removed or weakened.
func dbauthzTestStore(t *testing.T, db database.Store) database.Store {
	t.Helper()

	authz := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var tacs dbauthz.AccessControlStore = fakeAccessControlStore{}
	acs.Store(&tacs)
	return dbauthz.New(db, authz, testutil.Logger(t), acs)
}

// fakeAccessControlStore mirrors coderdtest.FakeAccessControlStore but is
// inlined here to avoid an import cycle (coderdtest imports coderd).
type fakeAccessControlStore struct{}

func (fakeAccessControlStore) GetTemplateAccessControl(t database.Template) dbauthz.TemplateAccessControl {
	return dbauthz.TemplateAccessControl{
		RequireActiveVersion: t.RequireActiveVersion,
	}
}

func (fakeAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, dbauthz.TemplateAccessControl) error {
	panic("not implemented")
}

func TestShouldRefreshOIDCToken(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	cases := []struct {
		name string
		link database.UserLink
		want bool
	}{
		{
			name: "NoRefreshToken",
			link: database.UserLink{OAuthExpiry: now.Add(-time.Hour)},
		},
		{
			name: "ZeroExpiry",
			link: database.UserLink{OAuthRefreshToken: "refresh"},
		},
		{
			name: "Expired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-time.Hour),
			},
			want: true,
		},
		{
			name: "Fresh",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := shouldRefreshOIDCToken(tc.link)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestOIDCMCPTokenSource(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	t.Run("NilConfig", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		require.Nil(t, newOIDCMCPTokenSource(db, nil, logger))
	})

	t.Run("NoLink", func(t *testing.T) {
		// When the user has no OIDC link the source returns ("", nil)
		// rather than an error so the caller can fall through to
		// "no Authorization header".
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, tok)
	})

	t.Run("FreshToken", func(t *testing.T) {
		// A non-expired token is returned as-is; no refresh is performed.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "fresh",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(time.Hour),
		})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{
			Token: &oauth2.Token{AccessToken: "should-not-be-used"},
		}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, "fresh", tok)
	})

	t.Run("RefreshExpired", func(t *testing.T) {
		// An expired token triggers a refresh; the new token is
		// persisted via UpdateUserLink. This exercises the dbauthz
		// elevation: chatd lacks ResourceSystem.Read and
		// ResourceUser.UpdatePersonal so a non-elevated context
		// would fail both reads and writes.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "stale",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(-time.Hour),
		})

		src := newOIDCMCPTokenSource(store, &testutil.OAuth2Config{
			Token: &oauth2.Token{
				AccessToken:  "fresh",
				RefreshToken: "new-refresh",
				Expiry:       dbtime.Now().Add(time.Hour),
			},
		}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, "fresh", tok)

		// Verify the refresh was persisted via UpdateUserLink.
		got, err := db.GetUserLinkByUserIDLoginType(
			dbauthz.AsSystemRestricted(context.Background()),
			database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
			},
		)
		require.NoError(t, err)
		require.Equal(t, "fresh", got.OAuthAccessToken)
		require.Equal(t, "new-refresh", got.OAuthRefreshToken)
	})

	t.Run("RefreshFailureReturnsEmpty", func(t *testing.T) {
		// A refresh attempt that fails (e.g. invalid client config)
		// must not surface an error to the caller; per the
		// UserOIDCTokenSource contract this is treated as "no
		// Authorization header".
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		store := dbauthzTestStore(t, db)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:            user.ID,
			LoginType:         database.LoginTypeOIDC,
			OAuthAccessToken:  "stale",
			OAuthRefreshToken: "refresh",
			OAuthExpiry:       dbtime.Now().Add(-time.Hour),
		})

		// An empty oauth2.Config triggers a refresh failure
		// because it has no token endpoint to call.
		src := newOIDCMCPTokenSource(store, &oauth2.Config{}, logger)
		ctx := dbauthz.AsChatd(context.Background())

		tok, err := src.OIDCAccessToken(ctx, user.ID)
		require.NoError(t, err)
		require.Empty(t, tok)
	})
}

func newMCPOAuth2TestAPI(keyByte byte) *API {
	api := &API{Options: &Options{}}
	for i := range api.OAuthSigningKey {
		api.OAuthSigningKey[i] = keyByte + byte(i)
	}
	return api
}

func TestMCPOAuth2StateRoundTrip(t *testing.T) {
	t.Parallel()

	api := newMCPOAuth2TestAPI(1)
	original := uuid.New()

	state, nonce, err := api.signMCPOAuth2State(original)
	require.NoError(t, err)
	require.NotEmpty(t, state)
	require.NotEmpty(t, nonce)

	got, gotNonce, err := api.verifyMCPOAuth2State(state)
	require.NoError(t, err)
	require.Equal(t, original, got)
	require.Equal(t, nonce, gotNonce)
}

func TestMCPOAuth2StateRejectsTampered(t *testing.T) {
	t.Parallel()

	api := newMCPOAuth2TestAPI(1)
	state, _, err := api.signMCPOAuth2State(uuid.New())
	require.NoError(t, err)

	// Flipping a byte inside the signature must fail HMAC
	// verification. We pick the first character after the
	// payload-signature delimiter so it is always a payload bit
	// of the signature, never a base64 padding bit.
	dotIdx := strings.Index(state, ".")
	require.Greater(t, dotIdx, 0)
	flipped := flipBase64URLChar(state[dotIdx+1])
	tampered := state[:dotIdx+1] + string(flipped) + state[dotIdx+2:]
	require.NotEqual(t, state, tampered, "tampered state must differ")
	_, _, err = api.verifyMCPOAuth2State(tampered)
	require.Error(t, err)

	// Garbage in is rejected too.
	_, _, err = api.verifyMCPOAuth2State("garbage")
	require.Error(t, err)

	// Empty input is rejected.
	_, _, err = api.verifyMCPOAuth2State("")
	require.Error(t, err)
}

// flipBase64URLChar returns a different valid base64url character
// (deterministically, by mapping each character to the next one in
// the alphabet). Used by tests to produce a tampered signature that
// is still well-formed base64url but decodes to different bytes.
func flipBase64URLChar(c byte) byte {
	switch {
	case c >= 'A' && c < 'Z':
		return c + 1
	case c == 'Z':
		return 'a'
	case c >= 'a' && c < 'z':
		return c + 1
	case c == 'z':
		return '0'
	case c >= '0' && c < '9':
		return c + 1
	case c == '9':
		return '-'
	case c == '-':
		return '_'
	case c == '_':
		return 'A'
	default:
		return 'A'
	}
}

func TestMCPOAuth2StateRejectsDifferentKey(t *testing.T) {
	t.Parallel()

	signer := newMCPOAuth2TestAPI(1)
	verifier := newMCPOAuth2TestAPI(2)

	state, _, err := signer.signMCPOAuth2State(uuid.New())
	require.NoError(t, err)

	_, _, err = verifier.verifyMCPOAuth2State(state)
	require.Error(t, err, "signature must not validate under a different key")
}
