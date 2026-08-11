package toolsdk

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"
)

const (
	PromptNameAgentsDelegate = "coder_agents_delegate"
	PromptNameAgentsCheck    = "coder_agents_check"
)

// PromptArgument describes one argument accepted by a Prompt.
type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

// Prompt defines an MCP prompt shared by the HTTP and CLI servers.
// See https://modelcontextprotocol.io/specification/2026-07-28/server/prompts.
type Prompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument

	Render func(args map[string]string) (string, error)
}

// AllPrompts is the canonical list of MCP prompts exposed by Coder MCP
// servers.
var AllPrompts = []Prompt{AgentsDelegate, AgentsCheck}

var AgentsDelegate = Prompt{
	Name:        PromptNameAgentsDelegate,
	Description: "Delegate a coding task to a Coder Agents chat and monitor it to completion.",
	Arguments: []PromptArgument{
		{
			Name:        "task",
			Description: "The task the Coder Agent should perform, including all context it needs.",
			Required:    true,
		},
		{
			Name:        "model_config_id",
			Description: "Optional model config UUID for the chat. When omitted, a model is picked from " + ToolNameListChatModelConfigs + ".",
		},
	},
	Render: func(args map[string]string) (string, error) {
		task, err := requiredPromptArg(args, "task")
		if err != nil {
			return "", err
		}
		var createStep string
		if modelConfigID := strings.TrimSpace(args["model_config_id"]); modelConfigID != "" {
			createStep = fmt.Sprintf("1. Call %s with the task above as the prompt and model_config_id %q.", ToolNameCreateChat, modelConfigID)
		} else {
			createStep = fmt.Sprintf("1. Call %s with the task above as the prompt. To pick a specific model, call %s first and pass its ID as model_config_id.", ToolNameCreateChat, ToolNameListChatModelConfigs)
		}
		return fmt.Sprintf(`Delegate the following task to a Coder Agent and see it through to completion.

<task>
%s
</task>

Follow these steps:
%s
2. Share the returned chat URL with the user right away so they can follow along.
3. Poll %s until the chat stops running, waiting between polls.
4. Read the transcript with %s; page older history with before_id while has_more is true.
5. If the agent needs input or the result needs iteration, reply with %s and keep monitoring.
6. Report the outcome to the user, including the chat URL and a summary of what the agent did.
`, task, createStep, ToolNameGetChat, ToolNameGetChatMessages, ToolNameSendChatMessage), nil
	},
}

var AgentsCheck = Prompt{
	Name:        PromptNameAgentsCheck,
	Description: "Check the status and recent activity of an existing Coder Agents chat.",
	Arguments: []PromptArgument{
		{
			Name:        "chat_id",
			Description: "UUID of the Coder Agents chat to check.",
			Required:    true,
		},
	},
	Render: func(args map[string]string) (string, error) {
		chatID, err := requiredPromptArg(args, "chat_id")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`Check on the Coder Agents chat %q and report back.

Follow these steps:
1. Call %s with the chat_id to get its status, last turn summary, and any last error.
2. Call %s with the chat_id for recent transcript context, including queued_messages.
3. Summarize for the user: what the agent is doing or has done, whether it is blocked or waiting for input, and any errors. Include the chat URL.
`, chatID, ToolNameGetChat, ToolNameGetChatMessages), nil
	},
}

func requiredPromptArg(args map[string]string, name string) (string, error) {
	value := strings.TrimSpace(args[name])
	if value == "" {
		return "", xerrors.Errorf("missing required prompt argument: %s", name)
	}
	return value, nil
}
