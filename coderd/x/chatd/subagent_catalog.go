package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const (
	spawnAgentToolName         = "spawn_agent"
	listSubagentModelsToolName = "list_subagent_models"

	subagentTypeGeneral     = "general"
	subagentTypeExplore     = "explore"
	subagentTypeComputerUse = "computer_use"
)

// unbilledSubagentToolNames excludes parent-side orchestration because
// child chats bill their own runtime. Include deprecated aliases.
var unbilledSubagentToolNames = map[string]bool{
	spawnAgentToolName:         true,
	"wait_agent":               true,
	"message_agent":            true,
	"interrupt_agent":          true,
	"close_agent":              true,
	"list_agents":              true,
	listSubagentModelsToolName: true,
}

type spawnAgentArgs struct {
	Type            string `json:"type"`
	Prompt          string `json:"prompt"`
	Title           string `json:"title,omitempty"`
	ModelConfigID   string `json:"model_config_id,omitempty" description:"Optional model configuration UUID obtained from an available model-discovery tool. Runs the child on that model instead of the configured default. Not supported for type 'computer_use'."`
	ReasoningEffort string `json:"reasoning_effort,omitempty" description:"Optional reasoning effort for the child: none, minimal, low, medium, high, xhigh, or max. Clamped to the selected model's supported range. Not supported for type 'computer_use'."`
}

type subagentDefinition struct {
	id                string
	description       string
	unavailableReason func(context.Context, *Server, database.Chat) string
	buildOptions      func(context.Context, *Server, database.Chat, database.Chat, uuid.UUID, *uuid.UUID, string) (childSubagentChatOptions, error)
}

func allSubagentDefinitions() []subagentDefinition {
	return []subagentDefinition{
		{
			id:          subagentTypeGeneral,
			description: "substantial delegated research, analysis, reasoning, review, planning support, and implementation",
			buildOptions: func(ctx context.Context, p *Server, parent database.Chat, _ database.Chat, _ uuid.UUID, explicitModelConfigID *uuid.UUID, _ string) (childSubagentChatOptions, error) {
				if explicitModelConfigID != nil {
					return childSubagentChatOptions{modelConfigIDOverride: explicitModelConfigID}, nil
				}
				modelConfigID, reasoningEffort, err := p.resolveSubagentModelConfigID(
					ctx,
					parent.OwnerID,
					parent.OrganizationID,
					codersdk.ChatModelOverrideContextGeneral,
				)
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				options := childSubagentChatOptions{}
				if modelConfigID != uuid.Nil {
					options.modelConfigIDOverride = &modelConfigID
					options.reasoningEffortOverride = reasoningEffort
				}
				return options, nil
			},
		},
		{
			id:          subagentTypeExplore,
			description: "narrow repository-local read-only code discovery and code tracing",
			buildOptions: func(ctx context.Context, p *Server, _ database.Chat, turnParent database.Chat, currentModelConfigID uuid.UUID, explicitModelConfigID *uuid.UUID, _ string) (childSubagentChatOptions, error) {
				modelConfigID := currentModelConfigID
				var reasoningEffort *string
				if explicitModelConfigID != nil {
					modelConfigID = *explicitModelConfigID
				} else {
					resolvedModelConfigID, resolvedReasoningEffort, err := p.resolveSubagentModelConfigID(
						ctx,
						turnParent.OwnerID,
						turnParent.OrganizationID,
						codersdk.ChatModelOverrideContextExplore,
					)
					if err != nil {
						return childSubagentChatOptions{}, err
					}
					if resolvedModelConfigID != uuid.Nil {
						modelConfigID = resolvedModelConfigID
					}
					reasoningEffort = resolvedReasoningEffort
				}
				inheritedMCPServerIDs, err := p.resolveExploreToolSnapshot(
					ctx,
					turnParent,
				)
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				// Clearing plan mode changes only the Explore model behavior.
				// The inherited tool snapshot still comes from the parent turn.
				clearPlanMode := database.NullChatPlanMode{}
				return childSubagentChatOptions{
					chatMode: database.NullChatMode{
						ChatMode: database.ChatModeExplore,
						Valid:    true,
					},
					modelConfigIDOverride:   &modelConfigID,
					reasoningEffortOverride: reasoningEffort,
					planModeOverride:        &clearPlanMode,
					inheritedMCPServerIDs:   inheritedMCPServerIDs,
				}, nil
			},
		},
		{
			id:          subagentTypeComputerUse,
			description: "desktop GUI interaction, screenshots, and browser or app automation",
			unavailableReason: func(ctx context.Context, p *Server, currentChat database.Chat) string {
				if currentChat.PlanMode.Valid && currentChat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
					return `type "computer_use" is unavailable in plan mode`
				}
				if !p.experiments.Enabled(codersdk.ExperimentChatVirtualDesktop) {
					return `type "computer_use" is unavailable because the chat-virtual-desktop experiment is not enabled`
				}
				_, _, _, err := p.computerUseProviderAndModelFromConfig(ctx)
				if err != nil {
					p.logger.Warn(ctx, "computer-use provider config is unavailable",
						slog.F("chat_id", currentChat.ID),
						slog.Error(err),
					)
					return `type "computer_use" is unavailable because its provider configuration could not be loaded`
				}
				return ""
			},
			buildOptions: func(ctx context.Context, p *Server, currentChat database.Chat, _ database.Chat, _ uuid.UUID, _ *uuid.UUID, prompt string) (childSubagentChatOptions, error) {
				provider, _, _, err := p.computerUseProviderAndModelFromConfig(ctx)
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				providerKeys, err := p.resolveUserProviderAPIKeysForProviderType(ctx, currentChat.OwnerID, string(provider))
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				if !userCanUseProviderKeys(providerKeys, string(provider)) {
					return childSubagentChatOptions{}, xerrors.Errorf(
						`API key for computer-use provider %q is not configured`,
						provider,
					)
				}
				return childSubagentChatOptions{
					chatMode: database.NullChatMode{
						ChatMode: database.ChatModeComputerUse,
						Valid:    true,
					},
					systemPrompt: computerUseSubagentSystemPrompt + "\n\n" + strings.TrimSpace(prompt),
				}, nil
			},
		},
	}
}

