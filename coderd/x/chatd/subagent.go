package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

var errInvalidModelOverrideMetadata = xerrors.New("invalid model override metadata")

type modelOverrideConfigResolver func(
	context.Context,
	uuid.UUID,
) (database.ChatModelConfig, string, error)

type modelOverrideProviderKeysResolver func(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (chatprovider.ProviderAPIKeys, error)

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
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
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
	overrideContext codersdk.ChatModelOverrideContext,
) (string, error) {
	switch overrideContext {
	case codersdk.ChatModelOverrideContextGeneral:
		return db.GetChatGeneralModelOverride(ctx)
	case codersdk.ChatModelOverrideContextExplore:
		return db.GetChatExploreModelOverride(ctx)
	default:
		return "", xerrors.Errorf(
			"unsupported subagent model override context %q",
			overrideContext,
		)
	}
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
			case errors.Is(err, sql.ErrNoRows):
				return database.ChatModelConfig{}, "", parsed.reasoningEffort, true, xerrors.Errorf(
					"%s model override is unavailable: %s",
					label,
					parsed.modelConfigID,
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

func (p *Server) resolvePersonalSubagentModelConfigID(
	ctx context.Context,
	ownerID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (uuid.UUID, *string, bool, error) {
	personalContext, err := personalModelOverrideContextForSubagent(overrideContext)
	if err != nil {
		return uuid.Nil, nil, false, err
	}
	raw, err := p.db.GetUserChatPersonalModelOverride(
		ctx,
		database.GetUserChatPersonalModelOverrideParams{
			UserID: ownerID,
			Key:    ChatPersonalModelOverrideKey(personalContext),
		},
	)
	if err != nil {
		if !xerrors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil, false, xerrors.Errorf(
				"get %s personal model override: %w",
				subagentModelOverrideLogLabel(overrideContext),
				err,
			)
		}
		raw = ""
	}

	parsed := ParseChatPersonalModelOverride(
		raw,
		codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
	)
	if parsed.Malformed {
		p.logger.Debug(ctx,
			"personal model override is malformed, using deployment default",
			slog.F("override_context", overrideContext),
			slog.F("owner_id", ownerID),
			slog.F("raw_model_config_id", strings.TrimSpace(raw)),
		)
	}
	switch parsed.Mode {
	case codersdk.ChatPersonalModelOverrideModeChatDefault:
		return uuid.Nil, nil, true, nil
	case codersdk.ChatPersonalModelOverrideModeDeploymentDefault:
	case codersdk.ChatPersonalModelOverrideModeModel:
		modelConfig, ok, err := p.resolvePersonalModelOverride(
			ctx,
			overrideContext,
			ownerID,
			parsed.ModelConfigID,
		)
		if err != nil {
			return uuid.Nil, nil, false, err
		}
		if ok {
			return modelConfig.ID, parsed.ReasoningEffort, true, nil
		}
	default:
		p.logger.Warn(ctx,
			"unsupported personal model override mode, using deployment default",
			slog.F("override_context", overrideContext),
			slog.F("owner_id", ownerID),
			slog.F("mode", parsed.Mode),
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
	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(
		ctx,
		modelConfigID,
	)
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
		return database.ChatModelConfig{}, false, xerrors.Errorf(
			"resolve provider API keys: %w",
			err,
		)
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

func withResolvedReasoningEffort(
	modelConfig database.ChatModelConfig,
	reasoningEffort *string,
) database.ChatModelConfig {
	if reasoningEffort == nil {
		return modelConfig
	}
	callConfig := codersdk.ChatModelCallConfig{}
	if len(modelConfig.Options) > 0 {
		if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
			return modelConfig
		}
	}
	resolvedEffort := chatprovider.ResolveReasoningEffort(
		reasoningEffort,
		callConfig.ReasoningEffort,
	)
	if resolvedEffort == nil {
		return modelConfig
	}
	callConfig.ReasoningEffort = &codersdk.ChatModelReasoningEffortConfig{
		Default: resolvedEffort,
		Max:     resolvedEffort,
	}
	options, err := json.Marshal(callConfig)
	if err != nil {
		return modelConfig
	}
	modelConfig.Options = options
	return modelConfig
}

func (p *Server) resolveSubagentModelConfigID(
	ctx context.Context,
	ownerID uuid.UUID,
	overrideContext codersdk.ChatModelOverrideContext,
) (uuid.UUID, *string, error) {
	//nolint:gocritic // Chatd needs its scoped config and user-data access here.
	chatdCtx := dbauthz.AsChatd(ctx)
	personalOverridesEnabled, err := p.db.GetChatPersonalModelOverridesEnabled(chatdCtx)
	if err != nil {
		return uuid.Nil, nil, xerrors.Errorf(
			"get chat personal model overrides enabled: %w",
			err,
		)
	}
	if personalOverridesEnabled {
		modelConfigID, reasoningEffort, resolved, err := p.resolvePersonalSubagentModelConfigID(
			chatdCtx,
			ownerID,
			overrideContext,
		)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if resolved {
			return modelConfigID, reasoningEffort, nil
		}
	}

	raw, err := readSubagentModelOverride(chatdCtx, p.db, overrideContext)
	if err != nil {
		return uuid.Nil, nil, xerrors.Errorf(
			"get %s model override: %w",
			subagentModelOverrideLogLabel(overrideContext),
			err,
		)
	}
	modelConfig, _, reasoningEffort, ok, err := p.resolveConfiguredModelOverride(
		chatdCtx,
		string(overrideContext),
		raw,
		ownerID,
		p.resolveModelConfigAndNormalizedProvider,
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
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, string, error) {
	if modelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	modelConfig, err := p.configCache.ModelConfigByID(ctx, modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	if !modelConfig.Enabled {
		return database.ChatModelConfig{}, "", sql.ErrNoRows
	}
	if modelConfig.AIProviderID.Valid {
		provider, err := p.db.GetAIProviderByID(ctx, modelConfig.AIProviderID.UUID)
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

func (p *Server) subagentTools(
	ctx context.Context,
	currentChat func() database.Chat,
	currentModelConfigID uuid.UUID,
) []fantasy.AgentTool {
	currentChatSnapshot := database.Chat{}
	if currentChat != nil {
		currentChatSnapshot = currentChat()
	}

	spawnAgentDescription := buildSpawnAgentDescription(
		ctx,
		p,
		currentChatSnapshot,
	)

	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			spawnAgentToolName,
			spawnAgentDescription,
			func(ctx context.Context, args spawnAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				parent, err := p.loadSubagentSpawnParentChat(ctx, currentChat)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				definition, err := resolveSubagentDefinition(
					ctx,
					p,
					parent,
					args.Type,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				turnParent := currentChatSnapshot
				if turnParent.ID == uuid.Nil {
					turnParent = parent
				}

				options, err := definition.buildOptions(
					ctx,
					p,
					parent,
					turnParent,
					currentModelConfigID,
					args.Prompt,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				childChat, err := p.createChildSubagentChatWithOptions(
					ctx,
					parent,
					args.Prompt,
					args.Title,
					options,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(withSubagentType(map[string]any{
					"chat_id": childChat.ID.String(),
					"title":   childChat.Title,
					"status":  string(childChat.Status),
				}, childChat)), nil
			},
		),
		fantasy.NewAgentTool(
			"wait_agent",
			"Wait until a spawned child agent finishes its task. "+
				"Returns the agent's response and status. A timeout is not "+
				"a failure: the agent is still running. Call wait_agent again "+
				"or use list_agents to check its status.",
			func(ctx context.Context, args waitAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				timeout := defaultSubagentWaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				parent := currentChat()
				var targetChatInfo *database.Chat
				if chat, lookupErr := p.db.GetChatByID(ctx, targetChatID); lookupErr == nil {
					targetChatInfo = &chat
				} else if !xerrors.Is(lookupErr, sql.ErrNoRows) {
					p.logger.Warn(ctx, "unexpected error looking up chat for recording",
						slog.F("chat_id", targetChatID),
						slog.Error(lookupErr),
					)
				}

				// Authorize: the target chat must be a descendant
				// of the current (parent) chat.
				isDescendant, descErr := isSubagentDescendant(ctx, p.db, parent.ID, targetChatID)
				if descErr != nil {
					return subagentErrorResponse(
						xerrors.New(fmt.Sprintf("failed to verify subagent relationship: %v", descErr)),
						targetChatInfo,
					), nil
				}
				if !isDescendant {
					return subagentErrorResponse(
						ErrSubagentNotDescendant,
						targetChatInfo,
					), nil
				}

				// Check if the target is a computer_use subagent
				// and start a desktop recording. Failures are
				// best-effort warnings. Recording never blocks
				// the wait_agent flow.
				var recordingID string
				var agentConn workspacesdk.AgentConn

				isComputerUseChat := targetChatInfo != nil &&
					targetChatInfo.Mode.Valid &&
					targetChatInfo.Mode.ChatMode == database.ChatModeComputerUse &&
					targetChatInfo.AgentID.Valid
				canRecord := isComputerUseChat && p.agentConnFn != nil

				if canRecord {
					conn, closeFn, connErr := p.agentConnFn(ctx, targetChatInfo.AgentID.UUID)
					if connErr == nil {
						agentConn = conn
						defer closeFn()

						recordingID = targetChatID.String()
						startErr := conn.StartDesktopRecording(ctx,
							workspacesdk.StartDesktopRecordingRequest{RecordingID: recordingID})
						if startErr != nil {
							p.logger.Warn(ctx, "failed to start desktop recording",
								slog.Error(startErr))
							recordingID = ""
						}
					} else {
						p.logger.Warn(ctx, "failed to get agent conn for recording",
							slog.Error(connErr))
					}
				}

				targetChat, report, awaitErr := p.awaitSubagentCompletion(
					ctx, parent.ID, targetChatID, timeout,
				)

				// On timeout or error, leave the recording running on
				// the agent so the next wait_agent call continues it.
				if awaitErr != nil {
					if xerrors.Is(awaitErr, ErrSubagentWaitTimeout) {
						// The agent may have completed in the gap between
						// the last poll and the timer firing. Re-check
						// completion with a fresh DB read to avoid acting
						// on a stale status (TOCTOU).
						checkedChat, checkedReport, done, checkErr := p.checkSubagentCompletion(ctx, targetChatID)
						if checkErr != nil {
							return subagentErrorResponse(checkErr, targetChatInfo), nil
						}
						if !done {
							return toolJSONResponse(withSubagentType(map[string]any{
								"chat_id":   targetChatID.String(),
								"title":     checkedChat.Title,
								"status":    string(checkedChat.Status),
								"timed_out": true,
							}, checkedChat)), nil
						}
						// The agent completed in the gap. Classify through
						// the same handler as the normal poll path. If the
						// agent errored, handleSubagentDone returns a
						// subagentStatusError that the error-status block
						// below catches.
						targetChat, report, awaitErr = handleSubagentDone(checkedChat, checkedReport)
						if awaitErr == nil {
							return p.waitAgentSuccessResponse(ctx, recordingID, agentConn, parent, targetChat, report), nil
						}
					}
					if errStatus, ok := errors.AsType[*subagentStatusError](awaitErr); ok {
						errChat := errStatus.chat
						lastError := subagentLastErrorMessage(errChat.LastError)
						if lastError == "" {
							lastError = errStatus.reason
						}
						return toolJSONResponse(withSubagentType(map[string]any{
							"chat_id":    errChat.ID.String(),
							"title":      errChat.Title,
							"status":     string(errChat.Status),
							"last_error": lastError,
							"report":     errStatus.report,
						}, errChat)), nil
					}
					return subagentErrorResponse(awaitErr, targetChatInfo), nil
				}

				// Only stop and store the recording on success.
				return p.waitAgentSuccessResponse(ctx, recordingID, agentConn, parent, targetChat, report), nil
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
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				var targetChatInfo *database.Chat
				if chat, lookupErr := p.db.GetChatByID(ctx, targetChatID); lookupErr == nil {
					targetChatInfo = &chat
				} else if !xerrors.Is(lookupErr, sql.ErrNoRows) {
					p.logger.Warn(ctx, "unexpected error looking up chat for message",
						slog.F("chat_id", targetChatID),
						slog.Error(lookupErr),
					)
				}
				busyBehavior := SendMessageBusyBehaviorQueue
				if args.Interrupt {
					busyBehavior = SendMessageBusyBehaviorInterrupt
				}
				targetChat, err := p.sendSubagentMessage(
					ctx,
					parent.ID,
					targetChatID,
					args.Message,
					busyBehavior,
				)
				if err != nil {
					return subagentErrorResponse(err, targetChatInfo), nil
				}

				interrupted := false
				if args.Interrupt && targetChatInfo != nil {
					interrupted = targetChatInfo.Status == database.ChatStatusRunning
				}
				return toolJSONResponse(withSubagentType(map[string]any{
					"chat_id":     targetChat.ID.String(),
					"title":       targetChat.Title,
					"status":      string(targetChat.Status),
					"interrupted": interrupted,
				}, targetChat)), nil
			},
		),
		fantasy.NewAgentTool(
			"interrupt_agent",
			"Interrupt a spawned child agent's current work. The "+
				"status may briefly read interrupting before transitioning "+
				"to waiting, or running if there are queued messages. "+
				"Resume with message_agent or leave it idle.",
			func(ctx context.Context, args interruptAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				var targetChatInfo *database.Chat
				if chat, lookupErr := p.db.GetChatByID(ctx, targetChatID); lookupErr == nil {
					targetChatInfo = &chat
				} else if !xerrors.Is(lookupErr, sql.ErrNoRows) {
					p.logger.Warn(ctx, "unexpected error looking up chat for interrupt",
						slog.F("chat_id", targetChatID),
						slog.Error(lookupErr),
					)
				}
				targetChat, interrupted, err := p.interruptSubagent(
					ctx,
					parent.ID,
					targetChatID,
				)
				if err != nil {
					return subagentErrorResponse(err, targetChatInfo), nil
				}

				return toolJSONResponse(withSubagentType(map[string]any{
					"chat_id":     targetChat.ID.String(),
					"title":       targetChat.Title,
					"interrupted": interrupted,
					"status":      string(targetChat.Status),
				}, targetChat)), nil
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
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				limit := defaultListAgentsLimit
				if args.Limit != nil {
					limit = min(max(*args.Limit, 1), maxListAgentsLimit)
				}
				offset := 0
				if args.Offset != nil && *args.Offset > 0 {
					offset = *args.Offset
				}

				parent := currentChat()
				if parent.ParentChatID.Valid {
					return fantasy.NewTextErrorResponse("list_agents is only available on root chats"), nil
				}
				rows, err := p.db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
					ParentIds: []uuid.UUID{parent.ID},
					// Exclude archived children by default. Do not pass an
					// invalid NullBool, which would include archived rows.
					Archived: sql.NullBool{Bool: false, Valid: true},
				})
				if err != nil {
					return fantasy.NewTextErrorResponse(xerrors.Errorf("list child agents: %w", err).Error()), nil
				}

				slices.SortStableFunc(rows, func(a, b database.GetChildChatsByParentIDsRow) int {
					if c := b.Chat.UpdatedAt.Compare(a.Chat.UpdatedAt); c != 0 {
						return c
					}
					return strings.Compare(b.Chat.ID.String(), a.Chat.ID.String())
				})

				total := len(rows)
				start := min(offset, total)
				end := min(start+limit, total)
				page := rows[start:end]

				agents := make([]map[string]any, 0, len(page))
				for _, row := range page {
					child := row.Chat
					agents = append(agents, withSubagentType(map[string]any{
						"chat_id":    child.ID.String(),
						"title":      child.Title,
						"status":     string(child.Status),
						"created_at": child.CreatedAt.Format(time.RFC3339),
						"updated_at": child.UpdatedAt.Format(time.RFC3339),
					}, child))
				}

				return toolJSONResponse(map[string]any{
					"agents":   agents,
					"total":    total,
					"returned": len(agents),
					"offset":   offset,
					"has_more": end < total,
				}), nil
			},
		),
	}
}

func (p *Server) loadSubagentSpawnParentChat(
	ctx context.Context,
	currentChat func() database.Chat,
) (database.Chat, error) {
	parent := currentChat()
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}
	reloadedParent, err := p.db.GetChatByID(ctx, parent.ID)
	if err != nil {
		p.logger.Warn(ctx, "failed to load parent chat for spawn_agent",
			slog.F("chat_id", parent.ID),
			slog.Error(err),
		)
		return database.Chat{}, xerrors.New("failed to load parent chat")
	}
	parent = reloadedParent
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}

	return parent, nil
}

func parseSubagentToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

// childSubagentChatOptions carries per-child overrides for subagent chat
// creation. modelConfigIDOverride, reasoningEffortOverride, and
// planModeOverride apply to any subagent. inheritedMCPServerIDs is an
// Explore-only snapshot of the spawning parent turn's effective external MCP
// entitlement. resolveExploreToolSnapshot computes and persists it on the
// child chat. Non-Explore children ignore this field.
type childSubagentChatOptions struct {
	chatMode                database.NullChatMode
	systemPrompt            string
	modelConfigIDOverride   *uuid.UUID
	reasoningEffortOverride *string
	planModeOverride        *database.NullChatPlanMode
	inheritedMCPServerIDs   []uuid.UUID
}

// resolveExploreToolSnapshot computes the child chat's inherited MCP
// server snapshot from the spawning parent turn.
//
// The MCP set is filtered in two stages. First,
// filterExternalMCPConfigsForTurn applies the parent turn's plan-mode
// policy to the parent's MCP configs, producing visibleConfigs. Second,
// if the parent is itself an Explore child, the visible set is narrowed to
// the parent's persisted MCPServerIDs so an Explore chain cannot
// re-escalate beyond the original grant. Non-Explore parents pass
// through the second stage unchanged.
func (p *Server) resolveExploreToolSnapshot(
	ctx context.Context,
	parent database.Chat,
) ([]uuid.UUID, error) {
	inheritedMCPServerIDs := []uuid.UUID{}
	if len(parent.MCPServerIDs) > 0 {
		configs, err := p.db.GetMCPServerConfigsByIDs(ctx, parent.MCPServerIDs)
		if err != nil {
			return nil, xerrors.Errorf("get parent MCP server configs for chat %s: %w", parent.ID, err)
		}

		visibleConfigs, _ := filterExternalMCPConfigsForTurn(
			configs,
			parent.PlanMode,
			parent.ParentChatID,
		)
		// Empty means the parent is not Explore, so all plan-filtered
		// configs remain eligible. Populated means the parent is
		// Explore, so only its persisted snapshot can pass.
		allowedParentIDs := map[uuid.UUID]struct{}{}
		if isExploreSubagentMode(parent.Mode) {
			for _, id := range parent.MCPServerIDs {
				allowedParentIDs[id] = struct{}{}
			}
		}
		for _, cfg := range visibleConfigs {
			if len(allowedParentIDs) > 0 {
				if _, ok := allowedParentIDs[cfg.ID]; !ok {
					continue
				}
			}
			inheritedMCPServerIDs = append(inheritedMCPServerIDs, cfg.ID)
		}
	}

	return inheritedMCPServerIDs, nil
}

func (p *Server) createChildSubagentChatWithOptions(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
	opts childSubagentChatOptions,
) (database.Chat, error) {
	if parent.ParentChatID.Valid {
		return database.Chat{}, xerrors.New("delegated chats cannot create child subagents")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, xerrors.New("prompt is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = subagentFallbackChatTitle(prompt)
	}

	rootChatID := parent.ID
	if parent.RootChatID.Valid {
		rootChatID = parent.RootChatID.UUID
	}

	modelConfigID := parent.LastModelConfigID
	if opts.modelConfigIDOverride != nil {
		modelConfigID = *opts.modelConfigIDOverride
	}
	if modelConfigID == uuid.Nil {
		return database.Chat{}, xerrors.New("model config is required")
	}
	childAPIKeyID, err := p.ensureSyntheticAPIKeyID(ctx, parent.OwnerID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("ensure synthetic API key: %w", err)
	}

	childPlanMode := parent.PlanMode
	if opts.planModeOverride != nil {
		childPlanMode = *opts.planModeOverride
	}

	mcpServerIDs := parent.MCPServerIDs
	if isExploreSubagentMode(opts.chatMode) {
		mcpServerIDs = slices.Clone(opts.inheritedMCPServerIDs)
	}
	if mcpServerIDs == nil {
		mcpServerIDs = []uuid.UUID{}
	}

	labelsJSON, err := json.Marshal(database.StringMap{})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal labels: %w", err)
	}
	childSystemPrompt := SanitizePromptText(opts.systemPrompt)
	// Resolve the deployment prompt before opening the transaction so
	// child chat creation does not hold one DB connection while waiting
	// for another pool checkout.
	deploymentPrompt := p.resolveDeploymentSystemPrompt(ctx)
	// Delegated chats cannot call list_agents or message_agent, so
	// strip the root-only orchestration guidance from their prompt.
	deploymentPrompt = strings.Replace(deploymentPrompt, subagentOrchestrationPromptBlock, "", 1)

	if limitErr := p.checkUsageLimit(ctx, p.db, parent.OwnerID, uuid.NullUUID{UUID: parent.OrganizationID, Valid: true}); limitErr != nil {
		return database.Chat{}, limitErr
	}

	workspaceAwareness := workspaceDetachedNoCreateAwareness
	if parent.WorkspaceID.Valid {
		workspaceAwareness = workspaceAttachedAwareness
	}
	workspaceAwarenessContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(workspaceAwareness),
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal workspace awareness: %w", err)
	}
	userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal initial user content: %w", err)
	}

	initialMessages := make([]chatstate.Message, 0, 4)
	if deploymentPrompt != "" {
		deploymentContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(deploymentPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal deployment system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(deploymentContent, modelConfigID))
	}
	if childSystemPrompt != "" {
		childSystemPromptContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(childSystemPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal child system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(childSystemPromptContent, modelConfigID))
	}
	initialMessages = append(initialMessages, systemMessage(workspaceAwarenessContent, modelConfigID))

	// The child shares the parent's workspace and agent, so it inherits
	// workspace context the same way a top-level chat does: pinned from the
	// agent's latest snapshot (see hydrateChatContextOnCreate below). The
	// parent's context is not copied into child history.
	initialMessages = append(initialMessages, userMessageWithAPIKeyID(userContent, modelConfigID, parent.OwnerID, childAPIKeyID, opts.reasoningEffortOverride))

	publisher := p.pubsub
	if publisher == nil {
		publisher = dbpubsub.NewInMemory()
	}
	result, err := chatstate.CreateChat(ctx, p.db, publisher, chatstate.CreateChatInput{
		OrganizationID:    parent.OrganizationID,
		OwnerID:           parent.OwnerID,
		WorkspaceID:       parent.WorkspaceID,
		BuildID:           parent.BuildID,
		AgentID:           parent.AgentID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChatID, Valid: true},
		LastModelConfigID: modelConfigID,
		Title:             title,
		Mode:              opts.chatMode,
		PlanMode:          childPlanMode,
		MCPServerIDs:      mcpServerIDs,
		Labels: pqtype.NullRawMessage{
			RawMessage: labelsJSON,
			Valid:      true,
		},
		DynamicTools:    pqtype.NullRawMessage{},
		ClientType:      parent.ClientType,
		InitialMessages: initialMessages,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("create child chat: %w", err)
	}

	child := result.Chat

	// Pin the child to its agent's latest context snapshot, mirroring the
	// top-level create path. The child shares the parent's workspace agent,
	// so this reproduces the parent's workspace context without copying it
	// through chat history.
	p.hydrateChatContextOnCreate(ctx, child)

	p.publishChatPubsubEvent(child, codersdk.ChatWatchEventKindCreated, nil)
	return child, nil
}

func (p *Server) sendSubagentMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	message string,
	busyBehavior SendMessageBusyBehavior,
) (database.Chat, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return database.Chat{}, xerrors.New("message is required")
	}

	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	// Look up the target chat to get the owner for CreatedBy.
	targetChat, err := p.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get target chat: %w", err)
	}

	sendResult, err := p.SendMessage(ctx, SendMessageOptions{
		ChatID:       targetChatID,
		CreatedBy:    targetChat.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(message)},
		BusyBehavior: busyBehavior,
	})
	if err != nil {
		return database.Chat{}, err
	}

	return sendResult.Chat, nil
}

