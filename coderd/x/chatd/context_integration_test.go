package chatd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestChatContextDirtyFromAgentPush is an end-to-end check of the chat
// context integration. An echo-provisioned workspace agent pushes a context
// snapshot that hydrates a bound chat; a later push with a different hash
// marks the chat dirty; the experimental API reports the dirty state and the
// snapshot error; returning to the pinned snapshot clears the dirty marker;
// snapshot errors live-sync without marking the chat dirty; and refresh re-pins
// the snapshot. Watch events carry each context change. An agent-less chat stays
// untouched throughout.
func TestChatContextDirtyFromAgentPush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentChatACPSubagents)}
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
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
	harnessSource := "codex"
	harnessHash := []byte{0x44}
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

	// Connect as the agent and push the initial snapshot. The push updates chat
	// context synchronously inside its transaction, so the chat reflects the
	// change by the time the RPC returns.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken))
	aAPI, _, err := agentClient.ConnectRPC210(ctx)
	require.NoError(t, err)
	defer func() { _ = aAPI.DRPCConn().Close() }()

	events := subscribeChatContextEvents(t, api.Pubsub, user.UserID, chat.ID)

	hashA := []byte{0x01, 0x02, 0x03}
	resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: hashA,
		Resources: []*agentproto.ContextResource{
			instructionResource(agentsSource, "hello-v1", agentsV1Hash),
			{
				Source: harnessSource, ContentHash: harnessHash, Status: agentproto.ContextResource_OK,
				Body: &agentproto.ContextResource_AcpHarness{
					AcpHarness: &agentproto.ACPHarnessBody{Slug: harnessSource, DisplayName: "Codex"},
				},
			},
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
	requireChatContextEvent(ctx, t, events, false, "")

	// The single-chat GET surfaces the pinned resources.
	require.Len(t, got.Context.Resources, 2, "GET reports the pinned resources")
	initialResources := resourcesBySource(got.Context.Resources)
	require.Equal(t, codersdk.ChatContextResourceKindInstructionFile, initialResources[agentsSource].Kind)
	require.Equal(t, codersdk.ChatContextResourceKindACPHarness, initialResources[harnessSource].Kind)

	// The initial push also copied the agent's resources onto the chat.
	pinned := pinnedResources(chat.ID)
	require.Len(t, pinned, 2, "initial hydration copies the agent's resources")
	require.Equal(t, agentsV1Hash, pinned[agentsSource].ContentHash)
	require.Equal(t, database.WorkspaceAgentContextBodyKindInstructionFile, pinned[agentsSource].BodyKind)
	require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, pinned[agentsSource].Status)
	require.Equal(t, harnessHash, pinned[harnessSource].ContentHash)
	require.Equal(t, database.WorkspaceAgentContextBodyKindAcpHarness, pinned[harnessSource].BodyKind)
	require.JSONEq(t, `{"slug":"codex","display_name":"Codex"}`, string(pinned[harnessSource].Body))
	require.Empty(t, pinnedResources(otherChat.ID), "agent-less chat has no pinned resources")

	// A later push changes the aggregate hash and reports a snapshot error.
	// The hash difference marks the chat dirty.
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
	require.True(t, got.Context.Dirty, "a changed aggregate hash marks the chat dirty")
	require.NotNil(t, got.Context.DirtySince)
	require.Equal(t, snapshotError, got.Context.Error, "snapshot errors live-sync while resources stay pinned")
	requireChatContextEvent(ctx, t, events, true, snapshotError)
	requireChatContextNil(otherChat.ID, "agent-less chat unaffected by context updates")

	// While dirty the GET still reports the pinned (hashA) resources.
	require.Len(t, got.Context.Resources, 1, "resources stay pinned while dirty")
	require.Equal(t, agentsSource, got.Context.Resources[0].Source)

	// Instruction bodies stay pinned to hashA until refresh.
	pinned = pinnedResources(chat.ID)
	require.Len(t, pinned, 1, "dirty marking does not re-copy resources")
	require.Equal(t, agentsV1Hash, pinned[agentsSource].ContentHash, "chat keeps the pinned snapshot's resources while dirty")

	pushAndGet := func(version uint64, hash []byte, snapshotError string, resources ...*agentproto.ContextResource) codersdk.Chat {
		t.Helper()
		resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
			Version:       version,
			AggregateHash: hash,
			SnapshotError: snapshotError,
			Resources:     resources,
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
		got, err := expClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Context)
		return got
	}

	const watcherError = "context watcher degraded"
	got = pushAndGet(3, hashA, watcherError, instructionResource(agentsSource, "hello-v1", agentsV1Hash))
	require.False(t, got.Context.Dirty, "returning to the pinned snapshot clears the dirty marker")
	require.Nil(t, got.Context.DirtySince)
	require.Equal(t, watcherError, got.Context.Error)
	requireChatContextEvent(ctx, t, events, false, watcherError)
	require.Equal(t, agentsV1Hash, pinnedResources(chat.ID)[agentsSource].ContentHash)

	got = pushAndGet(4, hashA, snapshotError, instructionResource(agentsSource, "hello-v1", agentsV1Hash))
	require.False(t, got.Context.Dirty, "an error-only change does not mark the chat dirty")
	require.Equal(t, snapshotError, got.Context.Error)
	requireChatContextEvent(ctx, t, events, false, snapshotError)

	// Mark the chat dirty again so refresh has a change to adopt.
	pushAndGet(5, hashB, snapshotError,
		instructionResource(agentsSource, "hello-v2", agentsV2Hash),
		skillResource(skillSource, skillHash),
	)

	requireChatContextEvent(ctx, t, events, true, snapshotError)

	// Refreshing re-pins the latest snapshot and clears the dirty marker.
	refreshed, err := expClient.RefreshChatContext(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.Context)
	require.False(t, refreshed.Context.Dirty, "refresh clears the dirty marker")
	require.Equal(t, snapshotError, refreshed.Context.Error)
	requireChatContextEvent(ctx, t, events, false, snapshotError)

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
	got = pushAndGet(6, hashB, snapshotError)
	require.False(t, got.Context.Dirty, "re-push of the pinned aggregate hash stays clean")

	got = pushAndGet(7, hashB, "")
	require.False(t, got.Context.Dirty, "resolving the snapshot error does not mark the chat dirty")
	require.Empty(t, got.Context.Error)
	requireChatContextEvent(ctx, t, events, false, "")
}

