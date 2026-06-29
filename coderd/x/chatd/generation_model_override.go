package chatd

import (
	"context"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// resolveGenerationModelOverride resolves a deployment-wide model override for a
// background generation feature (title or summary). overrideContext labels the
// override (and its error messages); readOverride loads the configured value.
// overrideSet is true when an override was configured; in that case any returned
// error is a hard failure and the caller should skip generation. When
// overrideSet is false, callers fall back to the chat's configured model.
func (p *Server) resolveGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
	modelOpts modelBuildOptions,
	overrideContext string,
	readOverride func(context.Context, database.Store) (string, error),
) (database.ChatModelConfig, fantasy.LanguageModel, chatprovider.ProviderAPIKeys, resolvedModelRoute, bool, error) {
	label := modelOverrideErrorLabel(overrideContext)
	raw, err := readOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, resolvedModelRoute{}, false, xerrors.Errorf(
			"read %s model override: %w",
			label,
			err,
		)
	}

	overrideProviderKeys := keys
	modelConfig, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		overrideContext,
		raw,
		chat.OwnerID,
		p.resolveModelConfigAndNormalizedProvider,
		func(ctx context.Context, ownerID uuid.UUID, aiProviderID uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
			if aiProviderID == uuid.Nil {
				resolvedProviderKeys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
				if err != nil || resolvedProviderKeys.Empty() {
					resolvedProviderKeys = keys
				}
				overrideProviderKeys = resolvedProviderKeys
				return resolvedProviderKeys, nil
			}
			resolvedProviderKeys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, aiProviderID)
			if err != nil {
				return chatprovider.ProviderAPIKeys{}, err
			}
			overrideProviderKeys = resolvedProviderKeys
			return resolvedProviderKeys, nil
		},
		modelOverrideFailureModeHard,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, resolvedModelRoute{}, overrideSet, err
	}
	if !overrideSet {
		return database.ChatModelConfig{}, nil, keys, resolvedModelRoute{}, false, nil
	}

	//nolint:gocritic // Overrides need chatd-scoped provider reads for user-owned chats.
	route, err := p.resolveModelRouteForConfig(dbauthz.AsChatd(ctx), chat.OwnerID, modelConfig, overrideProviderKeys)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, resolvedModelRoute{}, true, err
	}
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:         chat,
		ModelName:    modelConfig.Model,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(chat),
	}, route, modelOpts)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, resolvedModelRoute{}, true, xerrors.Errorf(
			"create %s model override: %w",
			label,
			err,
		)
	}
	return modelConfig, model, route.directProviderKeys(), route, true, nil
}
