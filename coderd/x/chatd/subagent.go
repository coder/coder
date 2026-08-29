package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

var ErrSubagentNotDescendant = xerrors.New("target chat is not a descendant of current chat")

// ErrSubagentWaitTimeout is returned by awaitSubagentCompletion when the
// wait deadline elapses before the subagent reaches a terminal status. The
// agent is still working and the wait can be retried.
var ErrSubagentWaitTimeout = xerrors.New("timed out waiting for delegated subagent completion")

// subagentToolNameAliases maps deprecated subagent tool names to their
// current names so historical close_agent calls in chat history still
// dispatch to interrupt_agent without advertising the old name in the
// tool list.
var subagentToolNameAliases = map[string]string{
	"close_agent": "interrupt_agent",
}

// subagentStatusError wraps a subagent that reached error status. It
// carries the chat and report so callers can surface a structured,
// recoverable-aware payload instead of a bare tool error.
type subagentStatusError struct {
	chat   database.Chat
	report string
	reason string
}

func (e *subagentStatusError) Error() string { return e.reason }

var (
	errInvalidModelOverrideMetadata   = xerrors.New("invalid model override metadata")
	errModelConfigOutsideOrganization = xerrors.Errorf("%w: model config belongs to another organization", sql.ErrNoRows)
)

type modelOverrideConfigResolver func(
	context.Context,
	uuid.UUID,
) (database.ChatModelConfig, string, error)

