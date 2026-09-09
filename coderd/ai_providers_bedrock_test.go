package coderd_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
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

func resolvedModels(ctx context.Context, t *testing.T, db database.Store, providerID uuid.UUID) []database.AIProviderBedrockResolvedModel {
	t.Helper()

	rows, err := db.GetAIProviderBedrockResolvedModelsByProviderIDs(ctx, []uuid.UUID{providerID})
	require.NoError(t, err)
	return rows
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

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		require.Len(t, paths(), 2, "each profile is resolved once")

		rows := resolvedModels(ctx, t, db, created.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "anthropic.claude-opus-4-8", rows[0].ResolvedModel)
		require.Equal(t, "anthropic.claude-haiku-4-5", rows[0].ResolvedSmallFastModel)
	})

	t.Run("CreateLeavesPlainModelIDsUnresolved", func(t *testing.T) {
		url, paths := mockBedrock(t, func(http.ResponseWriter, *http.Request) {
			t.Error("Bedrock called for plain model ids")
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		require.Empty(t, paths())
		require.Empty(t, resolvedModels(ctx, t, db, created.ID), "plain model ids are already model identities")
	})

	t.Run("CreateRejectsUnresolvableProfile", func(t *testing.T) {
		url, _ := mockBedrock(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"not authorized to perform bedrock:GetInferenceProfile"}`))
		})
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		require.Empty(t, resolvedModels(ctx, t, db, providers[0].ID))
	})

	t.Run("UpdateReresolvesChangedProfile", func(t *testing.T) {
		url, _ := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		require.Empty(t, resolvedModels(ctx, t, db, created.ID))

		//nolint:gocritic // Owner role is the audience for this endpoint.
		updated, err := client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Settings: bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Equal(t, testProfileARN, updated.Settings.Bedrock.Model)

		rows := resolvedModels(ctx, t, db, created.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "anthropic.claude-opus-4-8", rows[0].ResolvedModel)
		// The small/fast model is a plain ID, so it resolves to itself.
		require.Equal(t, "anthropic.claude-haiku-4-5", rows[0].ResolvedSmallFastModel)
	})

	t.Run("UpdateClearsResolutionWhenProfileReplacedByModelID", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		require.Len(t, resolvedModels(ctx, t, db, created.ID), 1)
		callsAfterCreate := len(paths())

		//nolint:gocritic // Owner role is the audience for this endpoint.
		_, err = client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Settings: bedrockSettings("eu.anthropic.claude-opus-4-8", "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Empty(t, resolvedModels(ctx, t, db, created.ID), "a plain model id needs no mapping")
		require.Len(t, paths(), callsAfterCreate, "no profile is left to resolve")
	})

	t.Run("UpdateWithoutSettingsKeepsResolution", func(t *testing.T) {
		url, paths := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
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
		_, err = client.UpdateAIProvider(ctx, created.ID.String(), codersdk.UpdateAIProviderRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)

		rows := resolvedModels(ctx, t, db, created.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "anthropic.claude-opus-4-8", rows[0].ResolvedModel)
		require.Len(t, paths(), callsAfterCreate, "an unrelated update does not call AWS")
	})

	t.Run("DeleteClearsResolution", func(t *testing.T) {
		url, _ := mockBedrock(t, respondWithModel("arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-8"))
		t.Setenv("AWS_ENDPOINT_URL_BEDROCK", url)

		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is the audience for this endpoint.
		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Name:     "bedrock-delete",
			Type:     codersdk.AIProviderTypeBedrock,
			BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
			Enabled:  true,
			Settings: *bedrockSettings(testProfileARN, "anthropic.claude-haiku-4-5"),
		})
		require.NoError(t, err)
		require.Len(t, resolvedModels(ctx, t, db, created.ID), 1)

		// Providers are soft-deleted, so the mapping has to be removed
		// explicitly rather than by the foreign key.
		//nolint:gocritic // Owner role is the audience for this endpoint.
		require.NoError(t, client.DeleteAIProvider(ctx, created.ID.String()))
		require.Empty(t, resolvedModels(ctx, t, db, created.ID))
	})
}
