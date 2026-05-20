package workspacediscovery_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/internal/workspacediscovery"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestFetchWorkspaceContextZeroTimeoutUsesParentContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	agent := database.WorkspaceAgent{
		ID:              agentID,
		Directory:       "/workspace",
		OperatingSystem: "linux",
	}

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().ContextConfig(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (workspacesdk.ContextConfigResponse, error) {
			require.NoError(t, ctx.Err())
			return workspacesdk.ContextConfigResponse{
				Parts: []codersdk.ChatMessagePart{
					{
						Type:               codersdk.ChatMessagePartTypeContextFile,
						ContextFilePath:    "/workspace/AGENTS.md",
						ContextFileContent: "raw instructions",
					},
					{
						Type:             codersdk.ChatMessagePartTypeSkill,
						SkillName:        "repo-helper",
						SkillDescription: "helps with the repo",
					},
				},
			}, nil
		},
	)

	result := workspacediscovery.FetchWorkspaceContext(ctx, workspacediscovery.WorkspaceContextOptions{
		Chat: chat,
		GetWorkspaceAgent: func(context.Context) (database.WorkspaceAgent, error) {
			return agent, nil
		},
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			return conn, nil
		},
		InstructionLookupTimeout: 0,
		Logger:                   slog.Make(),
		SanitizePromptText: func(s string) string {
			return "clean: " + s
		},
	})

	require.NotNil(t, result.Agent)
	require.True(t, result.WorkspaceConnOK)
	require.Len(t, result.Parts, 2)
	require.Equal(t, uuid.NullUUID{UUID: agentID, Valid: true}, result.Parts[0].ContextFileAgentID)
	require.Equal(t, "clean: raw instructions", result.Parts[0].ContextFileContent)
	require.Equal(t, "linux", result.Parts[0].ContextFileOS)
	require.Equal(t, "/workspace", result.Parts[0].ContextFileDirectory)
	require.Equal(t, uuid.NullUUID{UUID: agentID, Valid: true}, result.Parts[1].ContextFileAgentID)
}

func TestMCPToolsCacheLifecycle(t *testing.T) {
	t.Parallel()

	var cache sync.Map
	ownerID := uuid.New()
	workspaceID := uuid.New()
	oldKey := workspacediscovery.MCPToolsCacheKey{OwnerID: ownerID, WorkspaceID: workspaceID, AgentID: uuid.New()}
	newKey := workspacediscovery.MCPToolsCacheKey{OwnerID: ownerID, WorkspaceID: workspaceID, AgentID: uuid.New()}
	otherKey := workspacediscovery.MCPToolsCacheKey{OwnerID: ownerID, WorkspaceID: uuid.New(), AgentID: uuid.New()}
	tool := workspacesdk.MCPToolInfo{ServerName: "workspace", Name: "workspace__tool"}

	workspacediscovery.StoreMCPTools(&cache, oldKey, []workspacesdk.MCPToolInfo{tool})
	workspacediscovery.StoreMCPTools(&cache, otherKey, []workspacesdk.MCPToolInfo{tool})
	workspacediscovery.StoreMCPTools(&cache, newKey, []workspacesdk.MCPToolInfo{tool})

	_, ok := workspacediscovery.LoadCachedMCPTools(&cache, oldKey)
	require.False(t, ok)
	_, ok = workspacediscovery.LoadCachedMCPTools(&cache, newKey)
	require.True(t, ok)
	_, ok = workspacediscovery.LoadCachedMCPTools(&cache, otherKey)
	require.True(t, ok)

	workspacediscovery.DeleteMCPToolsForWorkspace(&cache, ownerID, workspaceID)
	_, ok = workspacediscovery.LoadCachedMCPTools(&cache, newKey)
	require.False(t, ok)
	_, ok = workspacediscovery.LoadCachedMCPTools(&cache, otherKey)
	require.True(t, ok)
}

func TestStoreMCPToolsSkipsInvalidAndEmpty(t *testing.T) {
	t.Parallel()

	var cache sync.Map
	validKey := workspacediscovery.MCPToolsCacheKey{OwnerID: uuid.New(), WorkspaceID: uuid.New(), AgentID: uuid.New()}
	workspacediscovery.StoreMCPTools(&cache, validKey, nil)
	_, ok := workspacediscovery.LoadCachedMCPTools(&cache, validKey)
	require.False(t, ok)

	workspacediscovery.StoreMCPTools(&cache, workspacediscovery.MCPToolsCacheKey{}, []workspacesdk.MCPToolInfo{{Name: "ignored"}})
	_, ok = workspacediscovery.LoadCachedMCPTools(&cache, workspacediscovery.MCPToolsCacheKey{})
	require.False(t, ok)
}

func TestDiscoverMCPToolsCacheHitAvoidsWorkspaceConn(t *testing.T) {
	t.Parallel()

	var cache sync.Map
	agentID := uuid.New()
	key := workspacediscovery.MCPToolsCacheKey{OwnerID: uuid.New(), WorkspaceID: uuid.New(), AgentID: agentID}
	workspacediscovery.StoreMCPTools(&cache, key, []workspacesdk.MCPToolInfo{{ServerName: "workspace", Name: "workspace__cached"}})

	result := workspacediscovery.DiscoverMCPTools(context.Background(), workspacediscovery.MCPToolsOptions{
		Cache: &cache,
		CacheKey: func(gotAgentID uuid.UUID) (workspacediscovery.MCPToolsCacheKey, bool) {
			require.Equal(t, agentID, gotAgentID)
			return key, true
		},
		ResolveWorkspaceAgentID: func(context.Context) (uuid.UUID, error) {
			return agentID, nil
		},
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			t.Fatal("cache hit should not dial workspace connection")
			return nil, xerrors.New("unexpected workspace connection dial")
		},
		DiscoveryTimeout: time.Second,
		Logger:           slog.Make(),
	})

	require.True(t, result.OK)
	require.Equal(t, key, result.Key)
	require.Len(t, result.Tools, 1)
	require.Equal(t, "workspace__cached", result.Tools[0].Name)
}