type modelOverrideProviderKeysResolver func(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (chatprovider.ProviderAPIKeys, error)

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

const (
	subagentAwaitPollInterval  = 200 * time.Millisecond
	subagentAwaitFallbackPoll  = 5 * time.Second
	defaultSubagentWaitTimeout = 5 * time.Minute

	defaultListAgentsLimit       = 10
	maxListAgentsLimit           = 50
	subagentRecordingStopTimeout = 90 * time.Second
)

// computerUseSubagentSystemPrompt is the system prompt prepended to
// every computer use subagent chat. It instructs the model on how to
// interact with the desktop environment via the computer tool.
const computerUseSubagentSystemPrompt = `You are a computer use agent with access to a desktop environment. You can see the screen, move the mouse, click, type, scroll, and drag.

Your primary tool is the "computer" tool which lets you interact with the desktop. After every action you take, you will receive a screenshot showing the current state of the screen. Use these screenshots to verify your actions and plan next steps.

Guidelines:
- Always start by taking a screenshot to see the current state of the desktop.
- Use wait or ordinary actions when you only need a screenshot for your own reasoning.
- Use an explicit screenshot action when you want to share a durable screenshot with the user; those screenshots are attached to the chat automatically.
- Be precise with coordinates when clicking or typing.
- Wait for UI elements to load before interacting with them.
- If an action doesn't produce the expected result, try alternative approaches.
- Report what you accomplished when done.`

type waitAgentArgs struct {
	ChatID         string `json:"chat_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" description:"Defaults to 5 minutes."`
}

type messageAgentArgs struct {
	ChatID    string `json:"chat_id"`
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

type interruptAgentArgs struct {
	ChatID string `json:"chat_id"`
}

type listAgentsArgs struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

type listSubagentModelsArgs struct{}

func subagentModelOverrideLogLabel(
	overrideContext codersdk.ChatModelOverrideContext,
) string {
	switch overrideContext {
	case codersdk.ChatModelOverrideContextGeneral:
		return "general delegated child"
	case codersdk.ChatModelOverrideContextExplore:
		return "explore"
	default:
		return string(overrideContext)
	}
}

func readSubagentModelOverride(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (database.ChatOrganizationModelOverride, error) {
	switch overrideContext {
	case codersdk.ChatModelOverrideContextGeneral,
		codersdk.ChatModelOverrideContextExplore:
	default:
		return database.ChatOrganizationModelOverride{}, xerrors.Errorf(
			"unsupported subagent model override context %q",
			overrideContext,
		)
	}
	return db.GetChatOrganizationModelOverride(ctx, database.GetChatOrganizationModelOverrideParams{
		OrganizationID: organizationID,
		Context:        string(overrideContext),
	})
}

func personalModelOverrideContextForSubagent(
	overrideContext codersdk.ChatModelOverrideContext,
) (codersdk.ChatPersonalModelOverrideContext, error) {
	switch overrideContext {
	case codersdk.ChatModelOverrideContextGeneral:
		return codersdk.ChatPersonalModelOverrideContextGeneral, nil
	case codersdk.ChatModelOverrideContextExplore:
		return codersdk.ChatPersonalModelOverrideContextExplore, nil
	default:
		return "", xerrors.Errorf(
			"unknown subagent model override context %q",
			overrideContext,
		)
	}
}

func userCanUseProviderKeys(
	providerKeys chatprovider.ProviderAPIKeys,
	providerName string,
) bool {
	return providerKeys.APIKey(providerName) != "" ||
		(chatprovider.ProviderAllowsAmbientCredentials(providerName) &&
			providerKeys.HasProvider(providerName))
}

type modelOverrideFailureMode int

const (
	modelOverrideFailureModeSoft modelOverrideFailureMode = iota
	modelOverrideFailureModeHard
)

func modelOverrideErrorLabel(overrideContext string) string {
	return strings.ReplaceAll(overrideContext, "_", " ")
}

// resolveConfiguredModelOverride returns ok when a usable override is
// resolved. In hard failure mode, ok is also true for configured but unusable
// overrides so callers can distinguish them from unset or malformed values.
// The normalized provider name is only meaningful for a usable override.
func (p *Server) resolveConfiguredModelOverride(
	ctx context.Context,
	overrideContext string,
	raw string,
	ownerID uuid.UUID,
	resolveModelConfig modelOverrideConfigResolver,
	resolveProviderKeys modelOverrideProviderKeysResolver,
	failureMode modelOverrideFailureMode,
) (database.ChatModelConfig, string, *string, bool, error) {
	parsed, ok := parseModelOverride(raw)
	if !ok {
		p.logger.Info(ctx,
			"invalid model override, ignoring",
			slog.F("override_context", overrideContext),
			slog.F("raw_model_config_id", strings.TrimSpace(raw)),
		)
		return database.ChatModelConfig{}, "", nil, false, nil
	}
	if parsed.modelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, "", nil, false, nil
	}

	modelConfig, providerName, err := resolveModelConfig(
		ctx,
		parsed.modelConfigID,
	)
	if err != nil {
		if failureMode == modelOverrideFailureModeHard {
			label := modelOverrideErrorLabel(overrideContext)
			switch {
			case errors.Is(err, sql.ErrNoRows), errors.Is(err, errModelConfigOutsideOrganization):
				return database.ChatModelConfig{}, "", parsed.reasoningEffort, true, xerrors.Errorf(
					"%s model override is unavailable: %s: %w",
					label,
					parsed.modelConfigID,
					err,
				)
			case errors.Is(err, errInvalidModelOverrideMetadata):
				return database.ChatModelConfig{}, "", parsed.reasoningEffort, true, xerrors.Errorf(
					"%s model override metadata is invalid for %s: %w",
					label,
					parsed.modelConfigID,
					err,
				)
			default:
				return database.ChatModelConfig{}, "", parsed.reasoningEffort, true, xerrors.Errorf(
					"resolve %s model override %s: %w",
					label,
					parsed.modelConfigID,
					err,
				)
			}
		}

		switch {
		case errors.Is(err, errModelConfigOutsideOrganization):
			p.logger.Info(ctx,
				"model override belongs to another organization, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", parsed.modelConfigID),
			)
		case errors.Is(err, sql.ErrNoRows):
			p.logger.Info(ctx,
				"model override is unavailable, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", parsed.modelConfigID),
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			p.logger.Info(ctx,
				"model override metadata is invalid, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", parsed.modelConfigID),
				slog.Error(err),
			)
		default:
			p.logger.Warn(ctx,
				"failed to resolve model override, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", parsed.modelConfigID),
				slog.Error(err),
			)
		}
		return database.ChatModelConfig{}, "", nil, false, nil
	}

	providerKeys, err := resolveProviderKeys(ctx, ownerID, modelConfigAIProviderID(modelConfig))
	if err != nil {
		return database.ChatModelConfig{}, "", nil, false, xerrors.Errorf(
			"resolve provider API keys: %w",
			err,
		)
	}
	if !userCanUseProviderKeys(providerKeys, providerName) {
		if failureMode == modelOverrideFailureModeHard {
			return database.ChatModelConfig{}, "", parsed.reasoningEffort, true, xerrors.Errorf(
				"%s model override credentials are unavailable for provider %q",
				modelOverrideErrorLabel(overrideContext),
				providerName,
			)
		}

		p.logger.Info(ctx,
			"model override credentials are unavailable, ignoring",
			slog.F("override_context", overrideContext),
			slog.F("model_config_id", parsed.modelConfigID),
			slog.F("provider", providerName),
		)
		return database.ChatModelConfig{}, "", nil, false, nil
	}
	return modelConfig, providerName, parsed.reasoningEffort, true, nil
}

