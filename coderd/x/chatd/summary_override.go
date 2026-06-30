package chatd

import (
	"context"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const summaryGenerationOverrideContext = "summary_generation"

func readSummaryGenerationModelOverride(
	ctx context.Context,
	db database.Store,
) (string, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatSummaryGenerationModelOverride(chatdCtx)
	if err != nil {
		return "", xerrors.Errorf(
			"get chat summary generation model override: %w",
			err,
		)
	}
	return raw, nil
}

// resolveSummaryGenerationModelOverride resolves the deployment-wide summary
// override. overrideSet reports whether one was configured; if true, any error is
// a hard failure (skip generation), and if false the caller uses the chat's model.
func (p *Server) resolveSummaryGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
	modelOpts modelBuildOptions,
) (database.ChatModelConfig, fantasy.LanguageModel, chatprovider.ProviderAPIKeys, resolvedModelRoute, bool, error) {
	return p.resolveGenerationModelOverride(
		ctx, chat, keys, modelOpts,
		summaryGenerationOverrideContext, readSummaryGenerationModelOverride,
	)
}
