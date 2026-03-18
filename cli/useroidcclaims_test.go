package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUserOIDCClaims(t *testing.T) {
	t.Parallel()

	fake := oidctest.NewFakeIDP(t,
		oidctest.WithServing(),
	)
	cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
		cfg.AllowSignups = true
	})
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		OIDCConfig: cfg,
	})

	t.Run("OwnClaims", func(t *testing.T) {
		t.Parallel()

		claims := jwt.MapClaims{
			"email":          "alice@coder.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
			"groups":         []string{"admin", "eng"},
		}
		userClient, loginResp := fake.Login(t, ownerClient, claims)
		defer loginResp.Body.Close()

		inv, root := clitest.New(t, "users", "oidc-claims", "-o", "json")
		clitest.SetupConfig(t, userClient, root)

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		err := inv.WithContext(testutil.Context(t, testutil.WaitMedium)).Run()
		require.NoError(t, err)

		var resp codersdk.OIDCClaimsResponse
		err = json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err, "unmarshal JSON output")
		require.NotEmpty(t, resp.Claims, "claims should not be empty")
		assert.Equal(t, "alice@coder.com", resp.Claims["email"])
	})

	t.Run("Table", func(t *testing.T) {
		t.Parallel()

		claims := jwt.MapClaims{
			"email":          "bob@coder.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}
		userClient, loginResp := fake.Login(t, ownerClient, claims)
		defer loginResp.Body.Close()

		inv, root := clitest.New(t, "users", "oidc-claims")
		clitest.SetupConfig(t, userClient, root)

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		err := inv.WithContext(testutil.Context(t, testutil.WaitMedium)).Run()
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, "email")
		require.Contains(t, output, "bob@coder.com")
	})

	t.Run("NotOIDCUser", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "users", "oidc-claims")
		clitest.SetupConfig(t, client, root)

		err := inv.WithContext(testutil.Context(t, testutil.WaitMedium)).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "not an OIDC user")
	})

	// Verify that two different OIDC users each only see their own
	// claims. The endpoint has no user parameter, so there is no way
	// to request another user's claims by design.
	t.Run("OnlyOwnClaims", func(t *testing.T) {
		t.Parallel()

		aliceClaims := jwt.MapClaims{
			"email":          "alice-isolation@coder.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}
		aliceClient, aliceLoginResp := fake.Login(t, ownerClient, aliceClaims)
		defer aliceLoginResp.Body.Close()

		bobClaims := jwt.MapClaims{
			"email":          "bob-isolation@coder.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}
		bobClient, bobLoginResp := fake.Login(t, ownerClient, bobClaims)
		defer bobLoginResp.Body.Close()

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Alice sees her own claims.
		aliceResp, err := aliceClient.UserOIDCClaims(ctx)
		require.NoError(t, err)
		assert.Equal(t, "alice-isolation@coder.com", aliceResp.Claims["email"])

		// Bob sees his own claims.
		bobResp, err := bobClient.UserOIDCClaims(ctx)
		require.NoError(t, err)
		assert.Equal(t, "bob-isolation@coder.com", bobResp.Claims["email"])
	})

	t.Run("ClaimsNeverNull", func(t *testing.T) {
		t.Parallel()

		// Use minimal claims — just enough for OIDC login.
		claims := jwt.MapClaims{
			"email":          "minimal@coder.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}
		userClient, loginResp := fake.Login(t, ownerClient, claims)
		defer loginResp.Body.Close()

		ctx := testutil.Context(t, testutil.WaitMedium)
		resp, err := userClient.UserOIDCClaims(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp.Claims, "claims should never be nil, expected empty map")
	})
}
