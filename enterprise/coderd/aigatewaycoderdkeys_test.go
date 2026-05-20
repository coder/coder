package coderd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aigatewaycoderdkey"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// aibridgeEnabledOpts returns coderdenttest options that fully enable AI
// Bridge: feature entitlement + deployment config flag.
func aibridgeEnabledOpts(t *testing.T) *coderdenttest.Options {
	t.Helper()
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	return &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	}
}

func TestAIGatewayCoderdKeys(t *testing.T) {
	t.Parallel()

	// Single instance shared by all subtests (except FeatureGate).
	// Subtests run sequentially because they share server state.
	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, owner := coderdenttest.New(t, aibridgeEnabledOpts(t))

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("CRUD", func(t *testing.T) {
		keys, err := ownerClient.ListAIGatewayCoderdKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)

		name := uniqueName(t, "happy")

		created, err := ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: name})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.ID)
		require.Equal(t, name, created.Name)
		require.Len(t, created.KeyPrefix, aigatewaycoderdkey.KeyPrefixLength)
		require.Len(t, created.Key, aigatewaycoderdkey.KeyLength)
		require.True(t, strings.HasPrefix(created.KeyPrefix, aigatewaycoderdkey.KeyTypePrefix), "key_prefix must start with %q, got %q", aigatewaycoderdkey.KeyTypePrefix, created.KeyPrefix)
		require.True(t, strings.HasPrefix(created.Key, created.KeyPrefix), "key must begin with key_prefix")
		require.WithinDuration(t, time.Now(), created.CreatedAt, time.Minute)

		keys, err = ownerClient.ListAIGatewayCoderdKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, created.ID, keys[0].ID)
		require.Equal(t, created.Name, keys[0].Name)
		require.Equal(t, created.KeyPrefix, keys[0].KeyPrefix)
		require.Nil(t, keys[0].LastUsedAt)

		require.NoError(t, ownerClient.DeleteAIGatewayCoderdKey(ctx, created.ID))

		keys, err = ownerClient.ListAIGatewayCoderdKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("ListResponseDoesNotLeakSecrets", func(t *testing.T) {
		created, err := ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{
			Name: uniqueName(t, "leak"),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ownerClient.DeleteAIGatewayCoderdKey(ctx, created.ID)
		})

		// Raw HTTP read of LIST to confirm the JSON shape.
		resp, err := ownerClient.Request(ctx, http.MethodGet, "/api/v2/aibridge/coderd-keys", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var raw []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
		require.NotEmpty(t, raw)
		_, hasSecret := raw[0]["secret"]
		_, hasHashed := raw[0]["hashed_secret"]
		require.False(t, hasSecret, "LIST response leaked plaintext secret")
		require.False(t, hasHashed, "LIST response leaked hashed_secret")
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("CreateValidation", func(t *testing.T) {
		// Empty name -> 400 (validate:"required" on request struct).
		_, err := ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: ""})
		require.ErrorContains(t, err, "Validation failed")

		// >64 char name -> 400 (DB check constraint).
		longName := strings.Repeat("a", 65)
		_, err = ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: longName})
		require.ErrorContains(t, err, "Invalid key name")

		// Uppercase name -> 400 (DB check constraint rejects non-lowercase).
		_, err = ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: "UPPER-CASE"})
		require.ErrorContains(t, err, "Invalid key name")

		// Duplicate name -> 400.
		name := uniqueName(t, "dup")
		created, err := ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: name})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ownerClient.DeleteAIGatewayCoderdKey(ctx, created.ID)
		})
		_, err = ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{Name: name})
		require.ErrorContains(t, err, "must be unique")
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("DeleteValidation", func(t *testing.T) {
		// Invalid UUID -> 400 (raw request; SDK method accepts uuid.UUID).
		resp, err := ownerClient.Request(ctx, http.MethodDelete, "/api/v2/aibridge/coderd-keys/not-a-uuid", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Delete existing key -> 204 (SDK returns nil error on 204).
		created, err := ownerClient.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{
			Name: uniqueName(t, "del"),
		})
		require.NoError(t, err)
		require.NoError(t, ownerClient.DeleteAIGatewayCoderdKey(ctx, created.ID))

		// Unknown UUID -> 404.
		err = ownerClient.DeleteAIGatewayCoderdKey(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("ReturnsForbiddenForNonOwners", func(t *testing.T) {
		member, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		_, err := member.CreateAIGatewayCoderdKey(ctx, codersdk.CreateAIGatewayCoderdKeyRequest{
			Name: uniqueName(t, "denied"),
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		_, err = member.ListAIGatewayCoderdKeys(ctx)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		err = member.DeleteAIGatewayCoderdKey(ctx, uuid.New())
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	// FeatureGate needs a separate instance without the AI Bridge entitlement.
	t.Run("FeatureGate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})

		//nolint:gocritic // Managing AI Gateway coderd keys is owner-only.
		_, err := ownerClient.ListAIGatewayCoderdKeys(ctx)
		require.Error(t, err)
	})
}

func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	return strings.ToLower(fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()))
}