func (p *Server) awaitSubagentCompletion(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (database.Chat, string, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, "", err
	}
	if !isDescendant {
		return database.Chat{}, "", ErrSubagentNotDescendant
	}

	// Check immediately before entering the poll loop.
	targetChat, report, done, checkErr := p.checkSubagentCompletion(ctx, targetChatID)
	if checkErr != nil {
		return database.Chat{}, "", checkErr
	}
	if done {
		return handleSubagentDone(targetChat, report)
	}

	if timeout <= 0 {
		timeout = defaultSubagentWaitTimeout
	}
	timer := p.clock.NewTimer(timeout, "chatd", "subagent_await")
	defer timer.Stop()

	// Subscribe for fast status notifications and use a less
	// aggressive fallback poll. If subscription fails, fall back to
	// the original 200ms polling.
	pollInterval := subagentAwaitFallbackPoll
	ch := make(chan struct{}, 1)
	notifyCh := (<-chan struct{})(ch)
	cancel, subErr := p.pubsub.SubscribeWithErr(
		coderdpubsub.ChatStateUpdateChannel(targetChatID),
		func(_ context.Context, _ []byte, _ error) {
			// Non-blocking send so we never stall the
			// pubsub dispatch goroutine.
			select {
			case ch <- struct{}{}:
			default:
			}
		},
	)
	if subErr == nil {
		defer cancel()
	} else {
		// Subscription failed; fall back to fast polling.
		pollInterval = subagentAwaitPollInterval
		notifyCh = nil
	}

	ticker := p.clock.NewTicker(pollInterval, "chatd", "subagent_poll")
	defer ticker.Stop()

	for {
		select {
		case <-notifyCh:
		case <-ticker.C:
		case <-timer.C:
			return database.Chat{}, "", ErrSubagentWaitTimeout
		case <-ctx.Done():
			return database.Chat{}, "", ctx.Err()
		}

		targetChat, report, done, checkErr = p.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return database.Chat{}, "", checkErr
		}
		if done {
			return handleSubagentDone(targetChat, report)
		}
	}
}

