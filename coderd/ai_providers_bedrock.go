package coderd

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/provider"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// resolveBedrockModels stores the model each of the provider's application
// inference profile ARNs refers to. It runs after the write commits, and on
// every save, because it calls AWS.
func (api *API) resolveBedrockModels(ctx context.Context, row database.AIProvider) error {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return xerrors.Errorf("decode settings: %w", err)
	}
	// BaseURL is the runtime endpoint; resolution calls the control plane.
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return nil
	}

	resolved, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return xerrors.Errorf("resolve bedrock inference profile: %w", err)
	}

	for profileARN, model := range resolved {
		err := api.Database.UpsertAIBedrockInferenceProfileModel(ctx, database.UpsertAIBedrockInferenceProfileModelParams{
			InferenceProfileArn: profileARN,
			ResolvedModel:       model,
		})
		if err != nil {
			return xerrors.Errorf("store resolved model for %q: %w", profileARN, err)
		}
	}
	return nil
}

// writeAIProviderResolutionError reports a failed resolution. The provider is
// stored either way, and serves the ARN as its own identity until a later save
// resolves it.
func (api *API) writeAIProviderResolutionError(ctx context.Context, rw http.ResponseWriter, err error) {
	api.Logger.Warn(ctx, "resolve bedrock inference profile", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Could not resolve the Bedrock application inference profile. Check that the ARN is correct and that the AWS identity used by Coder is allowed bedrock:GetInferenceProfile.",
		Detail:  err.Error(),
	})
}
