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

// callPurpose labels the flow a model call serves. It only feeds logging and
// debug attribution; behavior is driven by the other modelCallSpec fields.
type callPurpose string

const (
	callPurposeStandardTurn callPurpose = "standard_turn"
	callPurposeComputerUse  callPurpose = "computer_use"
	callPurposeTitle        callPurpose = "title"
	callPurposeSummary      callPurpose = "chat_summary"
	callPurposeStatusLabel  callPurpose = "turn_status_label"
	callPurposeCompaction   callPurpose = "compaction"
	callPurposeAdvisor      callPurpose = "advisor"
	callPurposeDebugRebuild callPurpose = "debug_rebuild"
)

// defaultChatMaxOutputTokens caps standard-turn output when the model config
// leaves MaxOutputTokens unset.
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
	mode   configSelectionMode
	config database.ChatModelConfig
	// providerType routes configFixedModel calls when no route override is
	// supplied.
	providerType string
	modelName    string
	// configOptions is the raw options JSON applied to client construction
	// for configFixedModel (beta headers, OpenAI transport override).
	configOptions []byte
	// callConfig supplies the per-call config for configFixedModel option
	// derivation, since there is no config row to parse.
	callConfig codersdk.ChatModelCallConfig
}

type debugPolicy int

const (
	// debugPolicyOff builds a plain client with no debug recording.
	debugPolicyOff debugPolicy = iota
	// debugPolicyAware records HTTP traffic and wraps the model when the
	// chat debug service enables this chat.
	debugPolicyAware
	// debugPolicyForced always records and wraps; used to rebuild a debug
	// transport after the caller verified debug is enabled.
	debugPolicyForced
)

type providerOptionPolicy int

const (
	// providerOptionsDerive converts the call config into per-call provider
	// options.
	providerOptionsDerive providerOptionPolicy = iota
	// providerOptionsOmit skips derivation. Used by flows that historically
	// never sent provider options and by callers that derive separately.
	providerOptionsOmit
)

// modelCallSpec describes one LLM call to resolve: which config to use, how
// to route it, and which construction policies apply. Build specs via the
// purpose-specific constructors so per-flow policy stays declared in one
// place.
type modelCallSpec struct {
	purpose         callPurpose
	chat            database.Chat
	config          configSelection
	requestedEffort *string
	providerOptions providerOptionPolicy
	debug           debugPolicy
	// debugSvc, debugWrapProvider, and debugWrapModel label forced debug
	// recordings; callers keep their historical attribution labels.
	debugSvc          *chatdebug.Service
	debugWrapProvider string
	debugWrapModel    string
	// routeOverride reuses a previously resolved route instead of resolving
	// one (debug transport rebuilds).
	routeOverride *aiGatewayModelRoute
	// chatdScopedRoute resolves the route with chatd scope. Deployment-wide
	// override models must route for user-owned chats regardless of the
	// caller's actor.
	chatdScopedRoute bool
	// defaultMaxOutputTokens applies the standard-turn output cap when the
	// config leaves MaxOutputTokens unset.
	defaultMaxOutputTokens bool
	buildOptions           modelBuildOptions
}

// chatRequestedEffort is the user's per-turn reasoning effort choice, which
// the config's bounds clamp during option derivation.
func chatRequestedEffort(chat database.Chat) *string {
	if !chat.LastReasoningEffort.Valid {
		return nil
	}
	return new(string(chat.LastReasoningEffort.ChatReasoningEffort))
}

func standardTurnSpec(chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:                callPurposeStandardTurn,
		chat:                   chat,
		config:                 configSelection{mode: configFromChat},
		requestedEffort:        chatRequestedEffort(chat),
		providerOptions:        providerOptionsDerive,
		debug:                  debugPolicyAware,
		defaultMaxOutputTokens: true,
		buildOptions:           buildOpts,
	}
}

// chatModelSpec resolves the chat's model without deriving provider options
// or applying the standard-turn token default. Summary and status-label
// calls historically send no provider options; that omission is preserved
// here as declared policy.
func chatModelSpec(purpose callPurpose, chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:         purpose,
		chat:            chat,
		config:          configSelection{mode: configFromChat},
		providerOptions: providerOptionsOmit,
		debug:           debugPolicyAware,
		buildOptions:    buildOpts,
	}
}

// titleChatSpec resolves the chat's own model as the title-generation
// fallback candidate. Title calls derive provider options without a
// requested effort: the user's per-turn effort choice applies to turns, not
// background title generation.
func titleChatSpec(chat database.Chat, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:         callPurposeTitle,
		chat:            chat,
		config:          configSelection{mode: configFromChat},
		providerOptions: providerOptionsDerive,
		debug:           debugPolicyAware,
		buildOptions:    buildOpts,
	}
}

