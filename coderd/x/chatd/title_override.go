package chatd

import (
	"context"
	"strings"

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
) (resolvedModelCall, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return resolvedModelCall{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	modelConfig, _, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
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
		return resolvedModelCall{}, overrideSet, err
	}
	if !overrideSet {
		return resolvedModelCall{}, false, nil
	}
	modelConfig = withResolvedReasoningEffort(modelConfig, overrideEffort)

	resolved, err := p.resolveModelCall(ctx, titleOverrideSpec(chat, modelConfig, modelOpts))
	if err != nil {
		return resolvedModelCall{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return resolved, true, nil
}
