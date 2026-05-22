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
// generation model override. It returns four values:
//
//   - modelConfig and model: populated only on success.
//   - overrideSet: true when the admin configured a non-empty override,
//     regardless of whether resolution succeeded. Callers MUST always check
//     err first; overrideSet alone does not imply the model is usable.
//   - err: non-nil when resolution failed. DB read failure returns
//     (zero, nil, false, err). With overrideSet=true, the override is
//     configured but unusable (deleted model, missing credentials, etc.) and
//     callers should treat this as a hard failure for explicit-override
//     semantics, not a soft fallback.
//
// When the override is unset or stored as malformed, the function returns
// (zero, nil, false, nil) so callers can fall back to default behavior.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (database.ChatModelConfig, fantasy.LanguageModel, chatprovider.ProviderAPIKeys, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, false, xerrors.Errorf(
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
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, overrideSet, err
	}
	if !overrideSet {
		return database.ChatModelConfig{}, nil, keys, false, nil
	}

	providerHint := modelConfig.Provider
	if modelConfig.AIProviderID.Valid {
		//nolint:gocritic // Title overrides need chatd-scoped provider reads for user-owned chats.
		provider, err := p.db.GetAIProviderByID(dbauthz.AsChatd(ctx), modelConfig.AIProviderID.UUID)
		if err != nil {
			return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, true, xerrors.Errorf("get AI provider for title generation override: %w", err)
		}
		if !provider.Enabled {
			return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, true, xerrors.Errorf("AI provider %s is disabled", modelConfig.AIProviderID.UUID)
		}
		providerHint = string(provider.Type)
	}
	model, err := chatprovider.ModelFromConfig(
		providerHint,
		modelConfig.Model,
		overrideProviderKeys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
		nil,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	if model == nil {
		return database.ChatModelConfig{}, nil, chatprovider.ProviderAPIKeys{}, true, xerrors.Errorf(
			"create title generation model override returned nil",
		)
	}

	return modelConfig, model, overrideProviderKeys, true, nil
}