// resolveOrganizationModelOverride resolves an override row whose composite
// foreign key already binds the model config to the same organization.
func (p *Server) resolveOrganizationModelOverride(
	ctx context.Context,
	overrideContext string,
	override database.ChatOrganizationModelOverride,
	ownerID uuid.UUID,
	resolveModelConfig modelOverrideConfigResolver,
	resolveProviderKeys modelOverrideProviderKeysResolver,
	failureMode modelOverrideFailureMode,
) (database.ChatModelConfig, string, *string, bool, error) {
	modelConfig, providerName, err := resolveModelConfig(ctx, override.ModelConfigID)
	if err != nil {
		if failureMode == modelOverrideFailureModeHard {
			label := modelOverrideErrorLabel(overrideContext)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return database.ChatModelConfig{}, "", ptr.FromNullString(override.ReasoningEffort), true, xerrors.Errorf(
					"%s model override is unavailable: %s",
					label,
					override.ModelConfigID,
				)
			case errors.Is(err, errInvalidModelOverrideMetadata):
				return database.ChatModelConfig{}, "", ptr.FromNullString(override.ReasoningEffort), true, xerrors.Errorf(
					"%s model override metadata is invalid for %s: %w",
					label,
					override.ModelConfigID,
					err,
				)
			default:
				return database.ChatModelConfig{}, "", ptr.FromNullString(override.ReasoningEffort), true, xerrors.Errorf(
					"resolve %s model override %s: %w",
					label,
					override.ModelConfigID,
					err,
				)
			}
		}

		switch {
		case errors.Is(err, sql.ErrNoRows):
			p.logger.Info(ctx,
				"model override is unavailable, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", override.ModelConfigID),
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			p.logger.Info(ctx,
				"model override metadata is invalid, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", override.ModelConfigID),
				slog.Error(err),
			)
		default:
			p.logger.Warn(ctx,
				"failed to resolve model override, ignoring",
				slog.F("override_context", overrideContext),
				slog.F("model_config_id", override.ModelConfigID),
				slog.Error(err),
			)
		}
		return database.ChatModelConfig{}, "", nil, false, nil
	}

	providerKeys, err := resolveProviderKeys(ctx, ownerID, modelConfigAIProviderID(modelConfig))
	if err != nil {
		return database.ChatModelConfig{}, "", nil, false, xerrors.Errorf(
			"resolve provider API keys: %w",
			err,
		)
	}
	if !userCanUseProviderKeys(providerKeys, providerName) {
		if failureMode == modelOverrideFailureModeHard {
			return database.ChatModelConfig{}, "", ptr.FromNullString(override.ReasoningEffort), true, xerrors.Errorf(
				"%s model override credentials are unavailable for provider %q",
				modelOverrideErrorLabel(overrideContext),
				providerName,
			)
		}
		p.logger.Info(ctx,
			"model override credentials are unavailable, ignoring",
			slog.F("override_context", overrideContext),
			slog.F("model_config_id", override.ModelConfigID),
			slog.F("provider", providerName),
		)
		return database.ChatModelConfig{}, "", nil, false, nil
	}
	return modelConfig, providerName, ptr.FromNullString(override.ReasoningEffort), true, nil
}

