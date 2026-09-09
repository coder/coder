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

// bedrockProfileUnresolvableError marks a failed Bedrock inference profile
// lookup so the write path reports it as a client-visible validation failure
// rather than an internal error.
type bedrockProfileUnresolvableError struct{ err error }

func (e bedrockProfileUnresolvableError) Error() string {
	return "resolve bedrock inference profile: " + e.err.Error()
}

func (e bedrockProfileUnresolvableError) Unwrap() error { return e.err }

// resolveBedrockModels records which model each of the provider's application
// inference profile ARNs refers to. An ARN identifies a billing wrapper rather
// than a model, so the gateway needs the mapping to detect capabilities, price
// usage, and record interceptions.
//
// Resolution is an AWS call, so it runs after the provider write has committed
// rather than holding a database transaction open across the network. It reads
// the stored row because that is the merged configuration the provider will
// actually use.
//
// It runs on every save, even for an ARN another provider already resolved, so
// that saving proves this provider's own identity can read the profile.
func (api *API) resolveBedrockModels(ctx context.Context, row database.AIProvider) error {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return xerrors.Errorf("decode settings: %w", err)
	}
	// Resolution is a Bedrock control-plane call, whereas the provider's
	// BaseURL is its runtime endpoint, so it is deliberately not carried here.
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return nil
	}

	resolved, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return bedrockProfileUnresolvableError{err: err}
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

// writeAIProviderResolutionError reports a failed Bedrock model resolution. The
// provider keeps the identifiers the operator asked for, but without a
// resolution the gateway cannot tell what an opaque profile ARN refers to, so
// it serves the ARN as its own identity until a later save resolves it.
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