func TestChatContextAdoptsNewBuildFromAgentPush(t *testing.T) {
	t.Parallel()

	const (
		chatAgentName           = "workspace-coderd-chat"
		sidecarAgentName        = "aaa-sidecar"
		oldContextHash     byte = 0x01
		newContextHash     byte = 0x02
		sidecarContextHash byte = 0x03
		restartContextHash byte = 0x04
		driftedSidecarHash byte = 0x05
	)

	ctx := testutil.Context(t, testutil.WaitLong)
	// In-memory publication finishes before the RPC returns, including when
	// no event is expected for an unchanged pin.
	ps := dbpubsub.NewInMemory()
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		Database:                 rawDB,
		Pubsub:                   ps,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)
	//nolint:gocritic // Inspect chat-owned context as the daemon subject.
	chatCtx := dbauthz.AsChatd(ctx)

	newVersion := func(templateID uuid.UUID, token, sidecarToken string) codersdk.TemplateVersion {
		t.Helper()
		graph := echo.ProvisionGraphWithAgent(token, func(g *proto.GraphComplete) {
			g.Resources[0].Agents[0].Name = chatAgentName
			g.Resources[0].Agents = append(g.Resources[0].Agents, &proto.Agent{
				Id:   uuid.NewString(),
				Name: sidecarAgentName,
				Auth: &proto.Agent_Token{Token: sidecarToken},
			})
		})
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: graph,
		}, func(req *codersdk.CreateTemplateVersionRequest) {
			req.TemplateID = templateID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		return version
	}
	chatAgentIDs := func(workspace codersdk.Workspace) (chatAgentID, sidecarID uuid.UUID) {
		t.Helper()
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 2)
		for _, agent := range workspace.LatestBuild.Resources[0].Agents {
			switch agent.Name {
			case chatAgentName:
				chatAgentID = agent.ID
			case sidecarAgentName:
				sidecarID = agent.ID
			default:
				t.Fatalf("unexpected workspace agent %q", agent.Name)
			}
		}
		require.NotEqual(t, uuid.Nil, chatAgentID)
		require.NotEqual(t, uuid.Nil, sidecarID)
		return chatAgentID, sidecarID
	}
	connectAgent := func(token string) agentproto.DRPCAgentClient210 {
		t.Helper()
		agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(token))
		rpc, _, err := agentClient.ConnectRPC210(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = rpc.DRPCConn().Close() })
		return rpc
	}
	push := func(rpc agentproto.DRPCAgentClient210, request *agentproto.PushContextStateRequest) {
		t.Helper()
		resp, err := rpc.PushContextState(ctx, request)
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	}
	instructionResource := func(source, content string, hash byte) *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      source,
			ContentHash: []byte{hash},
			SizeBytes:   uint64(len(content)),
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_InstructionFile{
				InstructionFile: &agentproto.InstructionFileBody{Content: []byte(content)},
			},
		}
	}
	readChat := func(id uuid.UUID) database.Chat {
		t.Helper()
		chat, err := db.GetChatByID(chatCtx, id)
		require.NoError(t, err)
		return chat
	}
	pinnedResources := func(id uuid.UUID) []database.ChatContextResource {
		t.Helper()
		resources, err := db.ListChatContextResourcesByChatID(chatCtx, id)
		require.NoError(t, err)
		return resources
	}

	// Provision the prior build with a chat agent and a sidecar.
	oldToken, oldSidecarToken := uuid.NewString(), uuid.NewString()
	oldVersion := newVersion(uuid.Nil, oldToken, oldSidecarToken)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, oldVersion.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	oldWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	oldAgentID, oldSidecarID := chatAgentIDs(oldWorkspace)
	workspaceID := uuid.NullUUID{UUID: workspace.ID, Valid: true}
	oldBuildID := uuid.NullUUID{UUID: oldWorkspace.LatestBuild.ID, Valid: true}
	oldBinding := uuid.NullUUID{UUID: oldAgentID, Valid: true}
	oldRPC := connectAgent(oldToken)
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	newChat := func(seed database.Chat) database.Chat {
		t.Helper()
		seed.OrganizationID = user.OrganizationID
		seed.OwnerID = user.UserID
		seed.LastModelConfigID = model.ID
		return dbgen.Chat(t, db, seed)
	}

	newOldBuildChat := func(agentID uuid.NullUUID, status database.ChatStatus) database.Chat {
		t.Helper()
		return newChat(database.Chat{
			WorkspaceID: workspaceID,
			BuildID:     oldBuildID,
			AgentID:     agentID,
			Status:      status,
		})
	}
	idle := newOldBuildChat(oldBinding, database.ChatStatusWaiting)
	errored := newOldBuildChat(oldBinding, database.ChatStatusError)
	lifecycleChat := newOldBuildChat(oldBinding, database.ChatStatusWaiting)
	unchanged := newOldBuildChat(oldBinding, database.ChatStatusWaiting)
	archived := newOldBuildChat(oldBinding, database.ChatStatusWaiting)
	matchingDirty := newOldBuildChat(uuid.NullUUID{UUID: oldSidecarID, Valid: true}, database.ChatStatusWaiting)

	// The sidecar pins matchingDirty to the latest build's eventual hash.
	oldSidecarRPC := connectAgent(oldSidecarToken)
	push(oldSidecarRPC, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: []byte{newContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/new/AGENTS.md", "new instructions", newContextHash)},
	})
	// A later sidecar snapshot leaves that pin dirty.
	push(oldSidecarRPC, &agentproto.PushContextStateRequest{
		Version:       2,
		Initial:       false,
		AggregateHash: []byte{driftedSidecarHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/drifted/AGENTS.md", "drifted instructions", driftedSidecarHash)},
	})
	matchingDirtyBefore := readChat(matchingDirty.ID)
	require.Equal(t, []byte{newContextHash}, matchingDirtyBefore.ContextAggregateHash)
	require.True(t, matchingDirtyBefore.ContextDirtySince.Valid)
	matchingDirtyResources := pinnedResources(matchingDirty.ID)
	require.Len(t, matchingDirtyResources, 1)
	// The old chat agent pins its chats to the prior context.
	push(oldRPC, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: []byte{oldContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/old/AGENTS.md", "old instructions", oldContextHash)},
	})
	// Its next snapshot marks those pins dirty.
	push(oldRPC, &agentproto.PushContextStateRequest{
		Version:       2,
		Initial:       false,
		AggregateHash: []byte{newContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/new/AGENTS.md", "new instructions", newContextHash)},
	})
	// Refresh one chat to match the latest build's eventual snapshot.
	_, err = expClient.RefreshChatContext(ctx, unchanged.ID)
	require.NoError(t, err)
	unchangedResources := pinnedResources(unchanged.ID)
	require.Len(t, unchangedResources, 1)

	// Workspace lifecycle tools clear the agent binding but keep the pin.
	lifecycleChat, err = db.UpdateChatBuildAgentBinding(chatCtx, database.UpdateChatBuildAgentBindingParams{
		ID:      lifecycleChat.ID,
		BuildID: lifecycleChat.BuildID,
	})
	require.NoError(t, err)
	_, err = db.ArchiveChatByID(chatCtx, archived.ID)
	require.NoError(t, err)
	unpinned := newChat(database.Chat{
		WorkspaceID: workspaceID,
		BuildID:     oldBuildID,
		Status:      database.ChatStatusWaiting,
	})
	otherWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     template.ID,
	})
	otherChat := newChat(database.Chat{
		WorkspaceID: uuid.NullUUID{UUID: otherWorkspace.ID, Valid: true},
		Status:      database.ChatStatusWaiting,
	})
	require.NoError(t, db.SetChatContextSnapshot(chatCtx, database.SetChatContextSnapshotParams{
		ID:            otherChat.ID,
		AggregateHash: []byte{oldContextHash},
	}))

	// Complete the latest build and verify prior-agent cleanup.
	newToken, sidecarToken := uuid.NewString(), uuid.NewString()
	latestVersion := newVersion(template.ID, newToken, sidecarToken)
	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition:        codersdk.WorkspaceTransitionStart,
		TemplateVersionID: latestVersion.ID,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
	newWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	newAgentID, sidecarID := chatAgentIDs(newWorkspace)
	newBuildID := uuid.NullUUID{UUID: build.ID, Valid: true}
	newBinding := uuid.NullUUID{UUID: newAgentID, Valid: true}
	require.NotEqual(t, oldAgentID, newAgentID)
	for _, previousAgentID := range []uuid.UUID{oldAgentID, oldSidecarID} {
		requirePriorAgentPurged(chatCtx, t, db, sqlDB, previousAgentID)
	}
	alreadyBound := newChat(database.Chat{
		WorkspaceID: workspaceID,
		BuildID:     newBuildID,
		AgentID:     newBinding,
		Status:      database.ChatStatusWaiting,
	})
	require.NoError(t, db.SetChatContextSnapshot(chatCtx, database.SetChatContextSnapshotParams{
		ID:            alreadyBound.ID,
		AggregateHash: []byte{oldContextHash},
	}))
	sidecarChat := newChat(database.Chat{
		WorkspaceID: workspaceID,
		BuildID:     newBuildID,
		AgentID:     uuid.NullUUID{UUID: sidecarID, Valid: true},
		Status:      database.ChatStatusWaiting,
	})

	events := subscribeChatContextEvents(t, ps, user.UserID, uuid.Nil)

	// A sidecar push must not adopt chats from the prior build.
	sidecarRPC := connectAgent(sidecarToken)
	push(sidecarRPC, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: []byte{sidecarContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/sidecar/AGENTS.md", "sidecar instructions", sidecarContextHash)},
	})
	require.Equal(t, oldBinding, readChat(idle.ID).AgentID, "the unselected agent cannot adopt chats")
	event := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, sidecarChat.ID, event.Chat.ID)
	require.Empty(t, events)

	// The selected chat agent adopts the latest build's context.
	newRPC := connectAgent(newToken)
	// The first accepted push need not carry Initial, which is process-scoped.
	push(newRPC, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       false,
		AggregateHash: []byte{newContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/new/AGENTS.md", "new instructions", newContextHash)},
	})
	adoptedChats := []database.Chat{idle, errored, lifecycleChat, alreadyBound, matchingDirty, unchanged}
	for _, before := range adoptedChats {
		chat := readChat(before.ID)
		require.Equal(t, newBinding, chat.AgentID)
		require.Equal(t, newBuildID, chat.BuildID)
		require.Equal(t, []byte{newContextHash}, chat.ContextAggregateHash)
		require.False(t, chat.ContextDirtySince.Valid)
		require.Equal(t, before.UpdatedAt, chat.UpdatedAt, "adoption does not reorder idle chats")
		resources := pinnedResources(chat.ID)
		require.Len(t, resources, 1)
		require.Equal(t, "/new/AGENTS.md", resources[0].Source)
	}
	require.Equal(t, unchangedResources, pinnedResources(unchanged.ID), "matching pins are not rewritten")
	require.Equal(t, matchingDirtyResources, pinnedResources(matchingDirty.ID), "clearing old-build drift preserves matching pinned resources")

	expectedEvents := []uuid.UUID{idle.ID, errored.ID, lifecycleChat.ID, alreadyBound.ID, matchingDirty.ID}
	var eventChatIDs []uuid.UUID
	for range expectedEvents {
		event := testutil.RequireReceive(ctx, t, events)
		require.False(t, event.Chat.Context.Dirty)
		eventChatIDs = append(eventChatIDs, event.Chat.ID)
	}
	require.ElementsMatch(t, expectedEvents, eventChatIDs)
	require.Empty(t, events, "unchanged context emits no event")
	require.Equal(t, oldBinding, readChat(archived.ID).AgentID)
	require.Equal(t, []byte{oldContextHash}, readChat(archived.ID).ContextAggregateHash)
	require.False(t, readChat(unpinned.ID).AgentID.Valid, "an unbound chat without a pin stays unbound")
	require.Nil(t, readChat(unpinned.ID).ContextAggregateHash)
	require.Equal(t, sidecarID, readChat(sidecarChat.ID).AgentID.UUID)
	require.Equal(t, []byte{sidecarContextHash}, readChat(sidecarChat.ID).ContextAggregateHash)
	require.False(t, readChat(otherChat.ID).AgentID.Valid)
	require.Equal(t, []byte{oldContextHash}, readChat(otherChat.ID).ContextAggregateHash)

	// A process restart in the same build still requires a context refresh.
	push(newRPC, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: []byte{restartContextHash},
		Resources:     []*agentproto.ContextResource{instructionResource("/changed/AGENTS.md", "changed instructions", restartContextHash)},
	})
	for _, before := range adoptedChats {
		chat := readChat(before.ID)
		require.Equal(t, []byte{newContextHash}, chat.ContextAggregateHash)
		require.True(t, chat.ContextDirtySince.Valid)
	}
}