func (p *Server) resolvePersonalSubagentModelConfigID(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (uuid.UUID, *string, bool, error) {
	personalContext, err := personalModelOverrideContextForSubagent(overrideContext)
	if err != nil {
		return uuid.Nil, nil, false, err
	}
	override, err := p.db.GetChatUserModelOverride(ctx, database.GetChatUserModelOverrideParams{
		UserID:         ownerID,
		OrganizationID: organizationID,
		Context:        string(personalContext),
	})
	if err != nil {
		if !xerrors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil, false, xerrors.Errorf(
				"get %s personal model override: %w",
				subagentModelOverrideLogLabel(overrideContext),
				err,
			)
		}
		return uuid.Nil, nil, false, nil
	}

	switch codersdk.ChatPersonalModelOverrideMode(override.Mode) {
	case codersdk.ChatPersonalModelOverrideModeChatDefault:
		return uuid.Nil, nil, true, nil
	case codersdk.ChatPersonalModelOverrideModeDeploymentDefault:
	case codersdk.ChatPersonalModelOverrideModeModel:
		if !override.ModelConfigID.Valid {
			p.logger.Warn(ctx,
				"personal model override has no model config, using deployment default",
				slog.F("override_context", overrideContext),
				slog.F("owner_id", ownerID),
			)
			break
		}
		modelConfig, ok, err := p.resolvePersonalModelOverride(
			ctx,
			overrideContext,
			ownerID,
			override.ModelConfigID.UUID,
		)
		if err != nil {
			return uuid.Nil, nil, false, err
		}
		if ok {
			return modelConfig.ID, ptr.FromNullString(override.ReasoningEffort), true, nil
		}
	default:
		p.logger.Warn(ctx,
			"unsupported personal model override mode, using deployment default",
			slog.F("override_context", overrideContext),
			slog.F("owner_id", ownerID),
			slog.F("mode", override.Mode),
		)
	}
	return uuid.Nil, nil, false, nil
}

func (p *Server) resolvePersonalModelOverride(
	ctx context.Context,
	overrideContext codersdk.ChatModelOverrideContext,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, bool, error) {
	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(ctx, ownerID, modelConfigID)
	if err != nil {
		switch {
		case xerrors.Is(err, sql.ErrNoRows):
			p.logger.Debug(ctx,
				"personal model override is unavailable, using deployment default",
				slog.F("override_context", overrideContext),
				slog.F("owner_id", ownerID),
				slog.F("model_config_id", modelConfigID),
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			p.logger.Debug(ctx,
				"personal model override metadata is invalid, using deployment default",
				slog.F("override_context", overrideContext),
				slog.F("owner_id", ownerID),
				slog.F("model_config_id", modelConfigID),
				slog.Error(err),
			)
		default:
			p.logger.Warn(ctx,
				"failed to resolve personal model override, using deployment default",
				slog.F("override_context", overrideContext),
				slog.F("owner_id", ownerID),
				slog.F("model_config_id", modelConfigID),
				slog.Error(err),
			)
		}
		return database.ChatModelConfig{}, false, nil
	}
	providerKeys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, modelConfigAIProviderID(modelConfig))
	if err != nil {
		return database.ChatModelConfig{}, false, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	if !userCanUseProviderKeys(providerKeys, providerName) {
		p.logger.Debug(ctx,
			"personal model override credentials are unavailable, using deployment default",
			slog.F("override_context", overrideContext),
			slog.F("owner_id", ownerID),
			slog.F("model_config_id", modelConfigID),
			slog.F("provider", providerName),
		)
		return database.ChatModelConfig{}, false, nil
	}
	return modelConfig, true, nil
}

func (p *Server) resolveSubagentModelConfigID(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (uuid.UUID, *string, error) {
	//nolint:gocritic // Chatd needs its scoped config and user-data access here.
	chatdCtx := dbauthz.AsChatd(ctx)
	personalOverridesEnabled, err := p.db.GetChatPersonalModelOverridesEnabled(chatdCtx)
	if err != nil {
		return uuid.Nil, nil, xerrors.Errorf("get chat personal model overrides enabled: %w", err)
	}
	if personalOverridesEnabled {
		modelConfigID, reasoningEffort, resolved, err := p.resolvePersonalSubagentModelConfigID(
			chatdCtx,
			ownerID,
			organizationID,
			overrideContext,
		)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if resolved {
			return modelConfigID, reasoningEffort, nil
		}
	}

	override, err := readSubagentModelOverride(chatdCtx, p.db, organizationID, overrideContext)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil, nil
		}
		return uuid.Nil, nil, xerrors.Errorf(
			"get %s model override: %w",
			subagentModelOverrideLogLabel(overrideContext),
			err,
		)
	}
	modelConfig, _, reasoningEffort, ok, err := p.resolveOrganizationModelOverride(
		chatdCtx,
		string(overrideContext),
		override,
		ownerID,
		func(ctx context.Context, modelConfigID uuid.UUID) (database.ChatModelConfig, string, error) {
			return p.resolveModelConfigAndNormalizedProvider(ctx, ownerID, modelConfigID)
		},
		p.resolveUserProviderAPIKeys,
		modelOverrideFailureModeSoft,
	)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if !ok {
		return uuid.Nil, nil, nil
	}
	return modelConfig.ID, reasoningEffort, nil
}