// handleSubagentDone translates a completed subagent check into the
// appropriate return value. An error-status chat is returned as a typed
// subagentStatusError that carries the chat and report so the
// wait_agent handler can surface a structured, recoverable-aware payload.
func handleSubagentDone(
	chat database.Chat,
	report string,
) (database.Chat, string, error) {
	if chat.Status == database.ChatStatusError {
		reason := strings.TrimSpace(report)
		if reason == "" {
			reason = "agent reached error status"
		}
		return database.Chat{}, "", &subagentStatusError{
			chat:   chat,
			report: report,
			reason: reason,
		}
	}
	return chat, report, nil
}

// subagentLastErrorMessage extracts the normalized, user-facing message
// from a chat's last_error payload, falling back to the raw JSON when the
// payload is not a recognized ChatError.
func subagentLastErrorMessage(raw pqtype.NullRawMessage) string {
	if !raw.Valid {
		return ""
	}
	var payload codersdk.ChatError
	if err := json.Unmarshal(raw.RawMessage, &payload); err == nil && payload.Message != "" {
		return payload.Message
	}
	return string(raw.RawMessage)
}

// waitAgentSuccessResponse stops and stores the recording (if active) and
// builds the normal completion payload for a wait_agent call.
func (p *Server) waitAgentSuccessResponse(
	ctx context.Context,
	recordingID string,
	agentConn workspacesdk.AgentConn,
	parent database.Chat,
	targetChat database.Chat,
	report string,
) fantasy.ToolResponse {
	var recResult recordingResult
	if recordingID != "" && agentConn != nil {
		// Use a fresh context for cleanup so a canceled
		// parent context does not prevent recording storage.
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentRecordingStopTimeout)
		defer stopCancel()
		recResult = p.stopAndStoreRecording(stopCtx, agentConn,
			recordingID, parent.ID, parent.OwnerID, parent.WorkspaceID)
	}
	resp := withSubagentType(map[string]any{
		"chat_id": targetChat.ID.String(),
		"title":   targetChat.Title,
		"report":  report,
		"status":  string(targetChat.Status),
	}, targetChat)
	if recResult.recordingFileID != "" {
		resp["recording_file_id"] = recResult.recordingFileID
	}
	if recResult.thumbnailFileID != "" {
		resp["thumbnail_file_id"] = recResult.thumbnailFileID
	}
	return toolJSONResponse(resp)
}