// TestChatContextLiveSyncFromAgentPush verifies that agent pushes live-sync MCP
// and ACP resources across bound chats without dirtying them, while prompt
// resources remain pinned until refresh.
func TestChatContextLiveSyncFromAgentPush(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentChatACPSubagents)}
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

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

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	newBoundChat := func() database.Chat {
		return dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
			AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
			LastModelConfigID: model.ID,
			Status:            database.ChatStatusWaiting,
		})
	}
	chat := newBoundChat()
	secondChat := newBoundChat()
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	agentsSource := "/home/coder/workspace/AGENTS.md"
	mcpSource := "grafana"
	agentsHash := []byte{0x11}
	toolsV1Hash := []byte{0x21}
	toolsV2Hash := []byte{0x22}
	instructionResource := func() *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      agentsSource,
			ContentHash: agentsHash,
			SizeBytes:   8,
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_InstructionFile{
				InstructionFile: &agentproto.InstructionFileBody{Content: []byte("hello-v1")},
			},
		}
	}
	mcpServerResource := func(hash []byte, toolNames ...string) *agentproto.ContextResource {
		tools := make([]*agentproto.MCPTool, 0, len(toolNames))
		for _, name := range toolNames {
			tools = append(tools, &agentproto.MCPTool{
				// The agent reports tool names prefixed with "<server>__".
				Name:        mcpSource + "__" + name,
				Description: name + " tool",
			})
		}
		return &agentproto.ContextResource{
			Source:      mcpSource,
			ContentHash: hash,
			SizeBytes:   32,
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_McpServer{
				McpServer: &agentproto.MCPServerBody{ServerName: mcpSource, Tools: tools},
			},
		}
	}
	harnessSource := "codex"
	harnessDir := "/home/coder/.coder/acp"
	harnessResource := func(hash []byte, displayName string) *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      harnessSource,
			SourcePath:  new(harnessDir),
			ContentHash: hash,
			SizeBytes:   32,
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_AcpHarness{
				AcpHarness: &agentproto.ACPHarnessBody{
					Slug: harnessSource, DisplayName: displayName,
				},
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
	requireClean := func(id uuid.UUID, msg string) codersdk.Chat {
		t.Helper()
		got, gerr := expClient.GetChat(ctx, id)
		require.NoError(t, gerr)
		require.NotNil(t, got.Context, msg)
		require.False(t, got.Context.Dirty, msg)
		return got
	}

	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken))
	aAPI, _, err := agentClient.ConnectRPC210(ctx)
	require.NoError(t, err)
	defer func() { _ = aAPI.DRPCConn().Close() }()

	// Model an initial push before the MCP engine connects.
	hash := []byte{0x01, 0x02}
	resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: hash,
		Resources:     []*agentproto.ContextResource{instructionResource()},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	requireClean(chat.ID, "chat hydrates clean from the MCP-less snapshot")
	require.Len(t, pinnedResources(chat.ID), 1, "no MCP rows before the engine connects")

	// A same-hash push must sync MCP rows without dirtying bound chats.
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       2,
		AggregateHash: hash,
		Resources: []*agentproto.ContextResource{
			instructionResource(),
			mcpServerResource(toolsV1Hash, "query", "search"),
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got := requireClean(chat.ID, "same-hash MCP push must not dirty the chat")
	for _, id := range []uuid.UUID{chat.ID, secondChat.ID} {
		pinned := pinnedResources(id)
		require.Len(t, pinned, 2, "MCP row synced onto chat %s", id)
		require.Equal(t, toolsV1Hash, pinned[mcpSource].ContentHash)
		require.Equal(t, database.WorkspaceAgentContextBodyKindMcpServer, pinned[mcpSource].BodyKind)
	}
	require.Empty(t, pinnedResources(otherChat.ID), "agent-less chat stays untouched")

	var mcpRes *codersdk.ChatContextResource
	for i, r := range got.Context.Resources {
		if r.Kind == codersdk.ChatContextResourceKindMCPServer {
			mcpRes = &got.Context.Resources[i]
		}
	}
	require.NotNil(t, mcpRes, "GET reports the synced MCP server")
	require.Equal(t, mcpSource, mcpRes.Source)
	toolNames := make([]string, 0, len(mcpRes.Tools))
	for _, tool := range mcpRes.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	require.ElementsMatch(t, []string{"query", "search"}, toolNames)

	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       3,
		AggregateHash: hash,
		Resources: []*agentproto.ContextResource{
			instructionResource(),
			mcpServerResource(toolsV2Hash, "query", "search", "annotate"),
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())
	requireClean(chat.ID, "tool-list change must not dirty the chat")
	require.Equal(t, toolsV2Hash, pinnedResources(chat.ID)[mcpSource].ContentHash, "changed tool list syncs")

	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       4,
		AggregateHash: hash,
		Resources:     []*agentproto.ContextResource{instructionResource()},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())
	requireClean(chat.ID, "server removal must not dirty the chat")
	for _, id := range []uuid.UUID{chat.ID, secondChat.ID} {
		pinned := pinnedResources(id)
		require.Len(t, pinned, 1, "removed server deleted from chat %s", id)
		require.NotContains(t, pinned, mcpSource)
	}

	// ACP discovery follows the initial context push and does not change its
	// hash. Metadata and probe failures must reach already-pinned chats too.
	harnessV1 := harnessResource([]byte{0x31}, "Codex")
	harnessV2 := harnessResource([]byte{0x32}, "Codex Updated")
	harnessFailed := harnessResource([]byte{0x32}, "Codex Updated")
	harnessFailed.Status = agentproto.ContextResource_INVALID
	harnessFailed.Error = "cannot initialize harness"
	pushVersion := uint64(5)
	for _, harness := range []*agentproto.ContextResource{harnessV1, harnessV2, harnessFailed} {
		status := database.WorkspaceAgentContextResourceStatusOk
		reportedSourcePath := harnessDir
		if harness.Status == agentproto.ContextResource_INVALID {
			status = database.WorkspaceAgentContextResourceStatusInvalid
			reportedSourcePath = ""
		}
		resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
			Version:       pushVersion,
			AggregateHash: hash,
			Resources:     []*agentproto.ContextResource{instructionResource(), harness},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())

		got = requireClean(chat.ID, "ACP changes must not dirty the chat")
		requireClean(secondChat.ID, "ACP changes must not dirty the second chat")
		body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(harness.GetAcpHarness())
		require.NoError(t, err)
		for _, id := range []uuid.UUID{chat.ID, secondChat.ID} {
			pinned := pinnedResources(id)
			require.Len(t, pinned, 2, "ACP row synced onto chat %s", id)
			require.Equal(t, database.WorkspaceAgentContextBodyKindAcpHarness, pinned[harnessSource].BodyKind)
			require.Equal(t, harness.ContentHash, pinned[harnessSource].ContentHash)
			require.Equal(t, harnessDir, pinned[harnessSource].SourcePath)
			require.Equal(t, status, pinned[harnessSource].Status)
			require.Equal(t, harness.Error, pinned[harnessSource].Error)
			require.JSONEq(t, string(body), string(pinned[harnessSource].Body))
		}
		require.Empty(t, pinnedResources(otherChat.ID), "agent-less chat stays untouched")
		require.Contains(t, got.Context.Resources, codersdk.ChatContextResource{
			Source: harnessSource, SourcePath: reportedSourcePath,
			Kind:      codersdk.ChatContextResourceKindACPHarness,
			SizeBytes: 32,
			Status:    codersdk.ChatContextResourceStatus(status),
			Error:     harness.Error,
		})
		pushVersion++
	}

	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       8,
		AggregateHash: hash,
		Resources:     []*agentproto.ContextResource{instructionResource()},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())
	for _, id := range []uuid.UUID{chat.ID, secondChat.ID} {
		requireClean(id, "harness removal must not dirty the chat")
		pinned := pinnedResources(id)
		require.Len(t, pinned, 1, "removed harness deleted from chat %s", id)
		require.NotContains(t, pinned, harnessSource)
	}

	// Pushes with a different hash still sync MCP/ACP rows while prompt rows
	// stay pinned.
	driftedHash := []byte{0x03, 0x04}
	changedInstruction := instructionResource()
	changedInstruction.ContentHash = []byte{0x12}
	changedInstruction.GetInstructionFile().Content = []byte("hello-v2")
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       9,
		AggregateHash: driftedHash,
		Resources: []*agentproto.ContextResource{
			changedInstruction,
			mcpServerResource(toolsV1Hash, "query"),
			harnessV1,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.True(t, got.Context.Dirty, "the changed aggregate hash marks the chat dirty")
	pinned := pinnedResources(chat.ID)
	require.Len(t, pinned, 3)
	require.Equal(t, toolsV1Hash, pinned[mcpSource].ContentHash, "MCP row syncs even on a dirty chat")
	require.Equal(t, harnessV1.ContentHash, pinned[harnessSource].ContentHash, "ACP row syncs even on a dirty chat")
	require.Equal(t, agentsHash, pinned[agentsSource].ContentHash, "prompt rows stay pinned while dirty")
}

