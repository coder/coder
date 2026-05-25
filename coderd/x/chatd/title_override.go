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
// generation model override. It returns:
//
//   - modelConfig and model: populated only on success.
//   - providerKeys and route: the credentials and routing metadata for the
//     resolved override model.
//   - overrideSet: true when the admin configured a non-empty override,
//     regardless of whether resolution succeeded. Callers MUST always check
//     err first; overrideSet alone does not imply the model is usable.
//   - err: non-nil when resolution failed. DB read failure returns
//     zero values with overrideSet=false. With overrideSet=true, the override
//     is configured but unusable (deleted model, missing credentials, etc.)
//     and callers should treat this as a hard failure for explicit-override
//     semantics, not a soft fallback.
//
// When the override is unset or stored as malformed, the function returns
// zero values with overrideSet=false so callers can fall back to default
// behavior.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (database.ChatModelConfig, fantasy.LanguageModel, chatprovider.ProviderAPIKeys, modelRoute, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, modelRoute{}, false, xerrors.Errorf(
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
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, modelRoute{}, overrideSet, err
	}
	if !overrideSet {
		return database.ChatModelConfig{}, nil, keys, modelRoute{}, false, nil
	}

	//nolint:gocritic // Title overrides need chatd-scoped provider reads for user-owned chats.
	providerHint, route, err := p.modelRouteForConfig(dbauthz.AsChatd(ctx), modelConfig)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, modelRoute{}, true, err
	}
	model, _, err := p.newModelFromConfig(
		ctx,
		chat,
		providerHint,
		modelConfig.Model,
		overrideProviderKeys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
		route,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, modelRoute{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return modelConfig, model, overrideProviderKeys, route, true, nil
}