func lookupSubagentDefinition(id string) (subagentDefinition, bool) {
	for _, def := range allSubagentDefinitions() {
		if def.id == id {
			return def, true
		}
	}
	return subagentDefinition{}, false
}

func availableSubagentDefinitions(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
) []subagentDefinition {
	defs := allSubagentDefinitions()
	available := make([]subagentDefinition, 0, len(defs))
	for _, def := range defs {
		if def.unavailableReasonText(ctx, p, currentChat) == "" {
			available = append(available, def)
		}
	}
	return available
}

func availableSubagentTypeIDs(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
) []string {
	defs := availableSubagentDefinitions(ctx, p, currentChat)
	ids := make([]string, 0, len(defs))
	for _, def := range defs {
		ids = append(ids, def.id)
	}
	return ids
}

func (d subagentDefinition) unavailableReasonText(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
) string {
	if d.unavailableReason == nil {
		return ""
	}
	return d.unavailableReason(ctx, p, currentChat)
}

func resolveSubagentDefinition(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
	rawSubagentType string,
) (subagentDefinition, error) {
	subagentType := strings.TrimSpace(rawSubagentType)
	def, ok := lookupSubagentDefinition(subagentType)
	if !ok {
		return subagentDefinition{}, xerrors.Errorf(
			"type must be one of: %s",
			strings.Join(availableSubagentTypeIDs(ctx, p, currentChat), ", "),
		)
	}
	if reason := def.unavailableReasonText(ctx, p, currentChat); reason != "" {
		return subagentDefinition{}, xerrors.New(reason)
	}
	return def, nil
}