// TestChatContextRefreshFromAgentToken covers the in-workspace
// `coder exp chat context refresh` (no chat argument) path, which authenticates
// with the agent token instead of a user session. The agent endpoint re-pins
// every dirty chat bound to the calling agent to its latest snapshot and
// clears the dirty marker, returning how many were refreshed. A chat bound to
// no agent must stay untouched, guarding the agent-scoped query.
func TestChatContextRefreshFromAgentToken(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	// Build a workspace with an agent via the echo provisioner so the agent
	// token is accepted by the agent middleware backing the endpoint.
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

	// Error chats accept new messages and must be included in agent-token refresh.
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusError,
	})
	otherChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	agentsSource := "/home/coder/workspace/AGENTS.md"
	instructionResource := func(content string, hash []byte) *agentproto.ContextResource {
		return &agentproto.ContextResource{
			Source:      agentsSource,
			ContentHash: hash,
			SizeBytes:   uint64(len(content)),
			Status:      agentproto.ContextResource_OK,
			Body: &agentproto.ContextResource_InstructionFile{
				InstructionFile: &agentproto.InstructionFileBody{Content: []byte(content)},
			},
		}
	}

	// The agent token drives both the DRPC push and the REST refresh.
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentToken))
	aAPI, _, err := agentClient.ConnectRPC210(ctx)
	require.NoError(t, err)
	defer func() { _ = aAPI.DRPCConn().Close() }()

	events := subscribeChatContextEvents(t, api.Pubsub, user.UserID, chat.ID)

	// Initial push hydrates the chat to a clean context.
	resp, err := aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       1,
		Initial:       true,
		AggregateHash: []byte{0x01},
		Resources:     []*agentproto.ContextResource{instructionResource("hello-v1", []byte{0x11})},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	// With nothing dirty, the agent-token refresh is a no-op.
	refresh, err := agentClient.RefreshChatContext(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, refresh.Refreshed, "no dirty chats to refresh")
	requireChatContextEvent(ctx, t, events, false, "")

	// A second push with a different hash marks the bound chat dirty.
	resp, err = aAPI.PushContextState(ctx, &agentproto.PushContextStateRequest{
		Version:       2,
		AggregateHash: []byte{0x02},
		Resources:     []*agentproto.ContextResource{instructionResource("hello-v2", []byte{0x22})},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())

	got, err := expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.True(t, got.Context.Dirty, "second push marks the chat dirty")
	requireChatContextEvent(ctx, t, events, true, "")

	// The agent-token refresh re-pins every dirty chat bound to the agent.
	refresh, err = agentClient.RefreshChatContext(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, refresh.Refreshed, "the dirty chat is re-pinned")
	requireChatContextEvent(ctx, t, events, false, "")

	got, err = expClient.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.False(t, got.Context.Dirty, "refresh clears the dirty marker")
	require.Len(t, got.Context.Resources, 1)
	require.Equal(t, agentsSource, got.Context.Resources[0].Source)

	// The agent-less chat is never returned by the agent-scoped query, so it
	// must stay unhydrated throughout.
	other, err := expClient.GetChat(ctx, otherChat.ID)
	require.NoError(t, err)
	require.Nil(t, other.Context, "agent-less chat stays untouched")

	// A follow-up refresh with nothing dirty is a no-op again.
	refresh, err = agentClient.RefreshChatContext(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, refresh.Refreshed, "nothing left to refresh")
}

