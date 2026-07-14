package chatd

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
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

// compactionModelOverride carries the built compaction override model plus
// the identity metadata debug runs and prompt sanitization need.
type compactionModelOverride struct {
	modelConfig      database.ChatModelConfig
	model            fantasy.LanguageModel
	resolvedProvider string
	resolvedModel    string
	// providerOptions include the override's reasoning effort for the
	// summary call.
	providerOptions fantasy.ProviderOptions
}

// resolvedCompactionOverride is the compaction override resolved at
// prepare time. The provider/model identity is resolved without building
// the model client so metrics recorded before the client exists
// (still-over-limit) attribute to the same model as the compact action's.
type resolvedCompactionOverride struct {
	Config database.ChatModelConfig
	// ResolvedProvider and ResolvedModel match the built client's
	// identity: ResolveModelWithProviderHint normalizes its hint, so the
	// normalized provider name here and the route's raw provider type in
	// buildCompactionOverrideModel yield the same result.
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
		p.resolveModelConfigAndNormalizedProvider,
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
		Config:           withResolvedReasoningEffort(modelConfig, overrideEffort),
		ResolvedProvider: resolvedProvider,
		ResolvedModel:    resolvedModel,
	}, nil
}

// buildCompactionOverrideModel resolves the route and constructs the model
// client for a usable override config. Errors are hard failures: a usable
// override that cannot be constructed must fail the generation visibly
// instead of silently compacting with the chat model.
func (p *Server) buildCompactionOverrideModel(
	ctx context.Context,
	chat database.Chat,
	modelConfig database.ChatModelConfig,
	modelOpts modelBuildOptions,
) (compactionModelOverride, error) {
	//nolint:gocritic // Compaction overrides need chatd-scoped provider reads for user-owned chats.
	route, err := p.resolveModelRouteForConfig(dbauthz.AsChatd(ctx), chat.OwnerID, modelConfig)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"resolve compaction model override route: %w",
			err,
		)
	}
	resolvedProvider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(
		modelConfig.Model,
		route.ModelProviderHint,
	)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"resolve compaction model override metadata: %w",
			err,
		)
	}
	model, _, err := p.newDebugAwareModel(ctx, modelClientRequest{
		Chat:         chat,
		ModelName:    modelConfig.Model,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(chat),
	}, route, modelOpts)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"create compaction model override: %w",
			err,
		)
	}
	providerOptions, err := compactionOverrideProviderOptions(model, modelConfig)
	if err != nil {
		return compactionModelOverride{}, err
	}
	return compactionModelOverride{
		modelConfig:      modelConfig,
		model:            model,
		resolvedProvider: resolvedProvider,
		resolvedModel:    resolvedModel,
		providerOptions:  providerOptions,
	}, nil
}

// compactionOverrideProviderOptions converts the override config's call
// options, including the admin-resolved reasoning effort, into provider
// options for the summary call.
func compactionOverrideProviderOptions(
	model fantasy.LanguageModel,
	modelConfig database.ChatModelConfig,
) (fantasy.ProviderOptions, error) {
	callConfig := codersdk.ChatModelCallConfig{}
	if len(modelConfig.Options) > 0 {
		if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
			return nil, xerrors.Errorf(
				"parse compaction model override call config: %w",
				err,
			)
		}
	}
	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(
		model,
		callConfig.ProviderOptions,
	)
	reasoningEffort := chatprovider.ResolveReasoningEffort(
		nil,
		callConfig.ReasoningEffort,
	)
	return chatprovider.ApplyReasoningEffort(model, providerOptions, reasoningEffort), nil
}
