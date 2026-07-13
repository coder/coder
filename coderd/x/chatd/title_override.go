package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const titleGenerationOverrideContext = "title_generation"

type parsedModelOverride struct {
	modelConfigID   uuid.UUID
	reasoningEffort *string
}

func parseModelOverride(raw string) (parsedModelOverride, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return parsedModelOverride{}, true
	}
	rawID, rawEffort, hasEffort := strings.Cut(trimmed, ":")
	modelConfigID, err := uuid.Parse(rawID)
	if err != nil || (hasEffort && rawEffort == "") {
		return parsedModelOverride{}, false
	}
	parsed := parsedModelOverride{modelConfigID: modelConfigID}
	if hasEffort {
		parsed.reasoningEffort = &rawEffort
	}
	return parsed, true
}

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
	modelOpts modelBuildOptions,
) (database.ChatModelConfig, fantasy.LanguageModel, aiGatewayModelRoute, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	modelConfig, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		titleGenerationOverrideContext,
		raw,
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
		Chat:         chat,
		ModelName:    modelConfig.Model,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(chat),
	}, route, modelOpts)
	if err != nil {
		return database.ChatModelConfig{}, nil, aiGatewayModelRoute{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return modelConfig, model, route, true, nil
}
