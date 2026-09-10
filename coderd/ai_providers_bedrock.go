package coderd

import (
	"context"
	"database/sql"
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
// inference profile ARNs refers to, returning the updated provider. It runs
// after the write commits, and on every save, because it calls AWS.
func (api *API) resolveBedrockModels(ctx context.Context, row database.AIProvider) (database.AIProvider, error) {
	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return row, xerrors.Errorf("decode settings: %w", err)
	}
	// BaseURL is the runtime endpoint; resolution calls the control plane.
	cfg := agplaibridge.BedrockConfig("", settings.Bedrock)
	if cfg == nil {
		return row, nil
	}

	resolved, err := provider.ResolveBedrockModels(ctx, *cfg)
	if err != nil {
		return row, xerrors.Errorf("resolve bedrock inference profile: %w", err)
	}
	if len(resolved) == 0 {
		return row, nil
	}
	settings.Bedrock.ResolvedModel = resolved[settings.Bedrock.Model]
	settings.Bedrock.ResolvedSmallFastModel = resolved[settings.Bedrock.SmallFastModel]

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

// clearBedrockModelResolution drops resolved identifiers a client supplied or
// an earlier save stored. The values are server-owned and rewritten after the
// write, so anything present beforehand is stale or forged.
func clearBedrockModelResolution(settings *codersdk.AIProviderSettings) {
	if settings.Bedrock == nil {
		return
	}
	settings.Bedrock.ResolvedModel = ""
	settings.Bedrock.ResolvedSmallFastModel = ""
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
