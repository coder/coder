package coderd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/stretchr/testify/assert"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

// these tests patch the map of license keys, so cannot be run in parallel
// nolint:paralleltest
func TestPostLicense(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyID := "testing"
	oldKeys := keys
	defer func() {
		t.Log("restoring keys")
		keys = oldKeys
	}()
	keys = map[string]ed25519.PublicKey{keyID: pubKey}

	t.Run("POST", func(t *testing.T) {
		client := coderdtest.New(t, &coderdtest.Options{APIBuilder: NewEnterprise})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test@coder.test",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
			},
			LicenseExpires: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			AccountType:    AccountTypeSalesforce,
			AccountID:      "testing",
			Version:        CurrentVersion,
			Features: Features{
				UserLimit: 0,
				AuditLog:  1,
			},
		}
		lic, err := makeLicense(claims, privKey, keyID)
		require.NoError(t, err)

		respLic, err := client.AddLicense(ctx, codersdk.AddLicenseRequest{
			License: lic,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, respLic.ID, int32(0))
		// just a couple spot checks for sanity
		assert.Equal(t, claims.AccountID, respLic.Claims["account_id"])
		features, ok := respLic.Claims["features"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, json.Number("1"), features[codersdk.FeatureAuditLog])
	})

	t.Run("POST_unathorized", func(t *testing.T) {
		client := coderdtest.New(t, &coderdtest.Options{APIBuilder: NewEnterprise})
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test@coder.test",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
			},
			LicenseExpires: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			AccountType:    AccountTypeSalesforce,
			AccountID:      "testing",
			Version:        CurrentVersion,
			Features: Features{
				UserLimit: 0,
				AuditLog:  1,
			},
		}
		lic, err := makeLicense(claims, privKey, keyID)
		require.NoError(t, err)

		_, err = client.AddLicense(ctx, codersdk.AddLicenseRequest{
			License: lic,
		})
		errResp := &codersdk.Error{}
		if xerrors.As(err, &errResp) {
			assert.Equal(t, 401, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 401")
		}
	})

	t.Run("POST_corrupted", func(t *testing.T) {
		client := coderdtest.New(t, &coderdtest.Options{APIBuilder: NewEnterprise})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "test@coder.test",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
			},
			LicenseExpires: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			AccountType:    AccountTypeSalesforce,
			AccountID:      "testing",
			Version:        CurrentVersion,
			Features: Features{
				UserLimit: 0,
				AuditLog:  1,
			},
		}
		lic, err := makeLicense(claims, privKey, keyID)
		require.NoError(t, err)

		_, err = client.AddLicense(ctx, codersdk.AddLicenseRequest{
			License: "h" + lic,
		})
		errResp := &codersdk.Error{}
		if xerrors.As(err, &errResp) {
			assert.Equal(t, 400, errResp.StatusCode())
		} else {
			t.Error("expected to get error status 400")
		}
	})
}

func makeLicense(c *Claims, privateKey ed25519.PrivateKey, keyID string) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header[HeaderKeyID] = keyID
	signedTok, err := tok.SignedString(privateKey)
	if err != nil {
		return "", xerrors.Errorf("sign license: %w", err)
	}
	return signedTok, nil
}