func modelConfigAIProviderID(modelConfig database.ChatModelConfig) uuid.UUID {
	if !modelConfig.AIProviderID.Valid {
		return uuid.Nil
	}
	return modelConfig.AIProviderID.UUID
}

func (p *Server) resolveModelConfigAndNormalizedProvider(
	ctx context.Context,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, string, error) {
	if modelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	modelCtx, err := p.callerModelConfigContext(ctx, ownerID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	modelConfig, err := p.db.GetChatModelConfigByID(modelCtx, modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	return p.resolveNormalizedProviderForModelConfig(ctx, modelConfig)
}

func (p *Server) resolveNormalizedProviderForModelConfig(
	ctx context.Context,
	modelConfig database.ChatModelConfig,
) (database.ChatModelConfig, string, error) {
	if !modelConfig.Enabled {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	if modelConfig.AIProviderID.Valid {
		//nolint:gocritic // Provider configuration remains a privileged Chatd read.
		provider, err := p.db.GetAIProviderByID(dbauthz.AsChatd(ctx), modelConfig.AIProviderID.UUID)
		if err != nil {
			return database.ChatModelConfig{}, "", err
		}
		if !provider.Enabled {
			return database.ChatModelConfig{}, "", sql.ErrNoRows
		}
		providerName := chatprovider.NormalizeProvider(string(provider.Type))
		if providerName == "" {
			return database.ChatModelConfig{}, "", errInvalidModelOverrideMetadata
		}
		if _, _, err := chatprovider.ResolveModelWithProviderHint(modelConfig.Model, providerName); err != nil {
			return database.ChatModelConfig{}, "", errInvalidModelOverrideMetadata
		}
		return modelConfig, providerName, nil
	}
	// Active configs carry a provider FK; resolved above. Missing FK means no usable config.
	return database.ChatModelConfig{}, "", sql.ErrNoRows
}

func (p *Server) resolveExplicitSpawnOverrides(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	args spawnAgentArgs,
) (*uuid.UUID, *string, error) {
	var explicitModelConfigID *uuid.UUID
	if raw := strings.TrimSpace(args.ModelConfigID); raw != "" {
		modelConfigID, err := uuid.Parse(raw)
		if err != nil {
			return nil, nil, xerrors.New(
				"invalid model_config_id: must be a valid UUID; use " +
					listSubagentModelsToolName + " to see available models",
			)
		}
		modelConfig, providerName, err := p.resolveModelConfigForOrganization(
			ctx,
			ownerID,
			organizationID,
			modelConfigID,
		)
		if err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return nil, nil, xerrors.New(
					"model_config_id not found or is disabled; use " +
						listSubagentModelsToolName + " to see available models",
				)
			case errors.Is(err, errInvalidModelOverrideMetadata):
				return nil, nil, xerrors.Errorf(
					"model_config_id metadata is invalid: %w",
					err,
				)
			default:
				p.logger.Warn(ctx, "failed to resolve spawn_agent model_config_id",
					slog.F("model_config_id", modelConfigID),
					slog.Error(err),
				)
				return nil, nil, xerrors.New("internal error looking up model config")
			}
		}
		//nolint:gocritic // Provider credentials remain privileged Chatd reads.
		providerKeys, err := p.resolveUserProviderAPIKeys(
			dbauthz.AsChatd(ctx),
			ownerID,
			modelConfigAIProviderID(modelConfig),
		)
		if err != nil {
			p.logger.Warn(ctx, "failed to resolve provider API keys for spawn_agent model_config_id",
				slog.F("model_config_id", modelConfigID),
				slog.Error(err),
			)
			return nil, nil, xerrors.New("internal error looking up model config")
		}
		if !userCanUseProviderKeys(providerKeys, providerName) {
			return nil, nil, xerrors.Errorf(
				"model_config_id credentials are unavailable for provider %q",
				providerName,
			)
		}
		explicitModelConfigID = &modelConfig.ID
	}

	var explicitReasoningEffort *string
	if raw := strings.TrimSpace(args.ReasoningEffort); raw != "" {
		effort := strings.ToLower(raw)
		if !chatprovider.IsValidReasoningEffort(effort) {
			return nil, nil, xerrors.Errorf(
				"invalid reasoning_effort: must be one of %s",
				strings.Join(codersdk.ChatModelReasoningEffortValues(), ", "),
			)
		}
		explicitReasoningEffort = &effort
	}

	return explicitModelConfigID, explicitReasoningEffort, nil
}

func (p *Server) listSpawnableModelConfigs(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
) ([]map[string]any, error) {
	modelCtx, err := p.callerModelConfigContext(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	rows, err := enabledChatModelConfigsForOrganization(modelCtx, p.db, organizationID)
	if err != nil {
		return nil, xerrors.Errorf("get enabled chat model configs: %w", err)
	}
	models := make([]map[string]any, 0, len(rows))
	providerKeysByID := make(map[uuid.UUID]chatprovider.ProviderAPIKeys)
	for _, row := range rows {
		providerName := chatprovider.NormalizeProvider(row.Provider)
		if providerName == "" {
			continue
		}
		if _, _, err := chatprovider.ResolveModelWithProviderHint(
			row.ChatModelConfig.Model,
			providerName,
		); err != nil {
			continue
		}
		providerID := modelConfigAIProviderID(row.ChatModelConfig)
		providerKeys, ok := providerKeysByID[providerID]
		if !ok {
			//nolint:gocritic // Provider credentials remain privileged Chatd reads.
			providerKeys, err = p.resolveUserProviderAPIKeys(
				dbauthz.AsChatd(ctx),
				ownerID,
				providerID,
			)
			if err != nil {
				return nil, xerrors.Errorf("resolve provider API keys: %w", err)
			}
			providerKeysByID[providerID] = providerKeys
		}
		if !userCanUseProviderKeys(providerKeys, providerName) {
			continue
		}
		entry := map[string]any{
			"model_config_id": row.ChatModelConfig.ID.String(),
			"display_name":    row.ChatModelConfig.DisplayName,
			"model":           row.ChatModelConfig.Model,
			"provider":        providerName,
			"context_limit":   row.ChatModelConfig.ContextLimit,
			"is_default":      row.ChatModelConfig.IsDefault,
		}
		callConfig := codersdk.ChatModelCallConfig{}
		if len(row.ChatModelConfig.Options) > 0 {
			if err := json.Unmarshal(row.ChatModelConfig.Options, &callConfig); err == nil {
				if efforts := chatprovider.SelectableReasoningEfforts(callConfig.ReasoningEffort); len(efforts) > 0 {
					entry["reasoning_efforts"] = efforts
				}
			}
		}
		models = append(models, entry)
	}
	return models, nil
}

func (p *Server) subagentTools(
	ctx context.Context,
	currentChat func() database.Chat,
	currentModelConfigID uuid.UUID,
) []fantasy.AgentTool {
	manager := newSubagentManager(p, currentChat, currentModelConfigID)
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			spawnAgentToolName,
			buildSpawnAgentDescription(ctx, p, manager.currentChatSnapshot),
			func(ctx context.Context, args spawnAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				chat, err := manager.Spawn(ctx, args)
				if _, ok := errors.AsType[*dispatch.Error](err); ok {
					return fantasy.ToolResponse{}, err
				}
				return subagentManagerToolResponse(spawnAgentResult(chat), err)
			},
		),
		fantasy.NewAgentTool(
			listSubagentModelsToolName,
			"List the enabled model configurations available for "+
				spawnAgentToolName+"'s model_config_id argument. Only models "+
				"usable with the chat owner's credentials are returned. Each "+
				"entry includes model_config_id, display_name, model, "+
				"provider, context_limit, is_default, and reasoning_efforts "+
				"(the values accepted by "+spawnAgentToolName+"'s "+
				"reasoning_effort for that model).",
			func(ctx context.Context, _ listSubagentModelsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				models, err := manager.ListModels(ctx)
				return subagentManagerToolResponse(subagentModelsResult{Models: models}, err)
			},
		),
		fantasy.NewAgentTool(
			"wait_agent",
			"Wait for a spawned child agent to finish and return its response "+
				"and status. Returns immediately when the agent finishes, even if "+
				"a longer timeout is set. A timeout does not stop the agent; call "+
				"wait_agent again or use list_agents to check its status.",
			func(ctx context.Context, args waitAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				result, err := manager.Await(ctx, args)
				return subagentManagerToolResponse(result, err)
			},
		),
		fantasy.NewAgentTool(
			"message_agent",
			"Send a follow-up message to a previously spawned child "+
				"agent. If the agent is idle, it resumes work on the "+
				"message. If the agent is busy, the message is queued and "+
				"processed after current work. Set interrupt to true to "+
				"stop the agent's current work; the message is queued and "+
				"processed next, after any already-queued messages. "+
				"After sending, use wait_agent to retrieve the response.",
			func(ctx context.Context, args messageAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				chat, interrupted, err := manager.Send(ctx, args)
				return subagentManagerToolResponse(subagentActionResult(chat, interrupted), err)
			},
		),
		fantasy.NewAgentTool(
			"interrupt_agent",
			"Interrupt a spawned child agent's current work. The "+
				"status may briefly read interrupting before transitioning "+
				"to waiting, or running if there are queued messages. "+
				"Resume with message_agent or leave it idle.",
			func(ctx context.Context, args interruptAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				chat, interrupted, err := manager.Interrupt(ctx, args)
				return subagentManagerToolResponse(subagentActionResult(chat, interrupted), err)
			},
		),
		fantasy.NewAgentTool(
			"list_agents",
			"List the child agents spawned by this chat, most recently "+
				"active first. Returns up to `limit` agents (default 10) "+
				"with `total` and `has_more`; use `offset` to page. The "+
				"sort order is best-effort: an agent's position may shift "+
				"if its updated_at changes between calls. Each "+
				"agent has chat_id, title, type, status, created_at, "+
				"updated_at. Status: running = working, "+
				"interrupting = transient, waiting = idle, "+
				"error = stopped on error.",
			func(ctx context.Context, args listAgentsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				result, err := manager.List(ctx, args)
				return subagentManagerToolResponse(result, err)
			},
		),
	}
}

