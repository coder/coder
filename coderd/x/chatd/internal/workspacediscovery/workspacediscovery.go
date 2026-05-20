// Package workspacediscovery contains raw workspace-agent discovery helpers
// for chatd. Chatd owns persistence, prompt assembly, and tool exposure policy.
package workspacediscovery

import (
	"cmp"
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
		opts.Logger.Debug(ctx, "failed to get workspace agent for context",
			slog.F("chat_id", opts.Chat.ID),
			slog.Error(agentErr),
		)
		return WorkspaceContextResult{}
	}

	directory := cmp.Or(loadedAgent.ExpandedDirectory, loadedAgent.Directory)

	result := WorkspaceContextResult{Agent: &loadedAgent}
	if opts.GetWorkspaceConn != nil {
		instructionCtx := ctx
		cancel := func() {}
		if opts.InstructionLookupTimeout > 0 {
			instructionCtx, cancel = context.WithTimeout(ctx, opts.InstructionLookupTimeout)
		}
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

// MCPToolsCacheKey identifies workspace MCP metadata that is safe to share
// across chats for the same user, workspace, and agent.
type MCPToolsCacheKey struct {
	OwnerID     uuid.UUID
	WorkspaceID uuid.UUID
	AgentID     uuid.UUID
}

// Valid reports whether key contains every component needed for cache reuse.
func (k MCPToolsCacheKey) Valid() bool {
	return k.OwnerID != uuid.Nil && k.WorkspaceID != uuid.Nil && k.AgentID != uuid.Nil
}

// MCPToolsCacheEntry stores workspace MCP tools discovered from an agent.
type MCPToolsCacheEntry struct {
	Key   MCPToolsCacheKey
	Tools []workspacesdk.MCPToolInfo
}

// MCPToolsCacheKeyFunc returns the cache key for a resolved workspace agent.
type MCPToolsCacheKeyFunc func(agentID uuid.UUID) (MCPToolsCacheKey, bool)

// LoadCachedMCPTools checks the shared MCP cache for an exact key match.
func LoadCachedMCPTools(
	cache *sync.Map,
	key MCPToolsCacheKey,
) ([]workspacesdk.MCPToolInfo, bool) {
	if cache == nil || !key.Valid() {
		return nil, false
	}
	cached, ok := cache.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := cached.(*MCPToolsCacheEntry)
	if !ok {
		return nil, false
	}
	return entry.Tools, true
}

// DeleteMCPToolsForWorkspace removes cached metadata for a user workspace.
func DeleteMCPToolsForWorkspace(
	cache *sync.Map,
	ownerID uuid.UUID,
	workspaceID uuid.UUID,
) {
	if cache == nil || ownerID == uuid.Nil || workspaceID == uuid.Nil {
		return
	}
	cache.Range(func(rawKey, _ any) bool {
		key, ok := rawKey.(MCPToolsCacheKey)
		if ok && key.OwnerID == ownerID && key.WorkspaceID == workspaceID {
			cache.Delete(key)
		}
		return true
	})
}

// DeleteStaleMCPToolsForWorkspace removes metadata from older agents.
func DeleteStaleMCPToolsForWorkspace(
	cache *sync.Map,
	currentKey MCPToolsCacheKey,
) {
	if cache == nil || !currentKey.Valid() {
		return
	}
	cache.Range(func(rawKey, _ any) bool {
		key, ok := rawKey.(MCPToolsCacheKey)
		if ok && key.OwnerID == currentKey.OwnerID &&
			key.WorkspaceID == currentKey.WorkspaceID &&
			key.AgentID != currentKey.AgentID {
			cache.Delete(key)
		}
		return true
	})
}

// StoreMCPTools stores non-empty workspace MCP metadata for an exact cache key.
func StoreMCPTools(
	cache *sync.Map,
	key MCPToolsCacheKey,
	tools []workspacesdk.MCPToolInfo,
) {
	if cache == nil || !key.Valid() || len(tools) == 0 {
		return
	}
	DeleteStaleMCPToolsForWorkspace(cache, key)
	cache.Store(key, &MCPToolsCacheEntry{
		Key:   key,
		Tools: tools,
	})
}

// MCPToolsOptions configures raw workspace MCP tool discovery.
type MCPToolsOptions struct {
	Cache                   *sync.Map
	CacheKey                MCPToolsCacheKeyFunc
	ResolveWorkspaceAgentID func(context.Context) (uuid.UUID, error)
	IsNoWorkspaceAgentError func(error) bool
	GetWorkspaceConn        func(context.Context) (workspacesdk.AgentConn, error)
	DiscoveryTimeout        time.Duration
	Logger                  slog.Logger
}

// MCPToolsResult is the raw workspace MCP metadata discovery result.
type MCPToolsResult struct {
	Tools []workspacesdk.MCPToolInfo
	Key   MCPToolsCacheKey
	OK    bool
}

// DiscoverMCPTools lists raw workspace MCP tool metadata and caches
// successful non-empty results by owner, workspace, and agent.
func DiscoverMCPTools(
	ctx context.Context,
	opts MCPToolsOptions,
) MCPToolsResult {
	if opts.ResolveWorkspaceAgentID == nil {
		return MCPToolsResult{}
	}
	agentID, agentErr := opts.ResolveWorkspaceAgentID(ctx)
	if agentErr != nil {
		if opts.IsNoWorkspaceAgentError != nil && opts.IsNoWorkspaceAgentError(agentErr) {
			return MCPToolsResult{}
		}
		opts.Logger.Warn(ctx, "failed to resolve workspace agent for MCP tools",
			slog.Error(agentErr))
		return MCPToolsResult{}
	}
	key, _ := mcpCacheKey(opts.CacheKey, agentID)
	DeleteStaleMCPToolsForWorkspace(opts.Cache, key)

	if tools, cached := LoadCachedMCPTools(opts.Cache, key); cached {
		return MCPToolsResult{Tools: tools, Key: key, OK: true}
	}
	if opts.GetWorkspaceConn == nil {
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

	StoreMCPTools(opts.Cache, key, toolsResp.Tools)
	return MCPToolsResult{Tools: toolsResp.Tools, Key: key, OK: true}
}

func mcpCacheKey(keyFunc MCPToolsCacheKeyFunc, agentID uuid.UUID) (MCPToolsCacheKey, bool) {
	if keyFunc == nil {
		return MCPToolsCacheKey{}, false
	}
	key, ok := keyFunc(agentID)
	if !ok || !key.Valid() {
		return MCPToolsCacheKey{}, false
	}
	return key, true
}