// subscribeChatContextEvents watches context events for chatID, or every chat
// owned by ownerID when chatID is uuid.Nil.
func subscribeChatContextEvents(t *testing.T, ps dbpubsub.Pubsub, ownerID, chatID uuid.UUID) <-chan codersdk.ChatWatchEvent {
	t.Helper()
	events := make(chan codersdk.ChatWatchEvent, 16)
	cancel, err := ps.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, event codersdk.ChatWatchEvent, err error) {
			if !assert.NoError(t, err) {
				return
			}
			if event.Kind == codersdk.ChatWatchEventKindContextDirty && (chatID == uuid.Nil || event.Chat.ID == chatID) {
				events <- event
			}
		}),
	)
	require.NoError(t, err)
	t.Cleanup(cancel)
	return events
}

func requireChatContextEvent(ctx context.Context, t *testing.T, events <-chan codersdk.ChatWatchEvent, dirty bool, snapshotError string) {
	t.Helper()
	event := testutil.RequireReceive(ctx, t, events)
	require.NotNil(t, event.Chat.Context)
	require.Equal(t, dirty, event.Chat.Context.Dirty, "watchers see the committed context")
	require.Equal(t, snapshotError, event.Chat.Context.Error)
	require.Equal(t, dirty, event.Chat.Context.DirtySince != nil, "dirty markers and timestamps agree")
}

