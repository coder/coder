package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	testProfileARN          = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/46u2vhiyo6z5"
	testSmallFastProfileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/8x1qk20fzp3r"
)

// stubBedrockResolver stands in for the AWS Bedrock control plane. It records
// the settings it was asked to resolve so tests can assert whether a write
// consulted AWS at all.
type stubBedrockResolver struct {
	models map[string]string
	err    error
	calls  []codersdk.AIProviderBedrockSettings
}

func (s *stubBedrockResolver) ResolveModels(_ context.Context, settings codersdk.AIProviderBedrockSettings) (model, smallFastModel string, err error) {
	s.calls = append(s.calls, settings)
	if s.err != nil {
		return "", "", s.err
	}
	resolve := func(configured string) string {
		if model, ok := s.models[configured]; ok {
			return model
		}
		return configured
	}
	return resolve(settings.Model), resolve(settings.SmallFastModel), nil
}

func bedrockSettings(model, smallFastModel string) *codersdk.AIProviderSettings {
	accessKey := "test-key"
	accessKeySecret := "test-secret"
	return &codersdk.AIProviderSettings{
		Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:          "us-east-1",
			AccessKey:       &accessKey,
			AccessKeySecret: &accessKeySecret,
			Model:           model,
			SmallFastModel:  smallFastModel,
		},
	}
}

func TestAIProvidersBedrockProfileResolution(t *testing.T) {
	t.Parallel()

	newClient := func(t *testing.T, resolver coderd.BedrockModelResolver) *codersdk.Client {
		t.Helper()

		client := coderdtest.New(t, &coderdtest.Options{AIProviderBedrockResolver: resolver})
		_ = coderdtest.CreateFirstUser(t, client)
		return client
	}

	t.Run("CreateStoresResolvedModels", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{models: map[string]string{
			testProfileARN:          "anthropic.claude-opus-4-8",
			testSmallFastProfileARN: "anthropic.claude-haiku-4-5",
		}}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-profiles",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings(testProfileARN, testSmallFastProfileARN),
		})
		require.NoError(t, err)
		require.NotNil(t, created.Settings.Bedrock)
		// The configured identifiers stay untouched: they remain the Bedrock
		// invocation target, and AWS attributes spend to them.
		require.Equal(t, testProfileARN, created.Settings.Bedrock.Model)
		require.Equal(t, testSmallFastProfileARN, created.Settings.Bedrock.SmallFastModel)
		require.Equal(t, "anthropic.claude-opus-4-8", created.Settings.Bedrock.ResolvedModel)
		require.Equal(t, "anthropic.claude-haiku-4-5", created.Settings.Bedrock.ResolvedSmallFastModel)
	})

	t.Run("CreateLeavesPlainModelIDsUnresolved", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-plain",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.NotNil(t, created.Settings.Bedrock)
		require.Empty(t, created.Settings.Bedrock.ResolvedModel)
		require.Empty(t, created.Settings.Bedrock.ResolvedSmallFastModel)
	})

	t.Run("CreateRejectsUnresolvableProfile", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{err: xerrors.New("AccessDeniedException: not authorized to perform bedrock:GetInferenceProfile")}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-denied",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 400, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Detail, "bedrock:GetInferenceProfile")

		//nolint:gocritic // Owner role is the audience for this endpoint.
		providers, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Empty(t, providers, "an unresolvable provider is not stored")
	})

	t.Run("CreateRejectsClientSuppliedResolution", func(t *testing.T) {
		t.Parallel()

		client := newClient(t, &stubBedrockResolver{})
		ctx := testutil.Context(t, testutil.WaitLong)

		settings := bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5")
		settings.Bedrock.ResolvedModel = "anthropic.claude-opus-4-8"

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-spoofed",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *settings,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 400, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Error(), "resolved_model")
	})

	t.Run("UpdateReresolvesChangedProfile", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{models: map[string]string{
			testProfileARN:          "anthropic.claude-opus-4-8",
			testSmallFastProfileARN: "anthropic.claude-haiku-4-5",
		}}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-update",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Empty(t, created.Settings.Bedrock.ResolvedModel)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Settings: bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Equal(t, testProfileARN, updated.Settings.Bedrock.Model)
		require.Equal(t, "anthropic.claude-opus-4-8", updated.Settings.Bedrock.ResolvedModel)
		require.Empty(t, updated.Settings.Bedrock.ResolvedSmallFastModel)
	})

	t.Run("UpdateClearsResolutionWhenProfileReplacedByModelID", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{models: map[string]string{
			testProfileARN: "anthropic.claude-opus-4-8",
		}}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-replace",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", created.Settings.Bedrock.ResolvedModel)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Settings: bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Empty(t, updated.Settings.Bedrock.ResolvedModel, "a plain model id resolves to itself")
	})

	t.Run("UpdateWithoutSettingsSkipsResolution", func(t *testing.T) {
		t.Parallel()

		resolver := &stubBedrockResolver{models: map[string]string{
			testProfileARN: "anthropic.claude-opus-4-8",
		}}
		client := newClient(t, resolver)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-keep",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		callsAfterCreate := len(resolver.calls)

		enabled := false
		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", updated.Settings.Bedrock.ResolvedModel)
		require.Len(t, resolver.calls, callsAfterCreate, "an unrelated update does not call AWS")
	})
}
