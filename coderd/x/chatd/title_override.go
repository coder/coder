package chatd

import (
	"context"
	"database/sql"

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
	orgID uuid.UUID,
) (database.ChatOrganizationModelOverride, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	override, err := db.GetChatOrganizationModelOverride(chatdCtx, database.GetChatOrganizationModelOverrideParams{
		OrganizationID: orgID,
		Context:        titleGenerationOverrideContext,
	})
	if err != nil {
		return database.ChatOrganizationModelOverride{}, xerrors.Errorf(
			"get chat title generation model override: %w",
			err,
		)
	}
	return override, nil
}

// resolveTitleGenerationModelOverride resolves the chat organization's
// title generation model override. overrideSet is true when an override was
// configured, whether or not it resolves: a configured override that does
// not resolve (disabled or soft-deleted config, since the composite FK
// makes cross-org pins unrepresentable) is a HARD failure, matching the
// pre-org-scoping behavior. Model construction failures after a successful
// resolution stay hard failures. When overrideSet is false (no row
// configured), callers may fall back to the default title model.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (database.ChatModelConfig, fantasy.LanguageModel, aiGatewayModelRoute, bool, error) {
	override, err := readTitleGenerationModelOverride(ctx, p.db, chat.OrganizationID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// Absence is unset: fall back to the default title model.
			return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, false, nil
		}
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	modelConfig, _, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		titleGenerationOverrideContext,
		override,
		chat.OwnerID,
		p.resolveModelConfigAndNormalizedProvider,
		func(ctx context.Context, ownerID uuid.UUID, aiProviderID uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
			return p.resolveUserProviderAPIKeys(ctx, ownerID, aiProviderID)
		},
		modelOverrideFailureModeHard,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, overrideSet, err
	}
	if !overrideSet {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, false, nil
	}
	modelConfig = withResolvedReasoningEffort(modelConfig, overrideEffort)

	//nolint:gocritic // Title overrides need chatd-scoped provider reads for user-owned chats.
	route, err := p.resolveModelRouteForConfig(dbauthz.AsChatd(ctx), chat.OwnerID, modelConfig)
	if err != nil {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, true, err
	}
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:          chat,
		ModelName:     modelConfig.Model,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(chat),
		ConfigOptions: modelConfig.Options,
	}, route, modelOpts)
	if err != nil {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return modelConfig, model, route, true, nil
}
