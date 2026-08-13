package chatd

import (
	"context"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

const defaultChatMaxOutputTokens = int64(32_000)

// fixedModelCall selects a provider/model pair that has no config row of its
// own (computer use).
type fixedModelCall struct {
	providerType string
	modelName    string
	callConfig   codersdk.ChatModelCallConfig
}

type modelCallSpec struct {
	// purpose labels resolver logs only; it does not affect call behavior.
	purpose        string
	chat           database.Chat
	explicitConfig *database.ChatModelConfig
	fixedModel     *fixedModelCall
	// requestedEffort overrides the config's default reasoning effort.
	requestedEffort *string
	// chatdScopedRoute resolves the route with chatd scope. Deployment-wide
	// override models must route for user-owned chats regardless of the
	// caller's actor.
	chatdScopedRoute bool
	buildOptions     modelBuildOptions
}

func chatRequestedEffort(chat database.Chat) *string {
	if !chat.LastReasoningEffort.Valid {
		return nil
	}
	return new(string(chat.LastReasoningEffort.ChatReasoningEffort))
}

// modelCallConfigParseError lets the advisor distinguish malformed options
// from route and client failures when deciding whether to fall back.
type modelCallConfigParseError struct{ err error }

func (e modelCallConfigParseError) Error() string {
	return "parse model call config: " + e.err.Error()
}

func (e modelCallConfigParseError) Unwrap() error { return e.err }

type resolvedModelCall struct {
	model            chatprovider.Model
	dbConfig         database.ChatModelConfig
	callConfig       codersdk.ChatModelCallConfig
	providerOptions  fantasy.ProviderOptions
	resolvedProvider string
	resolvedModel    string
	route            aiGatewayModelRoute
	debugEnabled     bool
}

// resolveModelCall is the single pipeline from a spec to a ready model
// client plus the call metadata flows need.
func (p *Server) resolveModelCall(ctx context.Context, spec modelCallSpec) (resolvedModelCall, error) {
	out := resolvedModelCall{}

	var modelName string
	var configOptions []byte
	switch {
	case spec.fixedModel != nil:
		modelName = spec.fixedModel.modelName
	case spec.explicitConfig != nil:
		out.dbConfig = *spec.explicitConfig
		modelName = out.dbConfig.Model
		configOptions = out.dbConfig.Options
	default:
		dbConfig, err := p.resolveModelConfig(ctx, spec.chat)
		if err != nil {
			return resolvedModelCall{}, xerrors.Errorf("resolve model config: %w", err)
		}
		if !dbConfig.Enabled {
			return resolvedModelCall{}, xerrors.Errorf("chat model config %s is disabled", dbConfig.ID)
		}
		out.dbConfig = dbConfig
		modelName = dbConfig.Model
		configOptions = dbConfig.Options
	}

	// clientCallConfig drives client construction; out.callConfig drives
	// per-call option derivation and comes from the chat model for computer
	// use, whose fixed model has no config of its own.
	clientCallConfig, err := parseModelConfigOptions(configOptions)
	if err != nil {
		return resolvedModelCall{}, modelCallConfigParseError{err: err}
	}
	if spec.fixedModel != nil {
		out.callConfig = spec.fixedModel.callConfig
	} else {
		out.callConfig = clientCallConfig
	}
	if out.callConfig.MaxOutputTokens == nil {
		out.callConfig.MaxOutputTokens = ptr.Ref(defaultChatMaxOutputTokens)
	}

	routeCtx := ctx
	if spec.chatdScopedRoute {
		//nolint:gocritic // Deployment-wide override models need chatd-scoped provider reads for user-owned chats.
		routeCtx = dbauthz.AsChatd(ctx)
	}
	if spec.fixedModel != nil {
		out.route, err = p.resolveModelRouteForProviderType(routeCtx, spec.chat.OwnerID, spec.fixedModel.providerType)
	} else {
		out.route, err = p.resolveModelRouteForConfig(routeCtx, spec.chat.OwnerID, out.dbConfig)
	}
	if err != nil {
		return resolvedModelCall{}, err
	}

	// The resolved identity feeds metadata, logs, and debug labels. The
	// client is constructed with the configured model string so gateway
	// validation sees the name exactly as configured.
	out.resolvedProvider, out.resolvedModel, err = chatprovider.ResolveModelWithProviderHint(
		modelName,
		out.route.ModelProviderHint,
	)
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf("resolve model metadata: %w", err)
	}

	debugSvc := p.debugService()
	out.debugEnabled = debugSvc != nil && debugSvc.IsEnabled(ctx, spec.chat.ID, spec.chat.OwnerID)

	buildOpts := spec.buildOptions
	buildOpts.RecordHTTP = out.debugEnabled
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:         spec.chat,
		ModelName:    modelName,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(spec.chat),
		CallConfig:   clientCallConfig,
	}, out.route, buildOpts)
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf("create model: %w", err)
	}

	if out.debugEnabled {
		model = model.WithLanguageModel(chatdebug.WrapModel(model.LanguageModel(), debugSvc, chatdebug.RecorderOptions{
			ChatID:   spec.chat.ID,
			OwnerID:  spec.chat.OwnerID,
			Provider: out.resolvedProvider,
			Model:    out.resolvedModel,
		}))
	}
	out.model = model

	out.providerOptions = out.deriveProviderOptions(out.callConfig, spec.requestedEffort)

	p.logger.Debug(ctx, "resolved model call",
		slog.F("purpose", spec.purpose),
		slog.F("chat_id", spec.chat.ID),
		slog.F("provider", out.resolvedProvider),
		slog.F("model", out.resolvedModel),
		slog.F("debug_enabled", out.debugEnabled),
	)
	return out, nil
}

