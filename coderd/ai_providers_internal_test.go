package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestMergeAIProviderSettings(t *testing.T) {
	t.Parallel()

	t.Run("OmittedSecretsPreserveExisting", func(t *testing.T) {
		t.Parallel()
		// A PATCH that only rotates non-secret fields must keep the
		// existing AccessKey and AccessKeySecret intact so the provider
		// keeps authenticating after the update.
		existing := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				Model:           "anthropic.claude-3-5-sonnet",
				AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture
				AccessKeySecret: ptr.Ref("secret-old"),
			},
		}
		patch := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region: "us-west-2",
				Model:  "anthropic.claude-3-5-haiku",
			},
		}
		merged := mergeAIProviderSettings(existing, patch)
		require.NotNil(t, merged.Bedrock)
		require.Equal(t, "us-west-2", merged.Bedrock.Region)
		require.Equal(t, "anthropic.claude-3-5-haiku", merged.Bedrock.Model)
		require.NotNil(t, merged.Bedrock.AccessKey)
		require.Equal(t, "AKIA-old", *merged.Bedrock.AccessKey)
		require.NotNil(t, merged.Bedrock.AccessKeySecret)
		require.Equal(t, "secret-old", *merged.Bedrock.AccessKeySecret)
	})

	t.Run("ExplicitEmptyClearsSecrets", func(t *testing.T) {
		t.Parallel()
		// An admin migrating from static AWS credentials to IAM
		// role-based auth needs to clear AccessKey and AccessKeySecret
		// in a single PATCH. Sending the field with an empty string is
		// the explicit clear signal.
		existing := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture
				AccessKeySecret: ptr.Ref("secret-old"),
			},
		}
		patch := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				AccessKey:       ptr.Ref(""),
				AccessKeySecret: ptr.Ref(""),
			},
		}
		merged := mergeAIProviderSettings(existing, patch)
		require.NotNil(t, merged.Bedrock)
		require.NotNil(t, merged.Bedrock.AccessKey)
		require.Equal(t, "", *merged.Bedrock.AccessKey)
		require.NotNil(t, merged.Bedrock.AccessKeySecret)
		require.Equal(t, "", *merged.Bedrock.AccessKeySecret)
	})

	t.Run("ExplicitRotatesSecrets", func(t *testing.T) {
		t.Parallel()
		existing := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				AccessKey:       ptr.Ref("AKIA-old"), //nolint:gosec // test fixture
				AccessKeySecret: ptr.Ref("secret-old"),
			},
		}
		patch := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				AccessKey:       ptr.Ref("AKIA-new"), //nolint:gosec // test fixture
				AccessKeySecret: ptr.Ref("secret-new"),
			},
		}
		merged := mergeAIProviderSettings(existing, patch)
		require.NotNil(t, merged.Bedrock)
		require.Equal(t, "AKIA-new", *merged.Bedrock.AccessKey)
		require.Equal(t, "secret-new", *merged.Bedrock.AccessKeySecret)
	})

	t.Run("NilPatchClearsSettings", func(t *testing.T) {
		t.Parallel()
		// patch.Bedrock == nil means the caller is dropping all
		// type-specific configuration (typically when sending a typed
		// patch against a provider whose type was switched).
		existing := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
		}
		merged := mergeAIProviderSettings(existing, codersdk.AIProviderSettings{})
		require.True(t, merged.IsZero())
	})
}
