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

type configSelectionMode int

const (
	// configFromChat resolves the chat's last model config, falling back to
	// the deployment default. The config must be enabled.
	configFromChat configSelectionMode = iota
	// configExplicit uses a config row the caller already selected (override
	// and preferred-model flows own their selection and fallback policy).
	configExplicit
	// configFixedModel builds a client for a provider/model pair without a
	// config row (computer use, debug transport rebuilds).
	configFixedModel
)

type configSelection struct {
	mode          configSelectionMode
	config        database.ChatModelConfig
	providerType  string
	modelName     string
	configOptions []byte
	callConfig    codersdk.ChatModelCallConfig
}

type debugPolicy int

const (
	debugPolicyOff debugPolicy = iota
	// debugPolicyAware records only when chat debug is enabled.
	debugPolicyAware
	// debugPolicyForced records after the caller has enabled debugging.
	debugPolicyForced
)

// modelCallSpec declares config, routing, and construction policy for one LLM
// call. Build it with a purpose-specific constructor.
type modelCallSpec struct {
	// purpose labels resolver logs only; it does not affect call behavior.
	purpose         string
	chat            database.Chat
	config          configSelection
	requestedEffort *string

	omitProviderOptions bool
	debug               debugPolicy
	debugSvc            *chatdebug.Service
	debugWrapProvider   string
	debugWrapModel      string
	routeOverride       *aiGatewayModelRoute
	// chatdScopedRoute resolves the route with chatd scope. Deployment-wide
	// override models must route for user-owned chats regardless of the
	// caller's actor.
	chatdScopedRoute       bool
	defaultMaxOutputTokens bool
	buildOptions           modelBuildOptions
}

func chatRequestedEffort(chat database.Chat) *string {
	if !chat.LastReasoningEffort.Valid {
		return nil
	}
	return new(string(chat.LastReasoningEffort.ChatReasoningEffort))
}

func standardTurnSpec(chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:                "standard_turn",
		chat:                   chat,
		config:                 configSelection{mode: configFromChat},
		requestedEffort:        chatRequestedEffort(chat),
		debug:                  debugPolicyAware,
		defaultMaxOutputTokens: true,
		buildOptions:           buildOpts,
	}
}

// chatModelSpec preserves summary and status-label behavior: no provider
// options and no standard-turn token default.
func chatModelSpec(purpose string, chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:             purpose,
		chat:                chat,
		config:              configSelection{mode: configFromChat},
		omitProviderOptions: true,
		debug:               debugPolicyAware,
		buildOptions:        buildOpts,
	}
}

// Background title generation uses the config's default reasoning effort, not
// the user's per-turn choice.
func titleChatSpec(chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:      "title",
		chat:         chat,
		config:       configSelection{mode: configFromChat},
		debug:        debugPolicyAware,
		buildOptions: buildOpts,
	}
}

// titleOverrideSpec uses chatd scope so owners need not have provider read
// access.
func titleOverrideSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:          "title",
		chat:             chat,
		config:           configSelection{mode: configExplicit, config: config},
		chatdScopedRoute: true,
		buildOptions:     buildOpts,
	}
}

// manualTitleSpec leaves debug instrumentation to a separate rebuild to
// preserve manual-title behavior.
func manualTitleSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:      "title",
		chat:         chat,
		config:       configSelection{mode: configExplicit, config: config},
		buildOptions: buildOpts,
	}
}

// compactionOverrideSpec receives a config with resolved reasoning effort and
// uses chatd scope so owners need not have provider read access.
func compactionOverrideSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:          "compaction",
		chat:             chat,
		config:           configSelection{mode: configExplicit, config: config},
		debug:            debugPolicyAware,
		chatdScopedRoute: true,
		buildOptions:     buildOpts,
	}
}

// advisorOverrideSpec omits provider options until the advisor pins its
// reasoning effort and output cap.
func advisorOverrideSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:             "advisor",
		chat:                chat,
		config:              configSelection{mode: configExplicit, config: config},
		omitProviderOptions: true,
		buildOptions:        buildOpts,
	}
}

// manualTitleDebugSpec preserves the resolved route and caller-selected
// attribution labels while enabling HTTP recording.
func manualTitleDebugSpec(
	chat database.Chat,
	config database.ChatModelConfig,
	route aiGatewayModelRoute,
	debugSvc *chatdebug.Service,
	routeProvider string,
	buildOpts modelBuildOptions,
) modelCallSpec {
	return modelCallSpec{
		purpose:             "debug_rebuild",
		chat:                chat,
		config:              configSelection{mode: configExplicit, config: config},
		omitProviderOptions: true,
		debug:               debugPolicyForced,
		debugSvc:            debugSvc,
		debugWrapProvider:   routeProvider,
		debugWrapModel:      config.Model,
		routeOverride:       &route,
		buildOptions:        buildOpts,
	}
}

