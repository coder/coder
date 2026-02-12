package coderd_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWebAuthnListCredentials(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	_ = owner

	// Initially no credentials.
	creds, err := client.ListWebAuthnCredentials(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, creds, 0)

	// Seed a credential directly in the database.
	//nolint:gocritic // Test uses system context for seeding.
	_, err = api.Database.InsertWebAuthnCredential(dbauthz.AsSystemRestricted(ctx), database.InsertWebAuthnCredentialParams{
		ID:              uuid.New(),
		UserID:          owner.UserID,
		CredentialID:    []byte("test-credential-id"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Aaguid:          make([]byte, 16),
		SignCount:       0,
		Name:            "Test Key",
	})
	require.NoError(t, err)

	// Now list should return one credential.
	creds, err = client.ListWebAuthnCredentials(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, creds, 1)
	assert.Equal(t, "Test Key", creds[0].Name)
	assert.Equal(t, owner.UserID, creds[0].UserID)
}

func TestWebAuthnDeleteCredential(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)

	// Seed a credential.
	credID := uuid.New()
	//nolint:gocritic // Test uses system context for seeding.
	_, err := api.Database.InsertWebAuthnCredential(dbauthz.AsSystemRestricted(ctx), database.InsertWebAuthnCredentialParams{
		ID:              credID,
		UserID:          owner.UserID,
		CredentialID:    []byte("delete-me-credential"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Aaguid:          make([]byte, 16),
		SignCount:       0,
		Name:            "Delete Me",
	})
	require.NoError(t, err)

	// Delete it.
	err = client.DeleteWebAuthnCredential(ctx, codersdk.Me, credID)
	require.NoError(t, err)

	// Verify it's gone.
	creds, err := client.ListWebAuthnCredentials(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, creds, 0)
}

func TestWebAuthnDeleteCredential_NotFound(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	err := client.DeleteWebAuthnCredential(ctx, codersdk.Me, uuid.New())
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}

func TestWebAuthnDeleteCredential_OtherUser(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Seed a credential for the owner.
	credID := uuid.New()
	//nolint:gocritic // Test uses system context for seeding.
	_, err := api.Database.InsertWebAuthnCredential(dbauthz.AsSystemRestricted(ctx), database.InsertWebAuthnCredentialParams{
		ID:              credID,
		UserID:          owner.UserID,
		CredentialID:    []byte("owner-credential"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Aaguid:          make([]byte, 16),
		SignCount:       0,
		Name:            "Owner Key",
	})
	require.NoError(t, err)

	// Member should not be able to delete the owner's credential.
	err = memberClient.DeleteWebAuthnCredential(ctx, codersdk.Me, credID)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	// The credential won't be found for 'me' because it belongs
	// to the owner, not the member.
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}

func TestWebAuthnBeginRegistration(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// Begin registration should return valid creation options.
	creation, err := client.BeginWebAuthnRegistration(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotEmpty(t, creation.Response.Challenge)
	require.NotEmpty(t, creation.Response.RelyingParty.Name)
	require.Equal(t, "Coder", creation.Response.RelyingParty.Name)
}

func TestWebAuthnChallenge_NoCredentials(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// Challenge should fail when no credentials are registered.
	_, err := client.RequestWebAuthnChallenge(ctx, codersdk.Me)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusPreconditionFailed, sdkErr.StatusCode())
}

func TestWebAuthnChallenge_WithCredentials(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)

	// Seed a credential.
	//nolint:gocritic // Test uses system context for seeding.
	_, err := api.Database.InsertWebAuthnCredential(dbauthz.AsSystemRestricted(ctx), database.InsertWebAuthnCredentialParams{
		ID:              uuid.New(),
		UserID:          owner.UserID,
		CredentialID:    []byte("test-credential"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Aaguid:          make([]byte, 16),
		SignCount:       0,
		Name:            "Test Key",
	})
	require.NoError(t, err)

	// Challenge should succeed and return assertion options.
	assertion, err := client.RequestWebAuthnChallenge(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotEmpty(t, assertion.Response.Challenge)
	require.NotEmpty(t, assertion.Response.AllowedCredentials)
}

func TestVerifyWebAuthnConnectJWT(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	userID := uuid.New()
	now := time.Now()
	claims := &coderd.WebAuthnConnectClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   userID.String(),
			Audience:  jwt.Audience{"coder-connect"},
			Expiry:    jwt.NewNumericDate(now.Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	// Sign a valid JWT.
	token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Verify it.
	parsedUserID, err := api.VerifyWebAuthnConnectJWT(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, userID, parsedUserID)
}

func TestVerifyWebAuthnConnectJWT_Expired(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	now := time.Now()
	claims := &coderd.WebAuthnConnectClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   uuid.NewString(),
			Audience:  jwt.Audience{"coder-connect"},
			Expiry:    jwt.NewNumericDate(now.Add(-1 * time.Minute)), // expired
			NotBefore: jwt.NewNumericDate(now.Add(-10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-10 * time.Minute)),
			ID:        uuid.NewString(),
		},
	}

	token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
	require.NoError(t, err)

	_, err = api.VerifyWebAuthnConnectJWT(ctx, token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify connection JWT")
}

func TestVerifyWebAuthnConnectJWT_WrongAudience(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	now := time.Now()
	claims := &coderd.WebAuthnConnectClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   uuid.NewString(),
			Audience:  jwt.Audience{"wrong-audience"},
			Expiry:    jwt.NewNumericDate(now.Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
	require.NoError(t, err)

	_, err = api.VerifyWebAuthnConnectJWT(ctx, token)
	require.Error(t, err)
}

func TestVerifyWebAuthnConnectJWT_InvalidToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	_, err := api.VerifyWebAuthnConnectJWT(ctx, "not-a-valid-jwt")
	require.Error(t, err)
}

func TestVerifyWebAuthnConnectJWT_ReplayRejected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	userID := uuid.New()
	now := time.Now()
	jti := uuid.NewString()
	claims := &coderd.WebAuthnConnectClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   userID.String(),
			Audience:  jwt.Audience{"coder-connect"},
			Expiry:    jwt.NewNumericDate(now.Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
	}

	token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
	require.NoError(t, err)

	// First use should succeed.
	parsedID, err := api.VerifyWebAuthnConnectJWT(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, userID, parsedID)

	// Second use of the same token should fail (replay).
	_, err = api.VerifyWebAuthnConnectJWT(ctx, token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already been used")
}

func TestVerifyWebAuthnConnectJWT_DifferentJTIsAllowed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	_, _, api := coderdtest.NewWithAPI(t, nil)

	userID := uuid.New()
	now := time.Now()

	// Two tokens with different JTIs should both work.
	for i := 0; i < 2; i++ {
		claims := &coderd.WebAuthnConnectClaims{
			RegisteredClaims: jwtutils.RegisteredClaims{
				Issuer:    api.DeploymentID,
				Subject:   userID.String(),
				Audience:  jwt.Audience{"coder-connect"},
				Expiry:    jwt.NewNumericDate(now.Add(5 * time.Minute)),
				NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Second)),
				IssuedAt:  jwt.NewNumericDate(now),
				ID:        uuid.NewString(), // unique JTI each time
			},
		}
		token, err := jwtutils.Sign(ctx, api.WebAuthnConnectKeyCache, claims)
		require.NoError(t, err)

		parsedID, err := api.VerifyWebAuthnConnectJWT(ctx, token)
		require.NoError(t, err, "token %d should verify", i)
		assert.Equal(t, userID, parsedID)
	}
}

func TestJTICache_MarkUsed(t *testing.T) {
	t.Parallel()

	cache := coderd.NewWebAuthnJTICacheForTest()

	// First use succeeds.
	assert.True(t, cache.MarkUsed("jti-1", time.Now().Add(5*time.Minute)))

	// Replay fails.
	assert.False(t, cache.MarkUsed("jti-1", time.Now().Add(5*time.Minute)))

	// Different JTI succeeds.
	assert.True(t, cache.MarkUsed("jti-2", time.Now().Add(5*time.Minute)))
}

func TestJTICache_ExpiredEntriesEvicted(t *testing.T) {
	t.Parallel()

	cache := coderd.NewWebAuthnJTICacheForTest()

	// Add an already-expired entry.
	assert.True(t, cache.MarkUsed("old-jti", time.Now().Add(-1*time.Second)))

	// Fill enough to trigger eviction (> maxSize/2).
	for i := 0; i < 6000; i++ {
		cache.MarkUsed(uuid.NewString(), time.Now().Add(5*time.Minute))
	}

	// The expired entry should have been evicted, so re-using it
	// should succeed (it's no longer in the cache).
	assert.True(t, cache.MarkUsed("old-jti", time.Now().Add(5*time.Minute)))
}