func (p *Server) interruptSubagent(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) (database.Chat, bool, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	if !isDescendant {
		return database.Chat{}, false, ErrSubagentNotDescendant
	}

	targetChat, err := p.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("get target chat: %w", err)
	}

	if targetChat.Status == database.ChatStatusWaiting {
		return targetChat, false, nil
	}

	updatedChat, err := p.InterruptChat(ctx, targetChat)
	if err != nil {
		// Idle / archived chats no longer satisfy the
		// chatstate.Interrupt precondition. Surface the error
		// so the caller can decide whether the parent expected
		// the subagent to already be waiting.
		return database.Chat{}, false, xerrors.Errorf("interrupt subagent chat: %w", err)
	}
	// chatstate.Interrupt lands active runs in `interrupting`
	// and requires-action chats in `running`. Workers finalize
	// the transition; accept either non-active status as long as
	// the transition committed.
	return updatedChat, true, nil
}

func (p *Server) checkSubagentCompletion(
	ctx context.Context,
	chatID uuid.UUID,
) (database.Chat, string, bool, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, "", false, xerrors.Errorf("get chat: %w", err)
	}

	// interrupting is transient: the worker transitions it to
	// waiting (no queued messages) or running (queued messages).
	// Treat it as not-done so the agent settles before
	// classification, avoiding stale partial output.
	if chat.Status == database.ChatStatusRunning ||
		chat.Status == database.ChatStatusInterrupting {
		return chat, "", false, nil
	}

	report, err := latestSubagentAssistantMessage(ctx, p.db, chatID)
	if err != nil {
		return database.Chat{}, "", false, err
	}

	return chat, report, true, nil
}

