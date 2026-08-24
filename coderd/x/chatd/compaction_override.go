package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const compactionOverrideContext = "compaction"

func readCompactionModelOverride(
	ctx context.Context,
	db database.Store,
) (string, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatCompactionModelOverride(chatdCtx)
	if err != nil {
		return "", xerrors.Errorf(
			"get chat compaction model override: %w",
			err,
		)
	}
	return raw, nil
}

// resolvedCompactionOverride is the compaction override resolved at
// prepare time. The provider/model identity is resolved without building
// the model client so metrics recorded before the client exists
// (still-over-limit) attribute to the same model as the compact action's.
type resolvedCompactionOverride struct {
	Config database.ChatModelConfig
	// ReasoningEffort is the override's requested effort, passed to the
	// model-call resolver when the compact action builds the client.
	ReasoningEffort  *string
	ResolvedProvider string
	ResolvedModel    string
}

// resolveCompactionOverrideConfig resolves the stored deployment-wide
// compaction model override. Unset, malformed, stale, and credential-less
// overrides fall back to the chat model (nil override). This runs on every
// generation prepare because the override's context limit feeds the
// compaction trigger; the model client is built only when compaction runs.
func (p *Server) resolveCompactionOverrideConfig(
	ctx context.Context,
	chat database.Chat,
) (*resolvedCompactionOverride, error) {
	raw, err := readCompactionModelOverride(ctx, p.db)
	if err != nil {
		return nil, xerrors.Errorf(
			"read compaction model override: %w",
			err,
		)
	}

	modelConfig, providerName, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		compactionOverrideContext,
		raw,
		chat.OwnerID,
		func(ctx context.Context, modelConfigID uuid.UUID) (database.ChatModelConfig, string, error) {
			return p.resolveModelConfigForOrganization(ctx, chat.OrganizationID, modelConfigID)
		},
		func(ctx context.Context, ownerID uuid.UUID, aiProviderID uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
			return p.resolveUserProviderAPIKeys(ctx, ownerID, aiProviderID)
		},
		modelOverrideFailureModeSoft,
	)
	if err != nil || !overrideSet {
		return nil, err
	}
	// Already validated by the shared resolver; failure is unreachable.
	resolvedProvider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(
		modelConfig.Model,
		providerName,
	)
	if err != nil {
		return nil, xerrors.Errorf(
			"resolve compaction model override identity: %w",
			err,
		)
	}
	return &resolvedCompactionOverride{
		Config:           modelConfig,
		ReasoningEffort:  overrideEffort,
		ResolvedProvider: resolvedProvider,
		ResolvedModel:    resolvedModel,
	}, nil
}