type subagentModelsResult struct {
	Models []map[string]any `json:"models"`
}

func spawnAgentResult(chat database.Chat) any {
	return struct {
		ChatID string `json:"chat_id"`
		Status string `json:"status"`
		Title  string `json:"title"`
		Type   string `json:"type"`
	}{
		ChatID: chat.ID.String(), Status: string(chat.Status),
		Title: chat.Title, Type: subagentTypeFromChat(chat),
	}
}

func subagentActionResult(chat database.Chat, interrupted bool) any {
	return struct {
		ChatID      string `json:"chat_id"`
		Interrupted bool   `json:"interrupted"`
		Status      string `json:"status"`
		Title       string `json:"title"`
		Type        string `json:"type"`
	}{
		ChatID: chat.ID.String(), Interrupted: interrupted, Status: string(chat.Status),
		Title: chat.Title, Type: subagentTypeFromChat(chat),
	}
}

func subagentManagerToolResponse(result any, err error) (fantasy.ToolResponse, error) {
	if err == nil {
		return toolJSONResponse(result), nil
	}
	var managerErr *subagentManagerError
	if errors.As(err, &managerErr) {
		return subagentErrorResponse(managerErr.err, managerErr.chat), nil
	}
	return fantasy.NewTextErrorResponse(err.Error()), nil
}

func toolJSONResponse(result any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}

func toolJSONErrorResponse(result map[string]any) fantasy.ToolResponse {
	resp := toolJSONResponse(result)
	resp.IsError = true
	return resp
}
