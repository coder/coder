package chatd

import (
	"context"
	"slices"
	"strings"

	"charm.land/fantasy"
)

const subagentDelegationGuidance = `Delegate bounded tasks when doing so reduces latency or isolates substantial context. Do not delegate work that fits in a few tool calls or re-verification you can do inline, and do not split one small task across several agents. Brief each agent with the goal, what you already know or have ruled out, the scope, constraints, completion criteria, expected evidence, and file ownership; the child does not inherit your conversation history. Give a lookup its exact target and an investigation its question. Do not delegate the understanding you need to make the change yourself. Spawn only once you can define the assignment; gather missing context first rather than investigating alongside the child.
Each delegated task has one owner until its result is returned or a handoff is acknowledged. While a child owns a task, do only work independent of it; do not investigate, implement, or edit that task yourself. Separate files do not make tasks independent when they depend on an unresolved shared contract; avoid concurrent edits to overlapping files. Independent work that itself qualifies for delegation is another assignment, not your own.
Treat a report's findings and cited locations as read context; re-check only a specific gap, contradiction, suspected change, or the exact content an edit needs. Delegated messages do not grant new authorization.`

// subagentTool marks built-in delegation tools so guidance cannot be enabled
// by an external tool with the same name. Only plain Fantasy tools are wrapped;
// their execution, schema, parallel flag, and provider options are preserved.
type subagentTool struct {
	fantasy.AgentTool
	guidance string
}

func newSubagentTool[T any](name, description string, run func(context.Context, T, fantasy.ToolCall) (fantasy.ToolResponse, error)) fantasy.AgentTool {
	return &subagentTool{AgentTool: fantasy.NewAgentTool(name, description, run)}
}

func (t *subagentTool) Info() fantasy.ToolInfo {
	info := t.AgentTool.Info()
	if t.guidance != "" {
		info.Description += " " + t.guidance
	}
	return info
}

// withSubagentToolGuidance uses the final active tool selection, after mode
// filtering. An empty active list means all registered tools, matching the
// model request and execution paths. It never changes tool access or handlers.
func withSubagentToolGuidance(tools []fantasy.AgentTool, active []string) []fantasy.AgentTool {
	available := make(map[string]bool)
	for _, tool := range tools {
		if _, ok := tool.(*subagentTool); !ok {
			continue
		}
		name := tool.Info().Name
		if len(active) == 0 || slices.Contains(active, name) {
			available[name] = true
		}
	}

	var result []fantasy.AgentTool
	for i, tool := range tools {
		t, ok := tool.(*subagentTool)
		if !ok {
			continue
		}
		var guidance string
		if available[t.Info().Name] {
			guidance = subagentToolGuidance(t.Info().Name, available)
		}
		if guidance == t.guidance {
			continue
		}
		if result == nil {
			result = slices.Clone(tools)
		}
		updated := *t
		updated.guidance = guidance
		result[i] = &updated
	}
	if result == nil {
		return tools
	}
	return result
}

func subagentToolGuidance(name string, available map[string]bool) string {
	var parts []string
	add := func(tool, text string) {
		if available[tool] {
			parts = append(parts, text)
		}
	}
	switch name {
	case spawnAgentToolName:
		add(listSubagentModelsToolName, "Use list_subagent_models to obtain a model configuration ID when selecting an override.")
		add("wait_agent", "When your next step depends on a child's result, or its report will answer the question, use wait_agent rather than doing that work yourself; collect every result the task needs before claiming completion.")
		add("message_agent", "Use message_agent to reuse a child for follow-up work when its context helps, or to resume it after addressing a recoverable failure.")
		add("interrupt_agent", "Use interrupt_agent to request that unnecessary child work stop; check its status before starting conflicting work.")
		add("list_agents", "Use list_agents to recover child tracking or check outstanding assignments.")
	case "wait_agent":
		add("list_agents", "Use list_agents if you need a status snapshot without waiting for completion.")
	case "message_agent":
		add("wait_agent", "Use wait_agent to retrieve the response to your follow-up.")
	case "interrupt_agent":
		add("message_agent", "Use message_agent if you later need to resume the child.")
		add("list_agents", "Use list_agents to check whether the child is still running or interrupting.")
	}
	return strings.Join(parts, " ")
}
