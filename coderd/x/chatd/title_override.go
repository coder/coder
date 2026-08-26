package chatd

import (
	"context"
	"database/sql"

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
	organizationID uuid.UUID,
) (database.ChatOrganizationModelOverride, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	override, err := db.GetChatOrganizationModelOverride(chatdCtx, database.GetChatOrganizationModelOverrideParams{
		OrganizationID: organizationID,
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

// resolveTitleGenerationModelOverride resolves the chat organization's title
// generation model override. A configured but unusable override is a hard
// failure. When no row is configured, callers may use the default title model.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (resolvedModelCall, bool, error) {
	override, err := readTitleGenerationModelOverride(ctx, p.db, chat.OrganizationID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return resolvedModelCall{}, false, nil
		}
		return resolvedModelCall{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	modelConfig, _, overrideEffort, overrideSet, err := p.resolveOrganizationModelOverride(
		ctx,
		titleGenerationOverrideContext,
		override,
		chat.OwnerID,
		func(ctx context.Context, modelConfigID uuid.UUID) (database.ChatModelConfig, string, error) {
			return p.resolveModelConfigAndNormalizedProvider(ctx, chat.OwnerID, modelConfigID)
		},
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
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:          "title",
		chat:             chat,
		explicitConfig:   &modelConfig,
		requestedEffort:  overrideEffort,
		chatdScopedRoute: true,
		buildOptions:     modelOpts,
	})
	if err != nil {
		return resolvedModelCall{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return resolved, true, nil
}