func latestSubagentAssistantMessage(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) (string, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	})
	if err != nil {
		return "", xerrors.Errorf("get chat messages: %w", err)
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != database.ChatMessageRoleAssistant ||
			message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		content, parseErr := chatprompt.ParseContent(message)
		if parseErr != nil {
			continue
		}
		text := strings.TrimSpace(contentBlocksToText(content))
		if text == "" {
			continue
		}
		return text, nil
	}

	return "", nil
}

// isSubagentDescendant reports whether targetChatID is a descendant
// of ancestorChatID by walking up the parent chain from the target.
// This is O(depth) DB queries instead of O(nodes) BFS.
func isSubagentDescendant(
	ctx context.Context,
	store database.Store,
	ancestorChatID uuid.UUID,
	targetChatID uuid.UUID,
) (bool, error) {
	if ancestorChatID == targetChatID {
		return false, nil
	}

	currentID := targetChatID
	visited := map[uuid.UUID]struct{}{} // cycle protection
	for {
		if _, seen := visited[currentID]; seen {
			return false, nil
		}
		visited[currentID] = struct{}{}

		chat, err := store.GetChatByID(ctx, currentID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return false, nil // chain broken; not a confirmed descendant
			}
			return false, xerrors.Errorf("get chat %s: %w", currentID, err)
		}
		if !chat.ParentChatID.Valid {
			return false, nil // reached root without finding ancestor
		}
		if chat.ParentChatID.UUID == ancestorChatID {
			return true, nil
		}
		currentID = chat.ParentChatID.UUID
	}
}

func subagentFallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "..."
	}

	return subagentTruncateRunes(title, maxRunes)
}

func subagentTruncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}

	return string(runes[:maxRunes])
}

func toolJSONResponse(result map[string]any) fantasy.ToolResponse {
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
