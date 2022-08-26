package coderd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

// TestAuthorizeAllEndpoints will check `authorize` is called on every endpoint registered.
// these tests patch the map of license keys, so cannot be run in parallel
// nolint:paralleltest
func TestAuthorizeAllEndpoints(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyID := "testing"
	oldKeys := keys
	defer func() {
		t.Log("restoring keys")
		keys = oldKeys
	}()
	keys = map[string]ed25519.PublicKey{keyID: pubKey}

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	a := coderdtest.NewAuthTester(ctx, t, &coderdtest.Options{APIBuilder: NewEnterprise})

	// We need a license in the DB, so that when we call GET api/v2/licenses there is one in the
	// list to check authz on.
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
	license, err := a.Client.AddLicense(ctx, codersdk.AddLicenseRequest{
		License: lic,
	})
	require.NoError(t, err)
	a.URLParams["licenses/{id}"] = fmt.Sprintf("licenses/%d", license.ID)

	skipRoutes, assertRoute := coderdtest.AGPLRoutes(a)
	assertRoute["POST:/api/v2/licenses"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionCreate,
		AssertObject: rbac.ResourceLicense,
	}
	assertRoute["GET:/api/v2/licenses"] = coderdtest.RouteCheck{
		StatusCode:   http.StatusOK,
		AssertAction: rbac.ActionRead,
		AssertObject: rbac.ResourceLicense,
	}
	assertRoute["DELETE:/api/v2/licenses/{id}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionDelete,
		AssertObject: rbac.ResourceLicense,
	}
	a.Test(ctx, assertRoute, skipRoutes)
}
