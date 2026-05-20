package workspacediscovery

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// WorkspaceContextOptions configures workspace context discovery.
type WorkspaceContextOptions struct {
	Chat                     database.Chat
	GetWorkspaceAgent        func(context.Context) (database.WorkspaceAgent, error)
	GetWorkspaceConn         func(context.Context) (workspacesdk.AgentConn, error)
	InstructionLookupTimeout time.Duration
	Logger                   slog.Logger
	SanitizePromptText       func(string) string
}

// WorkspaceContextResult is raw workspace context fetched from the agent.
type WorkspaceContextResult struct {
	Agent           *database.WorkspaceAgent
	Parts           []codersdk.ChatMessagePart
	WorkspaceConnOK bool
}

// FetchWorkspaceContext retrieves context-file and skill parts from the
// workspace agent, then stamps server-owned context metadata on the parts.
func FetchWorkspaceContext(
	ctx context.Context,
	opts WorkspaceContextOptions,
) WorkspaceContextResult {
	if !opts.Chat.WorkspaceID.Valid || opts.GetWorkspaceAgent == nil {
		return WorkspaceContextResult{}
	}

	loadedAgent, agentErr := opts.GetWorkspaceAgent(ctx)
	if agentErr != nil {
		return WorkspaceContextResult{}
	}

	directory := loadedAgent.ExpandedDirectory
	if directory == "" {
		directory = loadedAgent.Directory
	}

	result := WorkspaceContextResult{Agent: &loadedAgent}
	if opts.GetWorkspaceConn != nil {
		instructionCtx, cancel := context.WithTimeout(ctx, opts.InstructionLookupTimeout)
		defer cancel()

		conn, connErr := opts.GetWorkspaceConn(instructionCtx)
		if connErr != nil {
			opts.Logger.Debug(ctx, "failed to resolve workspace connection for instruction files",
				slog.F("chat_id", opts.Chat.ID),
				slog.Error(connErr),
			)
		} else {
			result.WorkspaceConnOK = true

			agentCfg, cfgErr := conn.ContextConfig(instructionCtx)
			if cfgErr != nil {
				opts.Logger.Debug(ctx, "failed to fetch context config from agent",
					slog.F("chat_id", opts.Chat.ID), slog.Error(cfgErr))
				// Treat a transient ContextConfig failure the same as a
				// failed connection so callers can retry on the next turn.
				result.WorkspaceConnOK = false
			} else {
				result.Parts = agentCfg.Parts
			}
		}
	}

	sanitize := opts.SanitizePromptText
	if sanitize == nil {
		sanitize = func(s string) string { return s }
	}
	agentID := uuid.NullUUID{UUID: loadedAgent.ID, Valid: true}
	for i := range result.Parts {
		result.Parts[i].ContextFileAgentID = agentID
		if result.Parts[i].Type == codersdk.ChatMessagePartTypeContextFile {
			result.Parts[i].ContextFileContent = sanitize(result.Parts[i].ContextFileContent)
			result.Parts[i].ContextFileOS = loadedAgent.OperatingSystem
			result.Parts[i].ContextFileDirectory = directory
		}
	}

	return result
}

// MCPToolsCacheEntry stores workspace MCP tools discovered from an agent.
type MCPToolsCacheEntry struct {
	AgentID uuid.UUID
	Tools   []workspacesdk.MCPToolInfo
}

// LoadCachedMCPTools checks a chat-scoped MCP cache for an agent match.
func LoadCachedMCPTools(
	cache *sync.Map,
	chatID uuid.UUID,
	agentID uuid.UUID,
) ([]workspacesdk.MCPToolInfo, bool) {
	if cache == nil {
		return nil, false
	}
	cached, ok := cache.Load(chatID)
	if !ok {
		return nil, false
	}
	entry, ok := cached.(*MCPToolsCacheEntry)
	if !ok || entry.AgentID != agentID {
		return nil, false
	}
	return entry.Tools, true
}

// StoreMCPTools stores non-empty workspace MCP metadata for a chat and agent.
func StoreMCPTools(
	cache *sync.Map,
	chatID uuid.UUID,
	agentID uuid.UUID,
	tools []workspacesdk.MCPToolInfo,
) {
	if cache == nil || len(tools) == 0 {
		return
	}
	cache.Store(chatID, &MCPToolsCacheEntry{
		AgentID: agentID,
		Tools:   tools,
	})
}

// MCPToolsOptions configures raw workspace MCP tool discovery.
type MCPToolsOptions struct {
	ChatID                  uuid.UUID
	Cache                   *sync.Map
	GetWorkspaceAgent       func(context.Context) (database.WorkspaceAgent, error)
	ResolveWorkspaceAgentID func(context.Context) (uuid.UUID, error)
	IsNoWorkspaceAgentError func(error) bool
	GetWorkspaceConn        func(context.Context) (workspacesdk.AgentConn, error)
	DiscoveryTimeout        time.Duration
	Logger                  slog.Logger
}

// MCPToolsResult is the raw workspace MCP metadata discovery result.
type MCPToolsResult struct {
	Tools []workspacesdk.MCPToolInfo
	OK    bool
}

// DiscoverMCPTools lists raw workspace MCP tool metadata and caches
// successful non-empty results by chat and agent ID.
func DiscoverMCPTools(
	ctx context.Context,
	opts MCPToolsOptions,
) MCPToolsResult {
	if opts.GetWorkspaceAgent != nil {
		if agent, agentErr := opts.GetWorkspaceAgent(ctx); agentErr == nil {
			if tools, ok := LoadCachedMCPTools(opts.Cache, opts.ChatID, agent.ID); ok {
				return MCPToolsResult{Tools: tools, OK: true}
			}
		}
	}

	if opts.ResolveWorkspaceAgentID == nil || opts.GetWorkspaceConn == nil {
		return MCPToolsResult{}
	}
	_, agentErr := opts.ResolveWorkspaceAgentID(ctx)
	if agentErr != nil {
		if opts.IsNoWorkspaceAgentError != nil && opts.IsNoWorkspaceAgentError(agentErr) {
			if opts.Cache != nil {
				opts.Cache.Delete(opts.ChatID)
			}
			return MCPToolsResult{}
		}
		opts.Logger.Warn(ctx, "failed to resolve workspace agent for MCP tools",
			slog.Error(agentErr))
		return MCPToolsResult{}
	}

	conn, connErr := opts.GetWorkspaceConn(ctx)
	if connErr != nil {
		opts.Logger.Warn(ctx, "failed to get workspace conn for MCP tools",
			slog.Error(connErr))
		return MCPToolsResult{}
	}

	listCtx := ctx
	cancel := func() {}
	if opts.DiscoveryTimeout > 0 {
		listCtx, cancel = context.WithTimeout(ctx, opts.DiscoveryTimeout)
	}
	defer cancel()

	toolsResp, listErr := conn.ListMCPTools(listCtx)
	if listErr != nil {
		opts.Logger.Warn(ctx, "failed to list workspace MCP tools",
			slog.Error(listErr))
		return MCPToolsResult{}
	}

	if len(toolsResp.Tools) > 0 && opts.GetWorkspaceAgent != nil {
		if agent, agentErr := opts.GetWorkspaceAgent(ctx); agentErr == nil {
			StoreMCPTools(opts.Cache, opts.ChatID, agent.ID, toolsResp.Tools)
		}
	}
	return MCPToolsResult{Tools: toolsResp.Tools, OK: true}
}