// titleOverrideSpec builds the deployment-wide title override model from the
// caller-selected config row. The route resolves with chatd scope so the
// override works for chats whose owner cannot read the provider.
func titleOverrideSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:          callPurposeTitle,
		chat:             chat,
		config:           configSelection{mode: configExplicit, config: config},
		providerOptions:  providerOptionsDerive,
		chatdScopedRoute: true,
		buildOptions:     buildOpts,
	}
}

// manualTitleSpec builds a caller-selected manual-title model: the preferred
// small model or the chat's own config as fallback. Debug recording is
// handled by a separate rebuild, matching the historical construction.
func manualTitleSpec(chat database.Chat, config database.ChatModelConfig, buildOpts modelBuildOptions) modelCallSpec {
	return modelCallSpec{
		purpose:         callPurposeTitle,
		chat:            chat,
		config:          configSelection{mode: configExplicit, config: config},
		providerOptions: providerOptionsDerive,
		buildOptions:    buildOpts,
	}
}

// computerUseSpec swaps in the deployment's computer-use model. The client is
// built without config options because the fixed model has no config row; the
// chat model's call config still drives per-call provider options so admin
// tuning follows the turn.
func computerUseSpec(
	chat database.Chat,
	modelProvider string,
	modelName string,
	chatCallConfig codersdk.ChatModelCallConfig,
	buildOpts modelBuildOptions,
) modelCallSpec {
	return modelCallSpec{
		purpose: callPurposeComputerUse,
		chat:    chat,
		config: configSelection{
			mode:         configFixedModel,
			providerType: modelProvider,
			modelName:    modelName,
			callConfig:   chatCallConfig,
		},
		requestedEffort: chatRequestedEffort(chat),
		providerOptions: providerOptionsDerive,
		debug:           debugPolicyAware,
		buildOptions:    buildOpts,
	}
}

// resolvedModelCall is the output of resolveModelCall: a ready client plus
// the metadata callers need for prompts, metrics, and debug attribution.
type resolvedModelCall struct {
	model      chatprovider.Model
	dbConfig   database.ChatModelConfig
	callConfig codersdk.ChatModelCallConfig
	// providerOptions is nil when the spec's policy omits derivation.
	providerOptions  fantasy.ProviderOptions
	resolvedProvider string
	resolvedModel    string
	route            aiGatewayModelRoute
	debugEnabled     bool
}

// resolveModelCall is the single pipeline from a call spec to a ready model
// client: config selection, call-config parse, route and identity resolution,
// client construction (including debug recording), and provider-option
// derivation.
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

	switch spec.config.mode {
	case configFixedModel:
		out.callConfig = spec.config.callConfig
	default:
		var err error
		out.callConfig, err = parseModelConfigOptions(configOptions)
		if err != nil {
			return resolvedModelCall{}, xerrors.Errorf("parse model call config: %w", err)
		}
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

	var err error
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
		// Preserved from newDebugAwareModel: debug-aware flows build the
		// client from the resolved identity while other flows pass the raw
		// configured model name. The distinction only matters for malformed
		// slash-namespaced model names, so it is kept rather than unified.
		clientRoute.ModelProviderHint = out.resolvedProvider
		clientModelName = out.resolvedModel
	}

	buildOpts := spec.buildOptions
	buildOpts.RecordHTTP = out.debugEnabled
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:          spec.chat,
		ModelName:     clientModelName,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(spec.chat),
		ConfigOptions: configOptions,
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

	if spec.providerOptions == providerOptionsDerive {
		out.providerOptions = chatprovider.ProviderOptionsForCall(model, out.callConfig, spec.requestedEffort)
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

// objectCallOverrides carries the caller-owned schema and token cap for a
// structured-output call. Quickgen flows use fixed caps instead of the model
// config's tuning.
type objectCallOverrides struct {
	schemaName        string
	schemaDescription string
	maxOutputTokens   int64
}

// newObjectCall builds the fantasy.ObjectCall envelope for one
// structured-output call. The caller attaches the prompt before sending.
func (r resolvedModelCall) newObjectCall(o objectCallOverrides) fantasy.ObjectCall {
	return fantasy.ObjectCall{
		SchemaName:        o.schemaName,
		SchemaDescription: o.schemaDescription,
		MaxOutputTokens:   ptr.Ref(o.maxOutputTokens),
		ProviderOptions:   r.providerOptions,
	}
}
