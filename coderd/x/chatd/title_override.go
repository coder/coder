package chatd

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const titleGenerationOverrideContext = "title_generation"

func readTitleGenerationModelOverride(
	ctx context.Context,
	db database.Store,
) (modelConfigID string, isMalformed bool, err error) {
	//nolint:gocritic // Chatd needs scoped deployment-config read access here.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatTitleGenerationModelOverride(chatdCtx)
	if err != nil {
		return "", false, err
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, nil
	}
	modelConfigUUID, err := uuid.Parse(trimmed)
	if err != nil {
		return "", true, nil
	}
	return modelConfigUUID.String(), false, nil
}

func (p *Server) resolveTitleGenerationModelConfig(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (database.ChatModelConfig, fantasy.LanguageModel, bool, error) {
	modelConfigID, malformed, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}
	if malformed {
		p.logger.Info(ctx,
			"invalid model override, ignoring",
			slog.F("override_context", titleGenerationOverrideContext),
		)
		return database.ChatModelConfig{}, nil, false, nil
	}
	if modelConfigID == "" {
		return database.ChatModelConfig{}, nil, false, nil
	}

	configuredModelConfigID, err := uuid.Parse(modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, nil, false, xerrors.Errorf(
			"parse normalized title generation model override: %w",
			err,
		)
	}
	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(
		ctx,
		configuredModelConfigID,
	)
	if err != nil {
		switch {
		case xerrors.Is(err, sql.ErrNoRows):
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"title generation model override is unavailable: %s",
				configuredModelConfigID,
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"title generation model override metadata is invalid for %s: %w",
				configuredModelConfigID,
				err,
			)
		default:
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"resolve title generation model override %s: %w",
				configuredModelConfigID,
				err,
			)
		}
	}

	if keys.APIKey(providerName) == "" &&
		!(chatprovider.ProviderAllowsAmbientCredentials(providerName) &&
			keys.HasProvider(providerName)) {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"title generation model override credentials are unavailable for provider %q",
			providerName,
		)
	}

	model, err := chatprovider.ModelFromConfig(
		modelConfig.Provider,
		modelConfig.Model,
		keys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
		nil,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	if model == nil {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"create title generation model override returned nil",
		)
	}

	return modelConfig, model, true, nil
}
