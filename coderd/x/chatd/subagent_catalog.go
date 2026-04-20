package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	spawnSubagentToolName = "spawn_subagent"

	subagentTypeGeneral     = "general"
	subagentTypeExplore     = "explore"
	subagentTypeComputerUse = "computer_use"
)

type spawnSubagentArgs struct {
	SubagentType string `json:"subagent_type"`
	Prompt       string `json:"prompt"`
	Title        string `json:"title,omitempty"`
}

type subagentDefinition struct {
	id                string
	description       string
	unavailableReason func(context.Context, *Server, database.Chat) string
	buildOptions      func(context.Context, *Server, database.Chat, uuid.UUID, string) (childSubagentChatOptions, error)
}

func allSubagentDefinitions() []subagentDefinition {
	return []subagentDefinition{
		{
			id:          subagentTypeGeneral,
			description: "delegated work that may inspect or modify workspace files",
			buildOptions: func(_ context.Context, _ *Server, _ database.Chat, _ uuid.UUID, _ string) (childSubagentChatOptions, error) {
				return childSubagentChatOptions{}, nil
			},
		},
		{
			id:          subagentTypeExplore,
			description: "read-only discovery, code tracing, and system understanding",
			buildOptions: func(ctx context.Context, p *Server, parent database.Chat, currentModelConfigID uuid.UUID, _ string) (childSubagentChatOptions, error) {
				modelConfigID, err := p.resolveExploreSubagentModelConfigID(
					ctx,
					parent.OwnerID,
					currentModelConfigID,
				)
				if err != nil {
					return childSubagentChatOptions{}, err
				}
				clearPlanMode := database.NullChatPlanMode{}
				return childSubagentChatOptions{
					chatMode: database.NullChatMode{
						ChatMode: database.ChatModeExplore,
						Valid:    true,
					},
					modelConfigIDOverride: &modelConfigID,
					planModeOverride:      &clearPlanMode,
				}, nil
			},
		},
		{
			id:          subagentTypeComputerUse,
			description: "desktop GUI interaction, screenshots, and browser or app automation",
			unavailableReason: func(ctx context.Context, p *Server, currentChat database.Chat) string {
				if currentChat.PlanMode.Valid && currentChat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
					return `subagent_type "computer_use" is unavailable in plan mode`
				}
				if !p.isAnthropicConfigured(ctx) || !p.isDesktopEnabled(ctx) {
					return `subagent_type "computer_use" is unavailable because computer use is not configured`
				}
				return ""
			},
			buildOptions: func(_ context.Context, _ *Server, _ database.Chat, _ uuid.UUID, prompt string) (childSubagentChatOptions, error) {
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
			"subagent_type must be one of: %s",
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
	result["subagent_type"] = subagentTypeFromChat(chat)
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

func buildSpawnSubagentDescription(
	ctx context.Context,
	p *Server,
	currentChat database.Chat,
) string {
	availableDefs := availableSubagentDefinitions(ctx, p, currentChat)
	description := "Spawn a delegated child subagent to work on a clearly scoped, " +
		"independent task in parallel. Use the subagent_type field to choose " +
		"the right specialist. Available subagent_type values: " +
		formatSubagentDefinitions(availableDefs) + ". Do not use this for " +
		"simple or quick operations you can handle directly with execute, " +
		"read_file, or write_file. Reserve writable subagents for tasks that " +
		"require intellectual work such as code analysis, writing new code, or " +
		"complex refactoring. Be careful when running parallel subagents: if " +
		"two subagents modify the same files they will conflict with each " +
		"other, so ensure parallel subagent tasks are independent. The child " +
		"agent receives the same workspace tools but cannot spawn its own " +
		"subagents. After spawning, use wait_agent to collect the result."
	if currentChat.PlanMode.Valid && currentChat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
		description += " During plan mode, general and explore subagents may use shell commands for exploration, such as cloning repositories, searching code, and running inspection commands, but they must not implement changes or intentionally modify workspace files."
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

func defaultSystemPromptPlanningGuidance() string {
	return "1. Use " + spawnSubagentToolName + " with subagent_type=\"" +
		subagentTypeExplore + "\" and wait_agent to research the codebase and " +
		"gather context as needed. Reserve subagent_type=\"" +
		subagentTypeGeneral + "\" for writable delegated work."
}

func planningOverlaySubagentGuidance() string {
	planModeDescriptions := map[string]string{
		subagentTypeGeneral: "delegated investigation, planning support, and non-mutating exploration",
	}

	return "Use read_file, execute, process_output, list_templates, read_template, and " +
		spawnSubagentToolName + " to gather context. In Plan Mode, " +
		spawnSubagentToolName + " delegation is for investigation and planning " +
		"support, not code writing or implementation. Allowed subagent_type " +
		"values in Plan Mode: " +
		formatSubagentDefinitionsWithDescriptionOverrides(
			subagentDefinitionsByID(
				subagentTypeGeneral,
				subagentTypeExplore,
			),
			planModeDescriptions,
		) + "."
}
