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

// TestMergeAIProviderSettingsUpstreamHeaders verifies PATCH merge semantics
// for the headers variant: a headers patch replaces stored settings verbatim
// (headers carry no secrets to carry forward), and a zero patch clears.
func TestMergeAIProviderSettingsUpstreamHeaders(t *testing.T) {
	t.Parallel()

	headers := func(h map[string]string) codersdk.AIProviderSettings {
		return codersdk.AIProviderSettings{
			UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{Headers: h},
		}
	}

	t.Run("HeadersPatchReplacesBedrock", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}}
		patch := headers(map[string]string{"X-A": "b"})
		merged := mergeAIProviderSettings(existing, patch)
		require.Nil(t, merged.Bedrock)
		require.Equal(t, map[string]string{"X-A": "b"}, merged.UpstreamHeaders.Headers)
	})

	t.Run("HeadersPatchReplacesHeaders", func(t *testing.T) {
		t.Parallel()
		existing := headers(map[string]string{"X-Old": "1"})
		patch := headers(map[string]string{"X-New": "2"})
		merged := mergeAIProviderSettings(existing, patch)
		require.Equal(t, map[string]string{"X-New": "2"}, merged.UpstreamHeaders.Headers)
	})

	t.Run("ZeroPatchClears", func(t *testing.T) {
		t.Parallel()
		existing := headers(map[string]string{"X-Old": "1"})
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{})
		require.True(t, merged.IsZero())
	})

	t.Run("BedrockPatchPreservesSecrets", func(t *testing.T) {
		t.Parallel()
		// A bedrock patch over headers-only settings has no stored secrets
		// to carry forward; it applies verbatim.
		secret := "secret"
		existing := headers(map[string]string{"X-Old": "1"})
		patch := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:          "us-east-1",
			AccessKeySecret: &secret,
		}}
		merged := mergeAIProviderSettings(existing, patch)
		require.Nil(t, merged.UpstreamHeaders)
		require.NotNil(t, merged.Bedrock)
		require.Equal(t, "us-east-1", merged.Bedrock.Region)
		require.Equal(t, &secret, merged.Bedrock.AccessKeySecret)
	})
}
