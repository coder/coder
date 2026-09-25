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

// TestMergeAIProviderSettingsClaudePlatform verifies replacement and clearing
// across settings variants.
func TestMergeAIProviderSettingsClaudePlatform(t *testing.T) {
	t.Parallel()

	t.Run("RegularFieldsReplace", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			Region:      "us-east-1",
			WorkspaceID: "stored-workspace",
		}}
		patch := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			Region:      "us-west-2",
			WorkspaceID: "patched-workspace",
		}}
		merged := mergeAIProviderSettings(existing, patch)
		require.NotNil(t, merged.ClaudePlatformAWS)
		require.Equal(t, patch.ClaudePlatformAWS, merged.ClaudePlatformAWS)
	})

	t.Run("SwitchingVariantsReplaces", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			Region:      "us-east-1",
			WorkspaceID: "stored-workspace",
		}}
		patch := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:         "us-east-1",
			Model:          "anthropic.claude-sonnet-4-5",
			SmallFastModel: "anthropic.claude-haiku-4-5",
		}}
		merged := mergeAIProviderSettings(existing, patch)
		require.Nil(t, merged.ClaudePlatformAWS)
		require.Equal(t, patch.Bedrock, merged.Bedrock)
	})

	t.Run("EmptyPatchClears", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			Region:      "us-east-1",
			WorkspaceID: "stored-workspace",
		}}
		require.True(t, mergeAIProviderSettings(existing, codersdk.AIProviderSettings{}).IsZero())
	})
}

// TestAIProviderUsesAmbientCredentials pins which settings variants can
// authenticate without a stored key, which drives has_effective_api_key and
// the chat model picker.
func TestAIProviderUsesAmbientCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		settings codersdk.AIProviderSettings
		want     bool
	}{
		{
			name: "NoSettings",
			want: false,
		},
		{
			name:     "Bedrock",
			settings: codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}},
			want:     true,
		},
		{
			name:     "ClaudePlatform",
			settings: codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{}},
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, aiProviderUsesAmbientCredentials(tc.settings))
		})
	}
}
