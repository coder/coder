package coderd_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	testProfileARN          = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/46u2vhiyo6z5"
	testSmallFastProfileARN = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/8x1qk20fzp3r"
)

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

// mockBedrock serves the Bedrock control-plane API and records the profile
// lookups it receives. Callers point the AWS SDK at the returned URL.
func mockBedrock(t *testing.T, handler http.HandlerFunc) (url string, paths func() []string) {
	t.Helper()

	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Path)
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(got)
	}
}

func respondWithModel(modelARN string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"modelArn":"` + modelARN + `"}]}`))
	}
}

// TestAIProvidersBedrockProfileResolution drives provider writes against a mock
// Bedrock control plane, so the AWS SDK path runs for real.
// NOTE: no t.Parallel() because the subtests use t.Setenv.
func TestAIProvidersBedrockProfileResolution(t *testing.T) {
	t.Run("CreateStoresResolvedModels", func(t *testing.T) {
		url, paths := mockBedrock(t, func(w http.ResponseWriter, r *http.Request) {
			modelARN := "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"
			if !strings.Contains(r.URL.Path, "46u2vhiyo6z5") {
				modelARN = "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-haiku-4-5"
			}
			respondWithModel(modelARN)(w, r)
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		require.Len(t, paths(), 2, "each profile is resolved once")
	})

	t.Run("CreateLeavesPlainModelIDsUnresolved", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called for plain model ids")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		require.Empty(t, paths())
	})

	t.Run("CreateRejectsUnresolvableProfile", func(t *testing.T) {
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"not authorized to perform bedrock:GetInferenceProfile"}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Detail, "GetInferenceProfile")

		// The provider is stored with the ARN the operator asked for, but
		// without a resolution the gateway refuses to serve it.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		providers, err := client.AIProviders(ctx)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		require.Empty(t, providers[0].Settings.Bedrock.ResolvedModel)
	})

	t.Run("CreateRejectsClientSuppliedResolution", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called for a rejected request")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Error(), "resolved_model")
		require.Empty(t, paths(), "validation rejects the write before any AWS call")
	})

	t.Run("UpdateReresolvesChangedProfile", func(t *testing.T) {
		url, _ := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		callsAfterCreate := len(paths())

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Settings: bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Empty(t, updated.Settings.Bedrock.ResolvedModel, "a plain model id resolves to itself")
		require.Len(t, paths(), callsAfterCreate, "no profile is left to resolve")
	})

	t.Run("UpdateWithoutSettingsSkipsResolution", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
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
		callsAfterCreate := len(paths())

		enabled := false
		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, "anthropic.claude-opus-4-8", updated.Settings.Bedrock.ResolvedModel)
		require.Len(t, paths(), callsAfterCreate, "an unrelated update does not call AWS")
	})
}
