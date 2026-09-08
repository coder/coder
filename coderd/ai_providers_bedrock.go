package coderd

import (
	"context"
	"database/sql"
	"errors"
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

// bedrockProfileUnresolvableError marks a failed Bedrock inference profile lookup
// so the write path reports it as a client-visible validation failure rather
// than an internal error.
type bedrockProfileUnresolvableError struct{ err error }

func (e bedrockProfileUnresolvableError) Error() string {
	return "resolve bedrock inference profile: " + e.err.Error()
}

func (e bedrockProfileUnresolvableError) Unwrap() error { return e.err }

// resolveBedrockModels fills in the server-owned resolved identifiers on
// settings. Only application inference profile ARNs are opaque, so only they
// cost an AWS call; every other identifier resolves to itself and is stored
// unresolved.
//
// Resolution runs here, where the provider is written, so the gateway never
// calls the Bedrock control plane: not at startup, not on reload, and not on a
// request.
func resolveBedrockModels(ctx context.Context, settings *codersdk.AIProviderSettings) error {
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return nil
	}

	model, smallFastModel, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return bedrockProfileUnresolvableError{err: err}
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

// applyBedrockResolution resolves the inference profile ARNs of a provider that
// was just written and stores the result on it. Resolution is an AWS call, so
// it runs after the write transaction has committed rather than holding a
// database connection open across the network.
//
// The provider row is the merged settings the operator will actually use, which
// is why resolution reads it back instead of the request.
func (api *API) applyBedrockResolution(ctx context.Context, row database.AIProvider) (database.AIProvider, error) {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return row, xerrors.Errorf("decode settings: %w", err)
	}
	if settings.Bedrock == nil {
		return row, nil
	}

	stored := *settings.Bedrock
	if err := resolveBedrockModels(ctx, &settings); err != nil {
		return row, err
	}
	if settings.Bedrock.ResolvedModel == stored.ResolvedModel &&
		settings.Bedrock.ResolvedSmallFastModel == stored.ResolvedSmallFastModel {
		return row, nil
	}

	encoded, err := encodeAIProviderSettings(settings)
	if err != nil {
		return row, xerrors.Errorf("encode settings: %w", err)
	}
	updated, err := api.Database.UpdateAIProvider(ctx, database.UpdateAIProviderParams{
		ID:          row.ID,
		Type:        row.Type,
		DisplayName: row.DisplayName,
		Icon:        row.Icon,
		Enabled:     row.Enabled,
		BaseUrl:     row.BaseUrl,
		Settings:    encoded,
		// SettingsKeyID is set by the dbcrypt wrapper.
		SettingsKeyID: sql.NullString{},
	})
	if err != nil {
		return row, xerrors.Errorf("store resolved models: %w", err)
	}
	return updated, nil
}

// writeAIProviderResolutionError reports a failed Bedrock model resolution. The
// provider keeps the identifiers the operator asked for, but without a
// resolution the gateway cannot tell what an opaque profile ARN refers to, so
// it refuses to serve the provider until the write succeeds.
func (api *API) writeAIProviderResolutionError(ctx context.Context, rw http.ResponseWriter, err error) {
	var unresolvable bedrockProfileUnresolvableError
	if !errors.As(err, &unresolvable) {
		writeAIProviderError(ctx, api.Logger, rw, err, "resolve bedrock inference profile", "Internal error resolving the Bedrock application inference profile.")
		return
	}
	api.Logger.Warn(ctx, "resolve bedrock inference profile", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Could not resolve the Bedrock application inference profile. Check that the ARN is correct and that the AWS identity used by Coder is allowed bedrock:GetInferenceProfile.",
		Detail:  err.Error(),
	})
}
