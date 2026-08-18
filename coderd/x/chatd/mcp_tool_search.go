package chatd

import (
	"encoding/json"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

const mcpToolSearchThresholdDivisor = 10

type deferredMCPTool struct {
	tool              fantasy.AgentTool
	server            string
	serverDescription string
}

type deferredMCPCandidateInput struct {
	mcpTools              []fantasy.AgentTool
	workspaceMCPTools     []fantasy.AgentTool
	mcpConfigByID         map[uuid.UUID]database.MCPServerConfig
	planMode              database.NullChatPlanMode
	parentChatID          uuid.NullUUID
	approvedMCPConfigIDs  map[uuid.UUID]struct{}
	includeWorkspaceTools bool
}

// collectDeferredMCPCandidates applies the same turn policy that
// filterToolsForTurn later applies to the executable tool set, so the
// find_tools catalog never advertises tools the turn cannot run.
func collectDeferredMCPCandidates(input deferredMCPCandidateInput) []deferredMCPTool {
	candidates := make([]deferredMCPTool, 0, len(input.mcpTools)+len(input.workspaceMCPTools))
	for _, tool := range input.mcpTools {
		if !toolAllowedForTurn(tool, input.planMode, input.parentChatID, input.approvedMCPConfigIDs) {
			continue
		}
		candidate := deferredMCPTool{tool: tool}
		if identified, ok := tool.(mcpclient.MCPToolIdentifier); ok {
			if config, exists := input.mcpConfigByID[identified.MCPServerConfigID()]; exists {
				candidate.server = config.Slug
				candidate.serverDescription = config.Description
			}
		}
		candidates = append(candidates, candidate)
	}
	if !input.includeWorkspaceTools {
		return candidates
	}
	for _, tool := range input.workspaceMCPTools {
		if !toolAllowedForTurn(tool, input.planMode, input.parentChatID, input.approvedMCPConfigIDs) {
			continue
		}
		candidates = append(candidates, deferredMCPTool{tool: tool, server: workspaceMCPServerName(tool)})
	}
	return candidates
}

// workspaceMCPServerName prefers the wrapper's unsanitized routing name
// because sanitization can truncate the model-facing name before the
// "__" separator, which would otherwise catalog each such tool under a
// fake single-tool server that prefix scoping cannot reach.
// Workspace config validation allows surrounding whitespace in server
// names, which find_tools trims from queries, so the catalog name is
// trimmed too; routing keeps the raw name.
func workspaceMCPServerName(tool fantasy.AgentTool) string {
	if namer, ok := tool.(interface{ ServerName() string }); ok {
		return strings.TrimSpace(namer.ServerName())
	}
	if server, _, ok := strings.Cut(tool.Info().Name, "__"); ok {
		return server
	}
	return ""
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
	dynamicToolNames  map[string]bool
}

func decideMCPToolSearch(input mcpToolSearchInput) mcpToolSearchDecision {
	decision := mcpToolSearchDecision{estimatedTokens: estimateDeferredMCPToolTokens(input.candidates)}
	if !input.experimentEnabled || len(input.candidates) == 0 {
		return decision
	}
	// A client-executed dynamic tool named find_tools would otherwise be
	// advertised alongside the built-in and capture its calls as
	// requires_action, so a collision on either surface fails open.
	if input.dynamicToolNames[chattool.FindToolsName] {
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
			SchemaTokens:      estimateDeferredMCPToolTokens([]deferredMCPTool{candidate}),
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

// deriveDeferredMCPActivations walks the surviving history newest first
// so that when the aggregate schema weight of activations exceeds
// tokenBudget, the least recently activated schemas are shed. The newest
// activation is always kept even when its schema alone exceeds the
// budget, so the tool the model just requested stays usable. Shed tools
// stay in the catalog and remain directly callable, which reactivates
// them as most recent. A tokenBudget <= 0 means unbounded.
//
// find_tools results are admitted at their tool-call row, after that
// row's direct tool calls, so a step's own search activations cannot
// shed the schema of a tool the model invoked directly. Results whose
// call row was compacted away are admitted at the result row.
//
// Direct calls whose tool result is an error do not activate: the call
// was denied before execution (hook policy, input validation) or
// failed, so inlining its schema would spend budget that surviving
// find_tools results have already promised elsewhere. Execution-time
// reservation may still charge such calls, which only under-claims.
func deriveDeferredMCPActivations(rows []database.ChatMessage, candidates []deferredMCPTool, tokenBudget float64) []string {
	candidateByName := make(map[string]deferredMCPTool, len(candidates))
	for _, candidate := range candidates {
		candidateByName[candidate.tool.Info().Name] = candidate
	}
	seen := make(map[string]struct{}, len(candidates))
	activated := make([]string, 0, len(candidates))
	usedTokens := 0.0
	appendName := func(name string) {
		candidate, ok := candidateByName[name]
		if !ok {
			return
		}
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		weight := estimateDeferredMCPToolTokens([]deferredMCPTool{candidate})
		if len(activated) > 0 && tokenBudget > 0 && usedTokens+weight > tokenBudget {
			return
		}
		usedTokens += weight
		activated = append(activated, name)
	}
	parsedParts := make([][]codersdk.ChatMessagePart, len(rows))
	findToolsCallIDs := make(map[string]struct{})
	erroredCallIDs := make(map[string]struct{})
	for i := range rows {
		parts, err := chatprompt.ParseContent(rows[i])
		if err != nil {
			continue
		}
		parsedParts[i] = parts
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == chattool.FindToolsName && part.ToolCallID != "" {
				findToolsCallIDs[part.ToolCallID] = struct{}{}
			}
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.IsError && part.ToolCallID != "" {
				erroredCallIDs[part.ToolCallID] = struct{}{}
			}
		}
	}
	pendingSearch := make(map[string][]string)
	for i := len(rows) - 1; i >= 0; i-- {
		for _, part := range parsedParts[i] {
			if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName != chattool.FindToolsName {
				if _, errored := erroredCallIDs[part.ToolCallID]; errored {
					continue
				}
				appendName(part.ToolName)
			}
		}
		for _, part := range parsedParts[i] {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == chattool.FindToolsName:
				var result chattool.FindToolsResult
				if err := json.Unmarshal(part.Result, &result); err != nil {
					continue
				}
				if _, paired := findToolsCallIDs[part.ToolCallID]; paired {
					pendingSearch[part.ToolCallID] = result.Activated
					continue
				}
				for _, name := range result.Activated {
					appendName(name)
				}
			case part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == chattool.FindToolsName:
				for _, name := range pendingSearch[part.ToolCallID] {
					appendName(name)
				}
				delete(pendingSearch, part.ToolCallID)
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
