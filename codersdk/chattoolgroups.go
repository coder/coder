package codersdk

// ChatToolGroup identifies a group of built-in chat tools that the chat
// owner can turn on or off per chat.
type ChatToolGroup string

const (
	ChatToolGroupWorkspace  ChatToolGroup = "workspace"
	ChatToolGroupSubagents  ChatToolGroup = "subagents"
	ChatToolGroupSkills     ChatToolGroup = "skills"
	ChatToolGroupMemory     ChatToolGroup = "memory"
	ChatToolGroupPlanning   ChatToolGroup = "planning"
	ChatToolGroupAdvisor    ChatToolGroup = "advisor"
	ChatToolGroupWebSearch  ChatToolGroup = "web_search"
	ChatToolGroupToolSearch ChatToolGroup = "tool_search"
)

// ChatToolGroupInfo describes one tool group in the catalog.
type ChatToolGroupInfo struct {
	ID          ChatToolGroup `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	// DefaultOn is the state of the group when the chat stores no choice.
	DefaultOn bool `json:"default_on"`
	// Switchable reports whether the chat owner can change the group.
	Switchable bool `json:"switchable"`
	// Needs lists groups that must be on for this group to be on.
	Needs []ChatToolGroup `json:"needs,omitempty"`
	// Experiment, when set, must be enabled for the group to be switchable.
	Experiment Experiment `json:"experiment,omitempty"`
}

// ChatToolGroupCatalog lists every tool group in display order.
var ChatToolGroupCatalog = []ChatToolGroupInfo{
	{
		ID:          ChatToolGroupWorkspace,
		Label:       "Workspace tools",
		Description: "Files, commands and workspaces",
		DefaultOn:   true,
		Switchable:  true,
		Experiment:  ExperimentChatToolSets,
	},
	{
		ID:          ChatToolGroupSubagents,
		Label:       "Sub-agents",
		Description: "Start and manage helper agents",
		DefaultOn:   true,
		Switchable:  true,
		Experiment:  ExperimentChatToolSets,
	},
	{
		ID:          ChatToolGroupSkills,
		Label:       "Skills",
		Description: "Read skills",
		DefaultOn:   true,
		Switchable:  true,
		Experiment:  ExperimentChatToolSets,
	},
	{
		ID:          ChatToolGroupMemory,
		Label:       "Memory",
		Description: "Save and read project memory",
		DefaultOn:   true,
		Switchable:  true,
		Experiment:  ExperimentChatToolSets,
	},
	{
		ID:          ChatToolGroupPlanning,
		Label:       "Planning",
		Description: "Write and propose plans",
		DefaultOn:   true,
		Switchable:  false,
		Needs:       []ChatToolGroup{ChatToolGroupWorkspace},
	},
	{
		ID:          ChatToolGroupAdvisor,
		Label:       "Advisor",
		Description: "Ask a stronger model for advice",
		DefaultOn:   true,
		Switchable:  false,
	},
	{
		ID:          ChatToolGroupWebSearch,
		Label:       "Web search",
		Description: "Search the web",
		DefaultOn:   true,
		Switchable:  false,
	},
	{
		ID:          ChatToolGroupToolSearch,
		Label:       "Tool search",
		Description: "Find deferred tools",
		DefaultOn:   true,
		Switchable:  false,
	},
}

// ChatToolGroupInfoByID returns the catalog entry for id.
func ChatToolGroupInfoByID(id ChatToolGroup) (ChatToolGroupInfo, bool) {
	for _, info := range ChatToolGroupCatalog {
		if info.ID == id {
			return info, true
		}
	}
	return ChatToolGroupInfo{}, false
}
