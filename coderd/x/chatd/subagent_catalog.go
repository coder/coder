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
	spawnAgentToolName = "spawn_agent"

	subagentTypeGeneral     = "general"
	subagentTypeExplore     = "explore"
	subagentTypeComputerUse = "computer_use"

	defaultSystemPromptPlanningGuidance = "1. Use " + spawnAgentToolName +
		" and wait_agent when delegation helps gather context. Prefer type=\"" +
		subagentTypeGeneral +
		"\" for substantial delegated research, analysis, reasoning, review, " +
		"planning support, or implementation. Use type=\"" + subagentTypeGeneral +
		"\" even for read-only work when the task is open-ended, multi-step, " +
		"parallel, requires synthesis, or may later need edits. When planning, " +
		"type=\"" + subagentTypeGeneral +
		"\" remains non-mutating until implementation is approved. Use type=\"" +
		subagentTypeExplore +
		"\" only for narrow repository-local read-only code discovery or code " +
		"tracing, such as locating files, callsites, or a bounded existing flow. " +
		"Do not use type=\"" + subagentTypeExplore +
		"\" for generic research, broad architecture analysis, planning synthesis, " +
		"external or web research, parallel research, or tasks that may need edits."
)

type spawnAgentArgs struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt"`
	Title  string `json:"title,omitempty"`
}

type subagentDefinition struct {
	id                string
	description       string
	unavailableReason func(context.Context, *Server, database.Chat) string
	buildOptions      func(context.Context, *Server, database.Chat, database.Chat, uuid.UUID, string) (childSubagentChatOptions, error)
}

func allSubagentDefinitions() []subagentDefinition {
	return []subagentDefinition{
		{
			id:          subagentTypeGeneral,
			description: "substantial delegated research, analysis, reasoning, review, planning support, and implementation",
			buildOptions: func(ctx context.Context, p *Server, parent database.Chat, _ database.Chat, _ uuid.UUID, _ string) (childSubagentChatOptions, error) {
				modelConfigID, reasoningEffort, err := p.resolveSubagentModelConfigID(
					ctx,
					parent.OwnerID,
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
			buildOptions: func(ctx context.Context, p *Server, _ database.Chat, turnParent database.Chat, currentModelConfigID uuid.UUID, _ string) (childSubagentChatOptions, error) {
				modelConfigID, reasoningEffort, err := p.resolveSubagentModelConfigID(
					ctx,
					turnParent.OwnerID,
					codersdk.ChatModelOverrideContextExplore,
				)
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				if modelConfigID == uuid.Nil {
					modelConfigID = currentModelConfigID
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
			buildOptions: func(ctx context.Context, p *Server, currentChat database.Chat, _ database.Chat, _ uuid.UUID, prompt string) (childSubagentChatOptions, error) {
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

func subagentDefinitionsByID(ids ...string) []subagentDefinition {
	defs := make([]subagentDefinition, 0, len(ids))
	for _, id := range ids {
		if def, ok := lookupSubagentDefinition(id); ok {
			defs = append(defs, def)
		}
	}
	return defs
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
		formatSubagentDefinitions(availableDefs) + ". Do not use this for " +
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
		"may need edits. Be careful when running parallel subagents: if two " +
		"subagents modify the same files they will conflict with each other, " +
		"so ensure parallel subagent tasks are independent. The child agent " +
		"receives the same workspace tools but cannot spawn its own subagents. " +
		"After spawning, use wait_agent to retrieve the result. Agents persist " +
		"after completion; reuse an agent via message_agent for follow-up work " +
		"when it already has relevant context. Spawned agents are your " +
		"responsibility: do not abandon one in a working state (running); " +
		"retrieve its result, redirect it with message_agent, or stop " +
		"it with interrupt_agent."
	if currentChat.PlanMode.Valid && currentChat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
		description += " During plan mode, type=\"" + subagentTypeGeneral +
			"\" is for non-mutating substantial investigation and planning support, " +
			"and type=\"" + subagentTypeExplore +
			"\" is for narrow repository-local lookup or tracing. Both may use " +
			"shell commands for exploration and inspection, but only type=\"" +
			subagentTypeGeneral +
			"\" should be used for cloning repositories or non-local investigation. " +
			"They must not implement changes or intentionally modify workspace files."
	}
	return description
}

func formatSubagentDefinitions(defs []subagentDefinition) string {
	return formatSubagentDefinitionsWithDescriptionOverrides(defs, nil)
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

func planningOverlaySubagentGuidance() string {
	planModeDescriptions := map[string]string{
		subagentTypeGeneral: "non-mutating substantial investigation, analysis, and planning support",
		subagentTypeExplore: "narrow repository-local codebase lookup and code tracing",
	}

	return "Use read_file, execute, process_output, list_templates, read_template, " +
		spawnAgentToolName + ", and approved external MCP tools when available to gather context. " +
		"Workspace MCP tools are not available in root plan mode, and side-effecting built-in tools such as process_list, process_signal, message_agent, interrupt_agent, and computer-use actions remain unavailable. In Plan Mode, " +
		spawnAgentToolName + " delegation is for investigation and planning " +
		"support, not code writing or implementation. Use type=\"" + subagentTypeGeneral +
		"\" for substantial investigation, reasoning, and planning support. " +
		"Use type=\"" + subagentTypeExplore +
		"\" only for narrow repository-local lookup or tracing. Allowed type " +
		"values in Plan Mode: " +
		formatSubagentDefinitionsWithDescriptionOverrides(
			subagentDefinitionsByID(
				subagentTypeGeneral,
				subagentTypeExplore,
			),
			planModeDescriptions,
		) + "."
}