// agentMCPToolContext specifies an mcp_server tool to seed into an agent's
// pushed context snapshot.
type agentMCPToolContext struct {
	AgentID         uuid.UUID
	ServerName      string
	ToolName        string
	ToolDescription string
}

// seedAgentMCPToolContext upserts an mcp_server context snapshot and resource
// for the agent, mirroring what PushContextState writes, so a chat bound to the
// agent hydrates a pinned, execution-ready MCP tool. The model-facing tool name
// is "<ServerName>__<ToolName>". It seeds the raw store directly so unit tests
// can exercise pinned MCP execution without a live agent connection.
func seedAgentMCPToolContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	tool agentMCPToolContext,
) {
	t.Helper()

	schema, err := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
	})
	require.NoError(t, err)

	body, err := protojson.Marshal(&agentproto.MCPServerBody{
		ServerName: tool.ServerName,
		Tools: []*agentproto.MCPTool{{
			Name:        tool.ToolName,
			Description: tool.ToolDescription,
			InputSchema: schema,
		}},
	})
	require.NoError(t, err)

	now := dbtime.Now()
	hash := []byte(tool.ServerName + ":" + tool.ToolName)
	_, err = db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: tool.AgentID,
		Version:          1,
		AggregateHash:    hash,
		ReceivedAt:       now,
	})
	require.NoError(t, err)

	_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
		WorkspaceAgentID: tool.AgentID,
		Source:           tool.ServerName,
		BodyKind:         database.WorkspaceAgentContextBodyKindMcpServer,
		Body:             body,
		ContentHash:      hash,
		SizeBytes:        int64(len(body)),
		Status:           database.WorkspaceAgentContextResourceStatusOk,
		Now:              now,
	})
	require.NoError(t, err)
}

// seedAgentInstructionContext upserts an instruction_file context snapshot and
// resource for the agent, mirroring what PushContextState writes, so a chat
// bound to the agent hydrates a pinned instruction block. It seeds the raw
// store directly so unit tests can exercise pinned workspace context without a
// live agent connection.
func seedAgentInstructionContext(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	agentID uuid.UUID,
	source string,
	content string,
) {
	t.Helper()

	body, err := protojson.Marshal(&agentproto.InstructionFileBody{Content: []byte(content)})
	require.NoError(t, err)

	now := dbtime.Now()
	hash := []byte("instruction:" + source)
	_, err = db.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
		WorkspaceAgentID: agentID,
		Version:          1,
		AggregateHash:    hash,
		ReceivedAt:       now,
	})
	require.NoError(t, err)

	_, err = db.UpsertWorkspaceAgentContextResource(ctx, database.UpsertWorkspaceAgentContextResourceParams{
		WorkspaceAgentID: agentID,
		Source:           source,
		BodyKind:         database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:             body,
		ContentHash:      hash,
		SizeBytes:        int64(len(body)),
		Status:           database.WorkspaceAgentContextResourceStatusOk,
		Now:              now,
	})
	require.NoError(t, err)
}
