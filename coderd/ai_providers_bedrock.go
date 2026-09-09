package coderd

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/provider"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// bedrockProfileUnresolvableError marks a failed Bedrock inference profile
// lookup so the write path reports it as a client-visible validation failure
// rather than an internal error.
type bedrockProfileUnresolvableError struct{ err error }

func (e bedrockProfileUnresolvableError) Error() string {
	return "resolve bedrock inference profile: " + e.err.Error()
}

func (e bedrockProfileUnresolvableError) Unwrap() error { return e.err }

// resolveBedrockModels records which models the provider's application
// inference profile ARNs refer to. An ARN identifies a billing wrapper rather
// than a model, so the gateway needs the mapping to detect capabilities, price
// usage, and record interceptions.
//
// Resolution is an AWS call, so it runs after the provider write has committed
// rather than holding a database transaction open across the network. It reads
// the stored row because that is the merged configuration the provider will
// actually use.
//
// Nothing is stored for a provider configured with plain model IDs: they are
// already model identities and need no mapping.
func (api *API) resolveBedrockModels(ctx context.Context, row database.AIProvider) error {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return xerrors.Errorf("decode settings: %w", err)
	}
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return nil
	}

	model, smallFastModel, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return bedrockProfileUnresolvableError{err: err}
	}
	if model == cfg.Model && smallFastModel == cfg.SmallFastModel {
		return nil
	}

	err = api.Database.UpsertAIProviderBedrockResolvedModels(ctx, database.UpsertAIProviderBedrockResolvedModelsParams{
		AIProviderID:           row.ID,
		ResolvedModel:          model,
		ResolvedSmallFastModel: smallFastModel,
	})
	if err != nil {
		return xerrors.Errorf("store resolved models: %w", err)
	}
	return nil
}

// clearBedrockModelResolution drops a provider's stored resolution. The write
// path calls it whenever the configured identifiers may have changed, so a
// stale mapping never outlives the ARN it describes. The provider is then
// unresolved until resolution succeeds, and the gateway will not serve it.
func clearBedrockModelResolution(ctx context.Context, db database.Store, providerID uuid.UUID) error {
	if err := db.DeleteAIProviderBedrockResolvedModels(ctx, providerID); err != nil {
		return xerrors.Errorf("clear resolved models: %w", err)
	}
	return nil
}

// writeAIProviderResolutionError reports a failed Bedrock model resolution. The
// provider keeps the identifiers the operator asked for, but without a
// resolution the gateway cannot tell what an opaque profile ARN refers to, so
// it refuses to serve the provider until a later save resolves it.
func (api *API) writeAIProviderResolutionError(ctx context.Context, rw http.ResponseWriter, err error) {
	var unresolvable bedrockProfileUnresolvableError
	if !xerrors.As(err, &unresolvable) {
		writeAIProviderError(ctx, api.Logger, rw, err, "resolve bedrock inference profile", "Internal error resolving the Bedrock application inference profile.")
		return
	}
	api.Logger.Warn(ctx, "resolve bedrock inference profile", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Could not resolve the Bedrock application inference profile. Check that the ARN is correct and that the AWS identity used by Coder is allowed bedrock:GetInferenceProfile.",
		Detail:  err.Error(),
	})
}
