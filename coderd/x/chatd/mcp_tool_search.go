package chatd

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// mcpToolSearchBudgetDivisor scales the activation and catalog budgets
// to the model context window: activated schemas may re-inline up to
// ContextLimit / 10 estimated tokens per generation.
const mcpToolSearchBudgetDivisor = 10

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
	wsStart := len(candidates)
	for _, tool := range input.workspaceMCPTools {
		if !toolAllowedForTurn(tool, input.planMode, input.parentChatID, input.approvedMCPConfigIDs) {
			continue
		}
		candidates = append(candidates, deferredMCPTool{tool: tool, server: workspaceMCPServerName(tool)})
	}
	trimWorkspaceServerNames(candidates, wsStart)
	return candidates
}

// trimWorkspaceServerNames trims surrounding whitespace from workspace
// server names, which config validation permits but find_tools strips
// from queries, so a padded server stays reachable by scope. Trimming
// is skipped when it would collapse distinct servers into one catalog
// identity (a padded and an unpadded sibling, or a collision with an
// external slug): those keep their raw names, so each server keeps its
// own catalog group and an exact-form scope matches only its own
// server.
func trimWorkspaceServerNames(candidates []deferredMCPTool, wsStart int) {
	sources := make(map[string]map[string]struct{}, len(candidates))
	for i, candidate := range candidates {
		key := candidate.server
		if i >= wsStart {
			key = strings.TrimSpace(key)
		}
		if sources[key] == nil {
			sources[key] = make(map[string]struct{}, 1)
		}
		sources[key][candidate.server] = struct{}{}
	}
	for i := wsStart; i < len(candidates); i++ {
		trimmed := strings.TrimSpace(candidates[i].server)
		if len(sources[trimmed]) == 1 {
			candidates[i].server = trimmed
		}
	}
}

// workspaceMCPServerName prefers the wrapper's unsanitized routing name
// because sanitization can truncate the model-facing name before the
// "__" separator, which would otherwise catalog each such tool under a
// fake single-tool server that prefix scoping cannot reach.
func workspaceMCPServerName(tool fantasy.AgentTool) string {
	if namer, ok := tool.(interface{ ServerName() string }); ok {
		return namer.ServerName()
	}
	if server, _, ok := strings.Cut(tool.Info().Name, "__"); ok {
		return server
	}
	return ""
}

type mcpToolSearchInput struct {
	experimentEnabled bool
	candidates        []deferredMCPTool
	dynamicToolNames  map[string]bool
}

// decideMCPToolSearch reports whether MCP tool schemas are deferred
// behind find_tools. With the experiment enabled, every generation with
// deferrable candidates defers.
func decideMCPToolSearch(input mcpToolSearchInput) bool {
	if !input.experimentEnabled || len(input.candidates) == 0 {
		return false
	}
	// A client-executed dynamic tool named find_tools would otherwise be
	// advertised alongside the built-in and capture its calls as
	// requires_action, so a collision on either surface fails open.
	if input.dynamicToolNames[chattool.FindToolsName] {
		return false
	}
	for _, candidate := range input.candidates {
		if candidate.tool.Info().Name == chattool.FindToolsName {
			return false
		}
	}
	return true
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
	// Workspace config validation permits an empty server key, and
	// candidates whose config lookup failed also carry no server, so
	// empty identities get a real label here. The label is the entry's
	// Server for grouping, scope matching, and scoring alike, and it is
	// collision-safe: a literal server with the same name keeps its own
	// group and scope.
	fallback := "workspace"
	taken := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.server != "" {
			taken[candidate.server] = struct{}{}
		}
	}
	for suffix := 2; ; suffix++ {
		if _, collides := taken[fallback]; !collides {
			break
		}
		fallback = "workspace-" + strconv.Itoa(suffix)
	}
	entries := make([]chattool.FindToolCatalogEntry, 0, len(candidates))
	for _, candidate := range candidates {
		info := candidate.tool.Info()
		server := candidate.server
		if server == "" {
			server = fallback
		}
		entries = append(entries, chattool.FindToolCatalogEntry{
			Name:              info.Name,
			Description:       info.Description,
			Server:            server,
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
// Direct calls whose tool result is an error are admitted last, newest
// first. History cannot distinguish a pre-execution denial (hook
// policy, input validation) from an executed call whose MCP server
// returned an error, so errored calls activate only with budget left
// after every other activation: the schema stays available for a
// corrected retry without displacing schemas that find_tools results
// or successful calls already claimed. Search-time reservations mirror
// this order because sibling calls settle before a step's searches run
// and the step result observer refunds errored reservations first, so
// searches see the same leftover budget.
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
	for i := range rows {
		parts, err := chatprompt.ParseContent(rows[i])
		if err != nil {
			continue
		}
		parsedParts[i] = parts
	}
	// Providers may reuse a tool-call ID in a later step, so a result
	// settles the newest unpaired call with its ID: a new call abandons
	// any older unpaired call, whose own result was lost or compacted
	// away, and abandoned calls count as successful rather than
	// adopting a later call's result. Results paired to a call
	// occurrence are recorded so orphan results, whose call row was
	// compacted away, are admitted at their own row even when a later
	// step reuses their ID.
	type partRef struct{ row, part int }
	callErrored := make(map[partRef]bool)
	resultPaired := make(map[partRef]struct{})
	pendingByID := make(map[string]partRef)
	for i := range rows {
		for j, part := range parsedParts[i] {
			if part.ToolCallID == "" {
				continue
			}
			switch part.Type {
			case codersdk.ChatMessagePartTypeToolCall:
				pendingByID[part.ToolCallID] = partRef{row: i, part: j}
			case codersdk.ChatMessagePartTypeToolResult:
				if ref, ok := pendingByID[part.ToolCallID]; ok {
					callErrored[ref] = part.IsError
					resultPaired[partRef{row: i, part: j}] = struct{}{}
					delete(pendingByID, part.ToolCallID)
				}
			}
		}
	}
	pendingSearch := make(map[string][]string)
	var erroredNames []string
	for i := len(rows) - 1; i >= 0; i-- {
		for j, part := range parsedParts[i] {
			if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName != chattool.FindToolsName {
				if callErrored[partRef{row: i, part: j}] {
					erroredNames = append(erroredNames, part.ToolName)
					continue
				}
				appendName(part.ToolName)
			}
		}
		for j, part := range parsedParts[i] {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == chattool.FindToolsName:
				var result chattool.FindToolsResult
				if err := json.Unmarshal(part.Result, &result); err != nil {
					continue
				}
				if _, paired := resultPaired[partRef{row: i, part: j}]; paired {
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
	for _, name := range erroredNames {
		appendName(name)
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
