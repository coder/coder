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
		ensureAIProviderExternalID(&s)
		require.Nil(t, s.Bedrock)
	})

	t.Run("NoRoleLeavesEmpty", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}}
		ensureAIProviderExternalID(&s)
		require.Empty(t, s.Bedrock.ExternalID)
	})

	t.Run("GeneratesWhenRoleSet", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			RoleARN: "arn:aws:iam::123456789012:role/BedrockRole",
		}}
		ensureAIProviderExternalID(&s)
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
		ensureAIProviderExternalID(&s)
		require.Equal(t, "existing-value", s.Bedrock.ExternalID)
	})

	t.Run("GeneratesUniqueValues", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]struct{})
		for range 10 {
			s := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
				RoleARN: "arn:aws:iam::123456789012:role/BedrockRole",
			}}
			ensureAIProviderExternalID(&s)
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

// TestMergeAIProviderSettingsClaudePlatform verifies the PATCH merge semantics
// for the Claude Platform variant: write-only credentials and the server-owned
// external ID are carried forward when omitted, and switching variants
// replaces rather than merges.
func TestMergeAIProviderSettingsClaudePlatform(t *testing.T) {
	t.Parallel()

	stored := func() codersdk.AIProviderSettings {
		return codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:        codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:          "us-east-1",
			WorkspaceID:     "wrkspc_stored",
			AccessKey:       new("AKIA-stored"), //nolint:gosec // fixture
			AccessKeySecret: new("secret-stored"),
			RoleARN:         "arn:aws:iam::123456789012:role/ClaudeRole",
			ExternalID:      "stored-value",
		}}
	}

	t.Run("OmittedCredentialsAreCarriedForward", func(t *testing.T) {
		t.Parallel()
		patch := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:    codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:      "us-west-2",
			WorkspaceID: "wrkspc_patched",
			RoleARN:     "arn:aws:iam::123456789012:role/ClaudeRole",
		}}
		merged := mergeAIProviderSettings(stored(), patch)
		require.NotNil(t, merged.ClaudePlatformAWS)
		require.Equal(t, "us-west-2", merged.ClaudePlatformAWS.Region)
		require.Equal(t, "wrkspc_patched", merged.ClaudePlatformAWS.WorkspaceID)
		require.Equal(t, "AKIA-stored", *merged.ClaudePlatformAWS.AccessKey)
		require.Equal(t, "secret-stored", *merged.ClaudePlatformAWS.AccessKeySecret)
	})

	// A pointer to the empty string is an explicit clear, e.g. migrating from
	// static credentials to the ambient AWS credential chain.
	t.Run("ExplicitClearWins", func(t *testing.T) {
		t.Parallel()
		patch := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:        codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:          "us-east-1",
			WorkspaceID:     "wrkspc_stored",
			AccessKey:       new(""),
			AccessKeySecret: new(""),
		}}
		merged := mergeAIProviderSettings(stored(), patch)
		require.NotNil(t, merged.ClaudePlatformAWS)
		require.Equal(t, "", *merged.ClaudePlatformAWS.AccessKey)
		require.Equal(t, "", *merged.ClaudePlatformAWS.AccessKeySecret)
	})

	t.Run("ExternalIDIsServerOwned", func(t *testing.T) {
		t.Parallel()
		patch := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:    codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:      "us-east-1",
			WorkspaceID: "wrkspc_stored",
			RoleARN:     "arn:aws:iam::123456789012:role/ClaudeRole",
			ExternalID:  "client-supplied-value",
		}}
		require.Error(t, validateAIProviderExternalIDUnchanged(stored(), patch))

		echoed := patch
		echoed.ClaudePlatformAWS.ExternalID = "stored-value"
		require.NoError(t, validateAIProviderExternalIDUnchanged(stored(), echoed))

		merged := mergeAIProviderSettings(stored(), echoed)
		require.Equal(t, "stored-value", merged.ClaudePlatformAWS.ExternalID)
	})

	// Switching authentication method is a full reconfiguration, so no field
	// from the previous variant survives.
	t.Run("SwitchingVariantsReplaces", func(t *testing.T) {
		t.Parallel()
		patch := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:         "us-east-1",
			Model:          "anthropic.claude-sonnet-4-5",
			SmallFastModel: "anthropic.claude-haiku-4-5",
		}}
		merged := mergeAIProviderSettings(stored(), patch)
		require.Nil(t, merged.ClaudePlatformAWS)
		require.NotNil(t, merged.Bedrock)
		require.Nil(t, merged.Bedrock.AccessKey)
	})

	t.Run("EmptyPatchClears", func(t *testing.T) {
		t.Parallel()
		merged := mergeAIProviderSettings(stored(), codersdk.AIProviderSettings{})
		require.True(t, merged.IsZero())
	})
}

// TestEnsureAIProviderExternalIDClaudePlatform covers external ID generation
// for the Claude Platform variant, which mirrors the Bedrock contract.
func TestEnsureAIProviderExternalIDClaudePlatform(t *testing.T) {
	t.Parallel()

	t.Run("NoRoleLeavesEmpty", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:    codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:      "us-east-1",
			WorkspaceID: "wrkspc_123",
		}}
		ensureAIProviderExternalID(&s)
		require.Empty(t, s.ClaudePlatformAWS.ExternalID)
	})

	t.Run("GeneratesWhenRoleSet", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:    codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:      "us-east-1",
			WorkspaceID: "wrkspc_123",
			RoleARN:     "arn:aws:iam::123456789012:role/ClaudeRole",
		}}
		ensureAIProviderExternalID(&s)
		require.NotEmpty(t, s.ClaudePlatformAWS.ExternalID)
	})

	t.Run("DoesNotOverwriteExisting", func(t *testing.T) {
		t.Parallel()
		s := codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
			AuthMode:    codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			Region:      "us-east-1",
			WorkspaceID: "wrkspc_123",
			RoleARN:     "arn:aws:iam::123456789012:role/ClaudeRole",
			ExternalID:  "existing-value",
		}}
		ensureAIProviderExternalID(&s)
		require.Equal(t, "existing-value", s.ClaudePlatformAWS.ExternalID)
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
			name: "ClaudePlatformIAM",
			settings: codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
				AuthMode: codersdk.AIProviderClaudePlatformAWSAuthModeIAM,
			}},
			want: true,
		},
		{
			// api_key mode is key-driven, exactly like plain Anthropic.
			name: "ClaudePlatformAPIKey",
			settings: codersdk.AIProviderSettings{ClaudePlatformAWS: &codersdk.AIProviderClaudePlatformAWSSettings{
				AuthMode: codersdk.AIProviderClaudePlatformAWSAuthModeAPIKey,
			}},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, aiProviderUsesAmbientCredentials(tc.settings))
		})
	}
}
