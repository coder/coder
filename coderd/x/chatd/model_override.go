package chatd

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const (
	titleGenerationOverrideContext = "title_generation"
	compactionOverrideContext      = "compaction"
	advisorOverrideContext         = "advisor"
)

var (
	errInvalidModelOverrideMetadata   = xerrors.New("invalid model override metadata")
	errModelConfigOutsideOrganization = xerrors.Errorf("%w: model config belongs to another organization", sql.ErrNoRows)
)

type modelOverrideFailureMode int

const (
	modelOverrideFailureModeSoft modelOverrideFailureMode = iota
	modelOverrideFailureModeHard
)

type modelOverrideSpec struct {
	context           string
	ownerID           uuid.UUID
	organizationID    uuid.UUID
	queryFailure      modelOverrideFailureMode
	configFailure     modelOverrideFailureMode
	credentialFailure modelOverrideFailureMode
}

type resolvedModelOverride struct {
	Config           database.ChatModelConfig
	ReasoningEffort  *string
	ResolvedProvider string
	ResolvedModel    string
	Set              bool
}

func (p *Server) resolveModelOverride(ctx context.Context, spec modelOverrideSpec) (resolvedModelOverride, error) {
	//nolint:gocritic // Chatd reads organization-scoped runtime configuration.
	override, err := p.db.GetChatOrganizationModelOverride(
		dbauthz.AsChatd(ctx),
		database.GetChatOrganizationModelOverrideParams{
			OrganizationID: spec.organizationID,
			Context:        spec.context,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return resolvedModelOverride{}, nil
		}
		if spec.queryFailure == modelOverrideFailureModeSoft {
			p.logger.Warn(ctx,
				"failed to load model override, ignoring",
				slog.F("override_context", spec.context),
				slog.F("organization_id", spec.organizationID),
				slog.Error(err),
			)
			return resolvedModelOverride{}, nil
		}
		return resolvedModelOverride{}, xerrors.Errorf(
			"get %s model override: %w",
			modelOverrideErrorLabel(spec.context),
			err,
		)
	}

	resolved := resolvedModelOverride{
		ReasoningEffort: ptr.FromNullString(override.ReasoningEffort),
		Set:             true,
	}
	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(
		ctx,
		spec.ownerID,
		override.ModelConfigID,
	)
	if err != nil {
		if spec.configFailure == modelOverrideFailureModeHard {
			label := modelOverrideErrorLabel(spec.context)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return resolved, xerrors.Errorf(
					"%s model override is unavailable: %s",
					label,
					override.ModelConfigID,
				)
			case errors.Is(err, errInvalidModelOverrideMetadata):
				return resolved, xerrors.Errorf(
					"%s model override metadata is invalid for %s: %w",
					label,
					override.ModelConfigID,
					err,
				)
			default:
				return resolved, xerrors.Errorf(
					"resolve %s model override %s: %w",
					label,
					override.ModelConfigID,
					err,
				)
			}
		}

		switch {
		case errors.Is(err, sql.ErrNoRows):
			p.logger.Info(ctx,
				"model override is unavailable, ignoring",
				slog.F("override_context", spec.context),
				slog.F("model_config_id", override.ModelConfigID),
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			p.logger.Info(ctx,
				"model override metadata is invalid, ignoring",
				slog.F("override_context", spec.context),
				slog.F("model_config_id", override.ModelConfigID),
				slog.Error(err),
			)
		default:
			p.logger.Warn(ctx,
				"failed to resolve model override, ignoring",
				slog.F("override_context", spec.context),
				slog.F("model_config_id", override.ModelConfigID),
				slog.Error(err),
			)
		}
		return resolvedModelOverride{}, nil
	}

	providerKeys, err := p.resolveUserProviderAPIKeys(ctx, spec.ownerID, modelConfigAIProviderID(modelConfig))
	if err != nil {
		return resolvedModelOverride{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	if !userCanUseProviderKeys(providerKeys, providerName) {
		if spec.credentialFailure == modelOverrideFailureModeHard {
			return resolved, xerrors.Errorf(
				"%s model override credentials are unavailable for provider %q",
				modelOverrideErrorLabel(spec.context),
				providerName,
			)
		}
		p.logger.Info(ctx,
			"model override credentials are unavailable, ignoring",
			slog.F("override_context", spec.context),
			slog.F("model_config_id", override.ModelConfigID),
			slog.F("provider", providerName),
		)
		return resolvedModelOverride{}, nil
	}

	resolvedProvider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(modelConfig.Model, providerName)
	if err != nil {
		return resolved, xerrors.Errorf("resolve %s model override identity: %w", modelOverrideErrorLabel(spec.context), err)
	}
	resolved.Config = modelConfig
	resolved.ResolvedProvider = resolvedProvider
	resolved.ResolvedModel = resolvedModel
	return resolved, nil
}

func modelOverrideErrorLabel(overrideContext string) string {
	return strings.ReplaceAll(overrideContext, "_", " ")
}

func userCanUseProviderKeys(providerKeys chatprovider.ProviderAPIKeys, providerName string) bool {
	return providerKeys.APIKey(providerName) != "" ||
		(chatprovider.ProviderAllowsAmbientCredentials(providerName) && providerKeys.HasProvider(providerName))
}

func modelConfigAIProviderID(modelConfig database.ChatModelConfig) uuid.UUID {
	if !modelConfig.AIProviderID.Valid {
		return uuid.Nil
	}
	return modelConfig.AIProviderID.UUID
}

func (p *Server) resolveModelConfigAndNormalizedProvider(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, string, error) {
	if modelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	modelCtx, err := p.callerModelConfigContext(ctx, ownerID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	modelConfig, err := p.db.GetChatModelConfigByID(modelCtx, modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	return p.resolveNormalizedProviderForModelConfig(ctx, modelConfig)
}

func (p *Server) resolveNormalizedProviderForModelConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (database.ChatModelConfig, string, error) {
	if !modelConfig.Enabled {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	if modelConfig.AIProviderID.Valid {
		//nolint:gocritic // Provider configuration remains a privileged Chatd read.
		provider, err := p.db.GetAIProviderByID(dbauthz.AsChatd(ctx), modelConfig.AIProviderID.UUID)
		if err != nil {
			return database.ChatModelConfig{}, "", err
		}
		if !provider.Enabled {
			return database.ChatModelConfig{}, "", sql.ErrNoRows
		}
		providerName := chatprovider.NormalizeProvider(string(provider.Type))
		if providerName == "" {
			return database.ChatModelConfig{}, "", errInvalidModelOverrideMetadata
		}
		if _, _, err := chatprovider.ResolveModelWithProviderHint(modelConfig.Model, providerName); err != nil {
			return database.ChatModelConfig{}, "", errInvalidModelOverrideMetadata
		}
		return modelConfig, providerName, nil
	}
	return database.ChatModelConfig{}, "", sql.ErrNoRows
}
