package chatd

import (
	"context"
	"encoding/json"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// chatToolSetsNotice is inserted into the system prompt when the
// workspace group is off for the turn.
const chatToolSetsNotice = "Workspace tools are off for this chat. You cannot read or write files, run commands, or create, start or stop workspaces. Earlier messages can mention these tools. If the user asks for this work, tell them to turn on Workspace tools in the + menu."

// builtinToolGroups maps every built-in AgentTool name that turn
// preparation can add to the tool group that owns it.
var builtinToolGroups = map[string]codersdk.ChatToolGroup{
	// Workspace.
	"read_file":        codersdk.ChatToolGroupWorkspace,
	"write_file":       codersdk.ChatToolGroupWorkspace,
	"edit_files":       codersdk.ChatToolGroupWorkspace,
	"attach_file":      codersdk.ChatToolGroupWorkspace,
	"execute":          codersdk.ChatToolGroupWorkspace,
	"process_output":   codersdk.ChatToolGroupWorkspace,
	"process_list":     codersdk.ChatToolGroupWorkspace,
	"process_signal":   codersdk.ChatToolGroupWorkspace,
	"list_templates":   codersdk.ChatToolGroupWorkspace,
	"read_template":    codersdk.ChatToolGroupWorkspace,
	"create_workspace": codersdk.ChatToolGroupWorkspace,
	"start_workspace":  codersdk.ChatToolGroupWorkspace,
	"stop_workspace":   codersdk.ChatToolGroupWorkspace,
	// Sub-agents.
	"spawn_agent":          codersdk.ChatToolGroupSubagents,
	"list_subagent_models": codersdk.ChatToolGroupSubagents,
	"wait_agent":           codersdk.ChatToolGroupSubagents,
	"message_agent":        codersdk.ChatToolGroupSubagents,
	"interrupt_agent":      codersdk.ChatToolGroupSubagents,
	"list_agents":          codersdk.ChatToolGroupSubagents,
	// Skills.
	"read_skill":      codersdk.ChatToolGroupSkills,
	"read_skill_file": codersdk.ChatToolGroupSkills,
	// Memory.
	"read_memory":        codersdk.ChatToolGroupMemory,
	"save_memory":        codersdk.ChatToolGroupMemory,
	"delete_memory":      codersdk.ChatToolGroupMemory,
	"consolidate_memory": codersdk.ChatToolGroupMemory,
	// Planning.
	"propose_plan":      codersdk.ChatToolGroupPlanning,
	"ask_user_question": codersdk.ChatToolGroupPlanning,
	// Advisor.
	"advisor": codersdk.ChatToolGroupAdvisor,
	// Tool search.
	"find_tools": codersdk.ChatToolGroupToolSearch,
}

// providerToolGroups maps provider-executed tool names to their group.
// Provider tools are not AgentTools.
var providerToolGroups = map[string]codersdk.ChatToolGroup{
	"web_search": codersdk.ChatToolGroupWebSearch,
	"computer":   codersdk.ChatToolGroupWorkspace,
}

// resolveToolGroups returns the on or off state of every catalog group
// for one turn. stored holds the chat's stored choices. experimentOn
// reports whether a group's experiment is on for the chat owner.
func resolveToolGroups(
	stored map[codersdk.ChatToolGroup]bool,
	experimentOn func(codersdk.Experiment) bool,
) map[codersdk.ChatToolGroup]bool {
	decided := map[codersdk.Experiment]bool{}
	isOn := func(ex codersdk.Experiment) bool {
		if v, ok := decided[ex]; ok {
			return v
		}
		v := experimentOn(ex)
		decided[ex] = v
		return v
	}
	groups := make(map[codersdk.ChatToolGroup]bool, len(codersdk.ChatToolGroupCatalog))
	for _, info := range codersdk.ChatToolGroupCatalog {
		on := info.DefaultOn
		if info.Switchable {
			if v, ok := stored[info.ID]; ok {
				on = v
			}
		}
		// Without the experiment a stored off stays off, and nothing
		// turns on. Skip the read when the value is the default anyway.
		if info.Experiment != "" && on != info.DefaultOn && !isOn(info.Experiment) {
			on = on && info.DefaultOn
		}
		groups[info.ID] = on
	}
	for _, info := range codersdk.ChatToolGroupCatalog {
		for _, need := range info.Needs {
			groups[info.ID] = groups[info.ID] && groups[need]
		}
	}
	return groups
}

// toolGroupOn reports whether group is on in groups. A nil map means
// every group has its default.
func toolGroupOn(groups map[codersdk.ChatToolGroup]bool, group codersdk.ChatToolGroup) bool {
	if groups == nil {
		info, ok := codersdk.ChatToolGroupInfoByID(group)
		return ok && info.DefaultOn
	}
	return groups[group]
}

// storedFormToolGroups returns the switchable groups whose resolved
// state differs from the default, which is the form the column stores.
func storedFormToolGroups(resolved map[codersdk.ChatToolGroup]bool) map[codersdk.ChatToolGroup]bool {
	out := map[codersdk.ChatToolGroup]bool{}
	for _, info := range codersdk.ChatToolGroupCatalog {
		if !info.Switchable {
			continue
		}
		if on := toolGroupOn(resolved, info.ID); on != info.DefaultOn {
			out[info.ID] = on
		}
	}
	return out
}

// parseStoredToolGroups decodes the chats.tool_groups column. Invalid
// JSON yields no choices.
func parseStoredToolGroups(raw json.RawMessage) map[codersdk.ChatToolGroup]bool {
	stored := map[codersdk.ChatToolGroup]bool{}
	if len(raw) == 0 {
		return stored
	}
	if err := json.Unmarshal(raw, &stored); err != nil || stored == nil {
		return map[codersdk.ChatToolGroup]bool{}
	}
	return stored
}

// reservedToolNames returns the built-in tool names whose group is off,
// so a dynamic tool cannot take them.
func reservedToolNames(groups map[codersdk.ChatToolGroup]bool) map[string]bool {
	reserved := map[string]bool{}
	for name, group := range builtinToolGroups {
		if !toolGroupOn(groups, group) {
			reserved[name] = true
		}
	}
	return reserved
}

// turnToolGroups returns the tool groups for the turn. The first step of
// a turn decides and later steps of the same turn reuse the decision.
func (server *Server) turnToolGroups(
	ctx context.Context,
	decisions *turnExperimentDecisions,
	chat database.Chat,
	messages []database.ChatMessage,
) map[codersdk.ChatToolGroup]bool {
	evaluate := func() map[codersdk.ChatToolGroup]bool {
		return resolveToolGroups(parseStoredToolGroups(chat.ToolGroups), func(ex codersdk.Experiment) bool {
			return server.experimentEvaluator.Enabled(ctx, chat.OwnerID, ex)
		})
	}
	if decisions == nil {
		return evaluate()
	}
	return decisions.toolGroupsForTurn(stopNudgeKey(messages), evaluate)
}
