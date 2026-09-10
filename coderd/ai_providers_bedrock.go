package coderd

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/provider"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// resolveBedrockProfiles asks AWS which model each application inference
// profile ARN in settings refers to. The result is empty when no identifier is
// an ARN, which costs no AWS call.
func resolveBedrockProfiles(ctx context.Context, settings codersdk.AIProviderSettings) (map[string]string, error) {
	resolved := map[string]string{}
	// BaseURL configures the runtime data-plane endpoint. GetInferenceProfile
	// uses the Bedrock control-plane endpoint derived from Region instead.
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return resolved, nil
	}
	resolved, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return nil, xerrors.Errorf("resolve bedrock inference profile: %w", err)
	}
	return resolved, nil
}

// applyBedrockResolution records what the configured identifiers refer to. An
// identifier that is not an application inference profile ARN is its own
// identity and stores nothing, which also discards any value a client supplied.
func applyBedrockResolution(settings *codersdk.AIProviderSettings, resolved map[string]string) {
	if settings.Bedrock == nil {
		return
	}
	settings.Bedrock.ResolvedModel = resolved[settings.Bedrock.Model]
	settings.Bedrock.ResolvedSmallFastModel = resolved[settings.Bedrock.SmallFastModel]
}

// writeAIProviderResolutionError reports a failed resolution. The write is
// rejected, because a stored ARN with no resolution would be served as its own
// identity and misshape every request made through it.
func (api *API) writeAIProviderResolutionError(ctx context.Context, rw http.ResponseWriter, err error) {
	api.Logger.Warn(ctx, "resolve bedrock inference profile", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Could not resolve the Bedrock application inference profile. Check that the ARN is correct and that the AWS identity used by Coder is allowed bedrock:GetInferenceProfile.",
		Detail:  err.Error(),
	})
}
