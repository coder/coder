package chatd

import (
	"encoding/json"
	"slices"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const mcpToolSearchThresholdDivisor = 10

type deferredMCPTool struct {
	tool              fantasy.AgentTool
	server            string
	serverDescription string
}

type mcpToolSearchDecision struct {
	apply           bool
	estimatedTokens float64
}

type mcpToolSearchInput struct {
	experimentEnabled bool
	forceDefer        bool
	contextWindow     int64
	candidates        []deferredMCPTool
}

func decideMCPToolSearch(input mcpToolSearchInput) mcpToolSearchDecision {
	decision := mcpToolSearchDecision{estimatedTokens: estimateDeferredMCPToolTokens(input.candidates)}
	if !input.experimentEnabled || len(input.candidates) == 0 {
		return decision
	}
	for _, candidate := range input.candidates {
		if candidate.tool.Info().Name == chattool.FindToolsName {
			return decision
		}
	}
	decision.apply = input.forceDefer ||
		(input.contextWindow > 0 && decision.estimatedTokens > float64(input.contextWindow)/mcpToolSearchThresholdDivisor)
	return decision
}

func configureDeferredMCPToolSearch(
	tools []fantasy.AgentTool,
	activeToolNames []string,
	candidates []deferredMCPTool,
	findTools fantasy.AgentTool,
	activations []string,
) ([]fantasy.AgentTool, []string, map[string]bool) {
	candidateNames := deferredMCPToolNameSet(candidates)
	ordered := make([]fantasy.AgentTool, 0, len(tools)+1)
	for _, tool := range tools {
		if !candidateNames[tool.Info().Name] {
			ordered = append(ordered, tool)
		}
	}
	ordered = append(ordered, findTools)
	for _, tool := range tools {
		if candidateNames[tool.Info().Name] {
			ordered = append(ordered, tool)
		}
	}

	activeToolNames = slices.DeleteFunc(activeToolNames, func(name string) bool { return candidateNames[name] })
	activeToolNames = append(activeToolNames, chattool.FindToolsName)
	activeToolNames = append(activeToolNames, activations...)
	return ordered, activeToolNames, candidateNames
}

func estimateDeferredMCPToolTokens(candidates []deferredMCPTool) float64 {
	chars := 0
	for _, candidate := range candidates {
		info := candidate.tool.Info()
		schema := map[string]any{"type": "object", "properties": info.Parameters}
		if len(info.Required) > 0 {
			schema["required"] = info.Required
		}
		serialized, _ := json.Marshal(schema)
		chars += len(info.Name) + len(info.Description) + len(serialized)
	}
	return float64(chars) / 2.5
}

func deferredMCPToolEntries(candidates []deferredMCPTool) []chattool.FindToolCatalogEntry {
	entries := make([]chattool.FindToolCatalogEntry, 0, len(candidates))
	for _, candidate := range candidates {
		info := candidate.tool.Info()
		entries = append(entries, chattool.FindToolCatalogEntry{
			Name:              info.Name,
			Description:       info.Description,
			Server:            candidate.server,
			ServerDescription: candidate.serverDescription,
			ParameterText:     flattenMCPParameterText(info.Parameters),
		})
	}
	return entries
}

func flattenMCPParameterText(value any) string {
	var values []string
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				values = append(values, key)
				walk(typed[key])
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case string:
			values = append(values, typed)
		}
	}
	walk(value)
	return strings.Join(values, " ")
}

func deriveDeferredMCPActivations(rows []database.ChatMessage, candidates []deferredMCPTool) []string {
	current := deferredMCPToolNameSet(candidates)
	seen := make(map[string]struct{}, len(candidates))
	activated := make([]string, 0, len(candidates))
	appendName := func(name string) {
		if !current[name] {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		activated = append(activated, name)
	}
	for _, row := range rows {
		parts, err := chatprompt.ParseContent(row)
		if err != nil {
			continue
		}
		for _, part := range parts {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == chattool.FindToolsName:
				var result chattool.FindToolsResult
				if err := json.Unmarshal(part.Result, &result); err != nil {
					continue
				}
				for _, name := range result.Activated {
					appendName(name)
				}
			case part.Type == codersdk.ChatMessagePartTypeToolCall:
				appendName(part.ToolName)
			}
		}
	}
	return activated
}

func deferredMCPToolNameSet(candidates []deferredMCPTool) map[string]bool {
	names := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		names[candidate.tool.Info().Name] = true
	}
	return names
}
