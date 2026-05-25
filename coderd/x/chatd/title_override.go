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

const titleGenerationOverrideContext = "title_generation"

func readTitleGenerationModelOverride(
	ctx context.Context,
	db database.Store,
) (string, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatTitleGenerationModelOverride(chatdCtx)
	if err != nil {
		return "", xerrors.Errorf(
			"get chat title generation model override: %w",
			err,
		)
	}
	return raw, nil
}

// resolveTitleGenerationModelOverride resolves the deployment-wide title
// generation model override. overrideSet is true when an override was
// configured; in that case any returned error is a hard failure. When
// overrideSet is false, callers may fall back to the default title model.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
	modelOpts modelBuildOptions,
) (database.ChatModelConfig, fantasy.LanguageModel, chatprovider.ProviderAPIKeys, resolvedModelRoute, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, resolvedModelRoute{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	overrideProviderKeys := keys
	modelConfig, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		titleGenerationOverrideContext,
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

	//nolint:gocritic // Title overrides need chatd-scoped provider reads for user-owned chats.
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
			"create title generation model override: %w",
			err,
		)
	}
	return modelConfig, model, route.directProviderKeys(), route, true, nil
}