func validateSubagentSpawnParent(currentChat database.Chat) error {
	if currentChat.ParentChatID.Valid {
		return xerrors.New("delegated chats cannot create child subagents")
	}
	if isExploreSubagentMode(currentChat.Mode) {
		return xerrors.New("explore chats cannot create child subagents")
	}
	return nil
}

func subagentTypeFromChat(chat database.Chat) string {
	if !chat.Mode.Valid {
		return subagentTypeGeneral
	}
	switch chat.Mode.ChatMode {
	case database.ChatModeExplore:
		return subagentTypeExplore
	case database.ChatModeComputerUse:
		return subagentTypeComputerUse
	default:
		return subagentTypeGeneral
	}
}

func withSubagentType(result map[string]any, chat database.Chat) map[string]any {
	if result == nil {
		result = map[string]any{}
	}
	result["type"] = subagentTypeFromChat(chat)
	return result
}

func subagentErrorResponse(err error, chat *database.Chat) fantasy.ToolResponse {
	if chat == nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return toolJSONErrorResponse(withSubagentType(map[string]any{
		"error": err.Error(),
	}, *chat))
}

func buildSpawnAgentDescription(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
) string {
	availableDefs := availableSubagentDefinitions(ctx, p, currentChat)
	description := "Spawn a delegated child subagent to work on a clearly scoped, " +
		"independent task in parallel. Use the type field to choose " +
		"the right specialist. Available type values: " +
		formatSubagentDefinitionsWithDescriptionOverrides(availableDefs, nil) + ". Do not use this for " +
		"simple or quick operations you can handle directly with execute, " +
		"read_file, or write_file. Prefer type=\"" + subagentTypeGeneral +
		"\" for substantial delegated research, analysis, reasoning, review, " +
		"planning support, or implementation, even when the child should only " +
		"report findings. When using type=\"" + subagentTypeGeneral +
		"\" for read-only work, explicitly instruct the child not to modify " +
		"files and to return findings. Use type=\"" + subagentTypeExplore +
		"\" only for narrow repository-local read-only code discovery or code " +
		"tracing, such as locating files, callsites, or a bounded existing flow. " +
		"Do not use type=\"" + subagentTypeExplore +
		"\" for generic research, broad architecture analysis, planning " +
		"synthesis, external or web research, parallel research, or tasks that " +
		"may need edits. " + subagentDelegationGuidance +
		" The child does not inherit your conversation history. Its tools " +
		"depend on its mode and current access; do not assume they match yours. " +
		"You may optionally set model_config_id to an available model " +
		"configuration UUID and reasoning_effort to select the child's effort; " +
		"both apply only to type \"" + subagentTypeGeneral + "\" and type \"" +
		subagentTypeExplore + "\". Do not invent model identifiers. " +
		"Track assignments you start and collect the results the task needs. " +
		"Account for unfinished work using the lifecycle tools available to " +
		"you; report a missing capability rather than claim work stopped or " +
		"finished. An error status can be recoverable; address its cause " +
		"before retrying."
	if currentChat.PlanMode.Valid && currentChat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
		description += " During plan mode, type=\"" + subagentTypeGeneral +
			"\" is for non-mutating substantial investigation and planning support, " +
			"and type=\"" + subagentTypeExplore +
			"\" is for narrow repository-local lookup or tracing. Both may use " +
			"shell commands for exploration and inspection, but only type=\"" +
			subagentTypeGeneral +
			"\" should be used for cloning repositories or non-local investigation. " +
			"They must not implement changes or edit existing project files; " +
			"cloning by type=\"general\" for inspection is the only intentional workspace-write exception."
	}
	return description
}

func formatSubagentDefinitionsWithDescriptionOverrides(
	defs []subagentDefinition,
	descriptionOverrides map[string]string,
) string {
	parts := make([]string, 0, len(defs))
	for _, def := range defs {
		description := def.description
		if override, ok := descriptionOverrides[def.id]; ok {
			description = override
		}
		parts = append(parts, def.id+" ("+description+")")
	}
	return strings.Join(parts, ", ")
}
