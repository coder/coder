package chatd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestChatContextDirtyFromAgentPush is an end-to-end check of the chat
// context integration. An echo-provisioned workspace agent pushes a context
// snapshot that hydrates a bound chat; a later push with a different hash
// marks the chat dirty; the experimental API reports the dirty state and the
// snapshot error; the refresh endpoint re-pins the latest snapshot and clears
// it; and a re-push of the now-pinned hash stays clean. A second chat bound to
// no agent stays untouched throughout, guarding the agent-scoped queries.
func TestChatContextDirtyFromAgentPush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         directChatRoutingDeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Build a workspace with an agent via the echo provisioner.
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	ws, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Len(t, ws.LatestBuild.Resources, 1)
	require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	// A chat bound to the agent. In production agent_id is set lazily during
	// a workspace turn (chatd.persistBuildAgentBinding); bind it directly here
	// so the test exercises the context flow rather than turn resolution.
	// dbgen.ChatModelConfig provisions an AI provider as needed so the chat's
	// last_model_config_id foreign key is satisfied.
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	// An unrelated chat bound to no agent. The hydrate and dirty queries
	// scope by agent_id, so this chat must stay untouched by every push
	// below; it guards against the scoping clause silently breaking.
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	// Before any push there is no pinned context.
	got, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Nil(t, got.Context, "no pinned context before the first push")

	requireChatContextNil := func(id uuid.UUID, msg string) {
		t.Helper()
		unrelated, err := expClient.GetChat(ctx, id)
		require.NoError(t, err)
		require.Nil(t, unrelated.Context, msg)
	}
	requireChatContextNil(otherChat.ID, "agent-less chat has no pinned context")

	// Resource builders and a reader for the per-chat pinned copy. The agent
	// pushes these; hydration and refresh copy them onto the bound chat.
	agentsSource := "/home/coder/workspace/AGENTS.md"
	skillSource := "/home/coder/workspace/.agents/skills/example/SKILL.md"
	agentsV1Hash := []byte{0x11}
	agentsV2Hash := []byte{0x22}
	skillHash := []byte{0x33}
	instructionResource := func(source, content string, hash []byte) *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      source,
			ContentHash: hash,
			SizeBytes:   uint64(len(content)),
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_InstructionFile{
				InstructionFile: &agentproto.InstructionFileBody{Content: []byte(content)},
			},
		}
	}
	skillResource := func(source string, hash []byte) *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      source,
			ContentHash: hash,
			SizeBytes:   16,
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_Skill{
				Skill: &agentproto.SkillMetaBody{Meta: []byte("---\nname: example\n---"), Name: "example", Description: "demo skill"},
			},
		}
	}
	pinnedResources := func(id uuid.UUID) map[string]database.ChatContextResource {
		t.Helper()
		//nolint:gocritic // Test reads the chat-owned rows as the chatd subject; ctx carries no per-user actor.
		rows, lerr := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), id)
		require.NoError(t, lerr)
		out := make(map[string]database.ChatContextResource, len(rows))
		for _, r := range rows {
			out[r.Source] = r
		}
		return out
	}

	// Index the GET-only context resources by source.
	resourcesBySource := func(resources []codersdk.ChatContextResource) map[string]codersdk.ChatContextResource {
		out := make(map[string]codersdk.ChatContextResource, len(resources))
		for _, r := range resources {
			out[r.Source] = r
		}
		return out
	}

	// Connect as the agent and push the initial snapshot. The push runs the
	// hydrate/dirty fan-out synchronously inside its transaction, so the chat
	// reflects the change by the time the RPC returns.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken))
	aAPI, _, err := agentClient.ConnectRPC210(ctx)
	require.NoError(t, err)
	defer func() { _ = aAPI.DRPCConn().Close() }()

	hashA := []byte{0x01, 0x02, 0x03}
	resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: hashA,
		Resources: []*agentproto.ContextResource{
			instructionResource(agentsSource, "hello-v1", agentsV1Hash),
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	// The initial push hydrates the chat to a clean (not dirty) context.
	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context, "chat should be hydrated after the initial push")
	require.False(t, got.Context.Dirty, "initial hydration is clean")
	require.Nil(t, got.Context.DirtySince)

	// The single-chat GET surfaces the pinned resources.
	require.Len(t, got.Context.Resources, 1, "GET reports the pinned resources")
	require.Equal(t, agentsSource, got.Context.Resources[0].Source)
	require.Equal(t, codersdk.ChatContextResourceKindInstructionFile, got.Context.Resources[0].Kind)

	// The initial push also copied the agent's resources onto the chat.
	pinned := pinnedResources(chat.ID)
	require.Len(t, pinned, 1, "initial hydration copies the agent's resources")
	require.Equal(t, agentsV1Hash, pinned[agentsSource].ContentHash)
	require.Equal(t, database.WorkspaceAgentContextBodyKindInstructionFile, pinned[agentsSource].BodyKind)
	require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, pinned[agentsSource].Status)
	require.Empty(t, pinnedResources(otherChat.ID), "agent-less chat has no pinned resources")

	// The agent refreshes its context and pushes a different hash carrying a
	// snapshot-level error, which drifts from the pinned hash and marks the
	// chat dirty.
	hashB := []byte{0x04, 0x05, 0x06}
	const snapshotError = "two sources failed to resolve"
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       2,
		AggregateHash: hashB,
		SnapshotError: snapshotError,
		Resources: []*agentproto.ContextResource{
			instructionResource(agentsSource, "hello-v2", agentsV2Hash),
			skillResource(skillSource, skillHash),
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.True(t, got.Context.Dirty, "drift should mark the chat dirty")
	require.NotNil(t, got.Context.DirtySince)
	require.Empty(t, got.Context.Error, "dirty marking leaves the pinned hash and error unchanged")
	requireChatContextNil(otherChat.ID, "agent-less chat unaffected by the dirty fan-out")

	// While dirty the GET still reports the pinned (hashA) resources.
	require.Len(t, got.Context.Resources, 1, "resources stay pinned while dirty")
	require.Equal(t, agentsSource, got.Context.Resources[0].Source)

	// The dirty fan-out must NOT re-copy resources: the chat keeps the bodies
	// from its pinned (hashA) snapshot until it is refreshed.
	pinned = pinnedResources(chat.ID)
	require.Len(t, pinned, 1, "dirty marking does not re-copy resources")
	require.Equal(t, agentsV1Hash, pinned[agentsSource].ContentHash, "chat keeps the pinned snapshot's resources while dirty")

	// Refreshing re-pins the latest snapshot (hash and error) and clears the
	// dirty marker.
	refreshed, err := expClient.RefreshChatContext(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.Context)
	require.False(t, refreshed.Context.Dirty, "refresh clears the dirty marker")
	require.Equal(t, snapshotError, refreshed.Context.Error, "refresh re-pins the snapshot error")

	// The refresh response itself must carry the freshly pinned resources, so
	// the client reflects the refresh without a full reload. A regression here
	// blanks the context indicator until the page is reloaded (which
	// re-fetches via GET).
	refreshRespResources := resourcesBySource(refreshed.Context.Resources)
	require.Len(t, refreshRespResources, 2, "refresh response includes the re-pinned resources")
	require.Equal(t, codersdk.ChatContextResourceKindInstructionFile, refreshRespResources[agentsSource].Kind)
	require.Equal(t, codersdk.ChatContextResourceKindSkill, refreshRespResources[skillSource].Kind)
	require.Equal(t, "example", refreshRespResources[skillSource].SkillName)

	// Refresh re-pinned the agent's current resources (the hashB set).
	pinned = pinnedResources(chat.ID)
	require.Len(t, pinned, 2, "refresh re-pins the agent's current resources")
	require.Equal(t, agentsV2Hash, pinned[agentsSource].ContentHash)
	require.Equal(t, skillHash, pinned[skillSource].ContentHash)
	require.Equal(t, database.WorkspaceAgentContextBodyKindSkill, pinned[skillSource].BodyKind)

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.False(t, got.Context.Dirty)

	// Refresh advanced the pin to hashB, so the GET now reports both pinned
	// resources.
	refreshedResources := resourcesBySource(got.Context.Resources)
	require.Len(t, refreshedResources, 2, "refresh re-pins both resources for the GET")
	require.Equal(t, codersdk.ChatContextResourceKindInstructionFile, refreshedResources[agentsSource].Kind)
	require.Equal(t, codersdk.ChatContextResourceKindSkill, refreshedResources[skillSource].Kind)
	require.Equal(t, "example", refreshedResources[skillSource].SkillName)

	// Re-pushing the now-pinned hash proves the refresh advanced the pin to
	// hashB: a matching hash must not re-dirty the chat.
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       3,
		AggregateHash: hashB,
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.False(t, got.Context.Dirty, "re-push of the pinned hash stays clean")
}
