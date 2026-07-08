package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestEnsureBedrockExternalID covers the server-owned external ID generation:
// it generates only when a role is configured and none is set, and never
// overwrites an existing value.
func TestEnsureBedrockExternalID(t *testing.T) {
	t.Parallel()

	t.Run("NilBedrockIsNoOp", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{}
		ensureBedrockExternalID(&s)
		require.Nil(t, s.Bedrock)
	})

	t.Run("NoRoleLeavesEmpty", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}}
		ensureBedrockExternalID(&s)
		require.Empty(t, s.Bedrock.ExternalID)
	})

	t.Run("GeneratesWhenRoleSet", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			RoleARN: "arn:aws:iam::123456789012:role/BedrockRole",
		}}
		ensureBedrockExternalID(&s)
		// The bounds are a sanity floor and ceiling, not a correctness
		// requirement. crypto/rand.Text() currently returns 26 chars, but
		// its docs allow future Go versions to return longer text. If a Go
		// upgrade trips these bounds, widen them or use different function.
		require.GreaterOrEqual(t, len(s.Bedrock.ExternalID), 26)
		require.LessOrEqual(t, len(s.Bedrock.ExternalID), 52)
	})

	t.Run("DoesNotOverwriteExisting", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			RoleARN:    "arn:aws:iam::123456789012:role/BedrockRole",
			ExternalID: "existing-value",
		}}
		ensureBedrockExternalID(&s)
		require.Equal(t, "existing-value", s.Bedrock.ExternalID)
	})

	t.Run("GeneratesUniqueValues", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]struct{})
		for range 10 {
			s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
				RoleARN: "arn:aws:iam::123456789012:role/BedrockRole",
			}}
			ensureBedrockExternalID(&s)
			_, dup := seen[s.Bedrock.ExternalID]
			require.False(t, dup, "external IDs must be unique per provider")
			seen[s.Bedrock.ExternalID] = struct{}{}
		}
	})
}

// TestMergeAIProviderSettingsExternalID verifies the external ID is treated as
// server-owned during a PATCH merge: a stored value is carried forward and
// overrides the patch so it can't be changed.
func TestMergeAIProviderSettingsExternalID(t *testing.T) {
	t.Parallel()

	roleARN := "arn:aws:iam::123456789012:role/BedrockRole"
	existing := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
		RoleARN:    roleARN,
		ExternalID: "stored-value",
	}}
	patch := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
		RoleARN:    roleARN,
		ExternalID: "client-supplied-value",
	}}
	merged := mergeAIProviderSettings(existing, patch)
	require.NotNil(t, merged.Bedrock)
	require.Equal(t, roleARN, merged.Bedrock.RoleARN)
	require.Equal(t, "stored-value", merged.Bedrock.ExternalID)
}

// TestMergeAIProviderSettingsWIF verifies WIF settings survive the
// PATCH merge: a WIF patch replaces the stored value wholesale, in
// both directions across the settings union.
func TestMergeAIProviderSettingsWIF(t *testing.T) {
	t.Parallel()

	wif := func(rule string) *codersdk.AIProviderWIFSettings {
		return &codersdk.AIProviderWIFSettings{
			FederationRuleID:  rule,
			OrganizationID:    "00000000-0000-0000-0000-000000000001",
			IdentityTokenFile: "/var/run/secrets/anthropic/token",
			ServiceAccountID:  "svac_test",
			WorkspaceID:       "wrkspc_test",
		}
	}

	t.Run("WIFPatchReplacesStoredWIF", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{WIF: wif("fdrl_old")}
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{WIF: wif("fdrl_new")})
		require.NotNil(t, merged.WIF)
		require.Equal(t, "fdrl_new", merged.WIF.FederationRuleID)
		require.Equal(t, *wif("fdrl_new"), *merged.WIF)
		require.Nil(t, merged.Bedrock)
	})

	t.Run("WIFPatchReplacesStoredBedrock", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}}
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{WIF: wif("fdrl_new")})
		require.NotNil(t, merged.WIF)
		require.Nil(t, merged.Bedrock)
	})

	t.Run("BedrockPatchReplacesStoredWIF", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{WIF: wif("fdrl_old")}
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}})
		require.Nil(t, merged.WIF)
		require.NotNil(t, merged.Bedrock)
	})

	t.Run("EmptyPatchClearsStoredWIF", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{WIF: wif("fdrl_old")}
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{})
		require.True(t, merged.IsZero())
	})
}
