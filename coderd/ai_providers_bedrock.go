package coderd

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/provider"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// errAIProviderProfileUnresolvable wraps a failed Bedrock inference profile
// lookup so the write path reports it as a client-visible validation failure
// rather than an internal error.
var errAIProviderProfileUnresolvable = xerrors.New("resolve bedrock inference profile")

// BedrockModelResolver resolves the configured Bedrock model identifiers of a
// provider to the model IDs the gateway records for capability detection,
// usage, and pricing. Identifiers that are not application inference profile
// ARNs resolve to themselves without calling AWS.
//
// Resolution runs here, where the provider is written, so the gateway never
// calls the Bedrock control plane: not at startup, not on reload, and not on a
// request. It is an interface so tests can supply results without AWS.
type BedrockModelResolver interface {
	ResolveModels(ctx context.Context, settings codersdk.AIProviderBedrockSettings) (model, smallFastModel string, err error)
}

// awsBedrockModelResolver resolves through the AWS Bedrock control plane using
// the provider's own credentials, including any assumed role.
type awsBedrockModelResolver struct{}

// ResolveModels resolves the configured identifiers to the model IDs the
// gateway records for capability detection, usage, and pricing. Only
// application inference profile ARNs are opaque, so only they cost an AWS call;
// every other identifier resolves to itself.
func (awsBedrockModelResolver) ResolveModels(ctx context.Context, settings codersdk.AIProviderBedrockSettings) (model, smallFastModel string, err error) {
	cfg := agplaibridge.BedrockConfig("", &settings)
	if cfg == nil {
		return settings.Model, settings.SmallFastModel, nil
	}
	return provider.ResolveBedrockModels(ctx, *cfg)
}

func (api *API) bedrockModelResolver() BedrockModelResolver {
	if api.AIProviderBedrockResolver != nil {
		return api.AIProviderBedrockResolver
	}
	return awsBedrockModelResolver{}
}

// resolveBedrockModels fills in the server-owned resolved identifiers on
// settings. A resolved value is stored only when it differs from the configured
// one, so plain model IDs stay unresolved and keep serving themselves.
//
// A failure is returned to the caller: an unresolvable profile must not be
// stored, because the gateway cannot tell what an opaque ARN refers to and
// would misshape every request made through it.
func (api *API) resolveBedrockModels(ctx context.Context, settings *codersdk.AIProviderSettings) error {
	if settings.Bedrock == nil {
		return nil
	}

	model, smallFastModel, err := api.bedrockModelResolver().ResolveModels(ctx, *settings.Bedrock)
	if err != nil {
		return xerrors.Errorf("%w: %w", errAIProviderProfileUnresolvable, err)
	}

	settings.Bedrock.ResolvedModel = ""
	if model != settings.Bedrock.Model {
		settings.Bedrock.ResolvedModel = model
	}
	settings.Bedrock.ResolvedSmallFastModel = ""
	if smallFastModel != settings.Bedrock.SmallFastModel {
		settings.Bedrock.ResolvedSmallFastModel = smallFastModel
	}
	return nil
}

// bedrockModelsMatch reports whether two settings configure the same model
// identifiers. The update path resolves against a snapshot taken outside the
// transaction, so it re-checks the merged settings before storing the result.
func bedrockModelsMatch(a, b codersdk.AIProviderSettings) bool {
	if a.Bedrock == nil || b.Bedrock == nil {
		return a.Bedrock == b.Bedrock
	}
	return a.Bedrock.Model == b.Bedrock.Model && a.Bedrock.SmallFastModel == b.Bedrock.SmallFastModel
}

// previewResolvedBedrockSettings merges patch onto the stored settings of the
// named provider and resolves the result, so the write path can perform the
// AWS lookup outside its transaction. The boolean reports whether a preview was
// produced: there is nothing to resolve, or the provider cannot be read, in
// which case the transaction reports the failure with its own error handling.
func (api *API) previewResolvedBedrockSettings(ctx context.Context, idOrName string, patch *codersdk.AIProviderSettings) (codersdk.AIProviderSettings, bool, error) {
	if patch == nil || patch.Bedrock == nil {
		return codersdk.AIProviderSettings{}, false, nil
	}
	old, err := lookupAIProvider(ctx, api.Database, idOrName)
	if err != nil {
		//nolint:nilerr // The transaction reports lookup failures.
		return codersdk.AIProviderSettings{}, false, nil
	}
	existing, err := db2sdk.AIProviderSettings(old.Settings)
	if err != nil {
		//nolint:nilerr // The transaction reports decode failures.
		return codersdk.AIProviderSettings{}, false, nil
	}

	preview := mergeAIProviderSettings(existing, *patch)
	ensureBedrockExternalID(&preview)
	if err := api.resolveBedrockModels(ctx, &preview); err != nil {
		return codersdk.AIProviderSettings{}, false, err
	}
	return preview, true, nil
}

// writeAIProviderResolutionError reports a failed Bedrock model resolution. The
// write is rejected rather than stored unresolved: the gateway cannot serve an
// opaque profile ARN, so accepting it would produce a provider that fails every
// request.
func (api *API) writeAIProviderResolutionError(ctx context.Context, rw http.ResponseWriter, err error) {
	api.Logger.Warn(ctx, "resolve bedrock inference profile", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Could not resolve the Bedrock application inference profile. Check that the ARN is correct and that the AWS identity used by Coder is allowed bedrock:GetInferenceProfile.",
		Detail:  err.Error(),
	})
}