// computerUseSpec uses the chat config only for per-call options because the
// fixed computer-use model has no config row.
func computerUseSpec(
	chat database.Chat,
	modelProvider string,
	modelName string,
	chatCallConfig codersdk.ChatModelCallConfig,
	buildOpts modelBuildOptions,
) modelCallSpec {
	return modelCallSpec{
		purpose: "computer_use",
		chat:    chat,
		config: configSelection{
			mode:         configFixedModel,
			providerType: modelProvider,
			modelName:    modelName,
			callConfig:   chatCallConfig,
		},
		requestedEffort: chatRequestedEffort(chat),
		debug:           debugPolicyAware,
		buildOptions:    buildOpts,
	}
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
	switch spec.config.mode {
	case configFromChat:
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
	case configExplicit:
		out.dbConfig = spec.config.config
		modelName = out.dbConfig.Model
		configOptions = out.dbConfig.Options
	case configFixedModel:
		modelName = spec.config.modelName
		configOptions = spec.config.configOptions
	}

	// clientCallConfig always comes from configOptions: it drives client
	// construction (beta headers, OpenAI transport override), while
	// out.callConfig drives per-call option derivation and can differ for
	// configFixedModel (computer use derives options from the chat model).
	clientCallConfig, err := parseModelConfigOptions(configOptions)
	if err != nil {
		return resolvedModelCall{}, modelCallConfigParseError{err: err}
	}
	if spec.config.mode == configFixedModel {
		out.callConfig = spec.config.callConfig
	} else {
		out.callConfig = clientCallConfig
	}
	if spec.defaultMaxOutputTokens && out.callConfig.MaxOutputTokens == nil {
		out.callConfig.MaxOutputTokens = ptr.Ref(defaultChatMaxOutputTokens)
	}

	if spec.routeOverride != nil {
		out.route = *spec.routeOverride
	} else {
		routeCtx := ctx
		if spec.chatdScopedRoute {
			//nolint:gocritic // Deployment-wide override models need chatd-scoped provider reads for user-owned chats.
			routeCtx = dbauthz.AsChatd(ctx)
		}
		var err error
		if spec.config.mode == configFixedModel {
			out.route, err = p.resolveModelRouteForProviderType(routeCtx, spec.chat.OwnerID, spec.config.providerType)
		} else {
			out.route, err = p.resolveModelRouteForConfig(routeCtx, spec.chat.OwnerID, out.dbConfig)
		}
		if err != nil {
			return resolvedModelCall{}, err
		}
	}

	out.resolvedProvider, out.resolvedModel, err = chatprovider.ResolveModelWithProviderHint(
		modelName,
		out.route.ModelProviderHint,
	)
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf("resolve model metadata: %w", err)
	}

	debugSvc := spec.debugSvc
	switch spec.debug {
	case debugPolicyAware:
		if debugSvc == nil {
			debugSvc = p.debugService()
		}
		out.debugEnabled = debugSvc != nil && debugSvc.IsEnabled(ctx, spec.chat.ID, spec.chat.OwnerID)
	case debugPolicyForced:
		out.debugEnabled = true
	case debugPolicyOff:
	}

	clientModelName := modelName
	clientRoute := out.route
	if spec.debug == debugPolicyAware {
		// Debug-aware calls preserve their historical use of the resolved identity;
		// other flows pass the configured model name.
		clientRoute.ModelProviderHint = out.resolvedProvider
		clientModelName = out.resolvedModel
	}

	buildOpts := spec.buildOptions
	buildOpts.RecordHTTP = out.debugEnabled
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:         spec.chat,
		ModelName:    clientModelName,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(spec.chat),
		CallConfig:   clientCallConfig,
	}, clientRoute, buildOpts)
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf("create model: %w", err)
	}

	if out.debugEnabled && debugSvc != nil {
		wrapProvider := out.resolvedProvider
		wrapModel := out.resolvedModel
		if spec.debug == debugPolicyForced {
			wrapProvider = spec.debugWrapProvider
			wrapModel = spec.debugWrapModel
		}
		model = model.WithLanguageModel(chatdebug.WrapModel(model.LanguageModel(), debugSvc, chatdebug.RecorderOptions{
			ChatID:   spec.chat.ID,
			OwnerID:  spec.chat.OwnerID,
			Provider: wrapProvider,
			Model:    wrapModel,
		}))
	}
	out.model = model

	if !spec.omitProviderOptions {
		out.providerOptions = out.deriveProviderOptions(out.callConfig, spec.requestedEffort)
	}

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

// Compaction summaries omit sampling and output-token options.
func (r resolvedModelCall) newCompactionSummaryCall() fantasy.Call {
	toolChoiceNone := fantasy.ToolChoiceNone
	return fantasy.Call{
		ToolChoice:      &toolChoiceNone,
		ProviderOptions: r.providerOptions,
	}
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
		ProviderOptions:   r.providerOptions,
	}
}