func (r resolvedModelCall) newCall() fantasy.Call {
	return fantasy.Call{
		ProviderOptions:  r.providerOptions,
		MaxOutputTokens:  r.callConfig.MaxOutputTokens,
		Temperature:      r.callConfig.Temperature,
		TopP:             r.callConfig.TopP,
		TopK:             r.callConfig.TopK,
		PresencePenalty:  r.callConfig.PresencePenalty,
		FrequencyPenalty: r.callConfig.FrequencyPenalty,
	}
}

// compactionSummaryCall follows the resolved call template, except summaries
// must not call tools and must not carry the default output cap: the summary
// request is non-streaming, and the Anthropic SDK rejects non-streaming
// requests whose max_tokens implies a completion longer than ten minutes.
func compactionSummaryCall(resolved resolvedModelCall) fantasy.Call {
	call := resolved.newCall()
	toolChoiceNone := fantasy.ToolChoiceNone
	call.ToolChoice = &toolChoiceNone
	call.MaxOutputTokens = nil
	return call
}

// deriveProviderOptions is the only production ProviderOptionsForCall call
// site; callers that mutate the call config after resolution re-derive here.
func (r resolvedModelCall) deriveProviderOptions(callConfig codersdk.ChatModelCallConfig, requestedEffort *string) fantasy.ProviderOptions {
	return chatprovider.ProviderOptionsForCall(r.model, callConfig, requestedEffort)
}

func (r resolvedModelCall) newObjectCall(schemaName, schemaDescription string, maxOutputTokens int64) fantasy.ObjectCall {
	return fantasy.ObjectCall{
		SchemaName:        schemaName,
		SchemaDescription: schemaDescription,
		MaxOutputTokens:   ptr.Ref(maxOutputTokens),
		Temperature:       r.callConfig.Temperature,
		TopP:              r.callConfig.TopP,
		TopK:              r.callConfig.TopK,
		PresencePenalty:   r.callConfig.PresencePenalty,
		FrequencyPenalty:  r.callConfig.FrequencyPenalty,
		ProviderOptions:   r.providerOptions,
	}
}
