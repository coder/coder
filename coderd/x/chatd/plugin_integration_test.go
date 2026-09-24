package chatd_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestChatContextPluginSkillsFromAgentPush runs a real workspace agent
// against a scan root holding an Agent Plugin (manifest plus one skill) and
// a plain workspace skill, sends one chat turn, and checks three views of
// the result: the rows pinned to the chat, the GET /chats/{id} context
// listing, and the <available-skills> index in the system prompt the model
// received.
func TestChatContextPluginSkillsFromAgentPush(t *testing.T) {
	t.Parallel()

	const (
		pluginName             = "acme"
		pluginSkillName        = "deploy"
		pluginSkillDescription = "Deploy via acme"
		plainSkillName         = "review"
		plainSkillDescription  = "Review code"
	)

	type turnResult struct {
		// Sources are canonical on-disk paths of the resources the agent
		// scans; they key both pinned and listed.
		pluginSource      string
		pluginSkillSource string
		plainSkillSource  string
		// pinned holds the chat_context_resources rows produced by the
		// test's scan root, keyed by source.
		pinned map[string]database.ChatContextResource
		// listed holds the GET /chats/{id} context resources keyed by
		// source.
		listed map[string]codersdk.ChatContextResource
		// systemPrompt is the concatenated system messages of the first
		// streaming LLM request.
		systemPrompt string
	}

	runPluginTurn := func(t *testing.T) turnResult {
		t.Helper()

		ctx := testutil.Context(t, testutil.WaitSuperLong)

		// The scan root is registered as a user source, so the agent
		// discovers .agents/plugins/* as plugins and .agents/skills/* as
		// plain skills without relying on HOME or the working directory.
		root, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		pluginSource := filepath.Join(root, ".agents", "plugins", pluginName)
		pluginSkillSource := filepath.Join(pluginSource, "skills", pluginSkillName)
		plainSkillSource := filepath.Join(root, ".agents", "skills", plainSkillName)
		require.NoError(t, os.MkdirAll(pluginSkillSource, 0o755))
		require.NoError(t, os.MkdirAll(plainSkillSource, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(pluginSource, "plugin.json"),
			[]byte(`{"$schema":"`+agentcontext.PluginSchemaV1+`","name":"`+pluginName+`","version":"1.0.0","description":"Acme tools"}`),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(pluginSkillSource, "SKILL.md"),
			[]byte("---\nname: "+pluginSkillName+"\ndescription: "+pluginSkillDescription+"\n---\nRun the acme deploy.\n"),
			0o600,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(plainSkillSource, "SKILL.md"),
			[]byte("---\nname: "+plainSkillName+"\ndescription: "+plainSkillDescription+"\n---\nReview it.\n"),
			0o600,
		))

		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		db := api.Database
		aibridgedtest.StartTestAIBridgeDaemon(ctx, t, api, nil)
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

		_ = agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{SkillsDirs: root}
		})
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		ws, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err)
		require.Len(t, ws.LatestBuild.Resources, 1)
		require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)
		agentID := ws.LatestBuild.Resources[0].Agents[0].ID

		// The chat binds its agent on the first turn and pins that agent's
		// pushed snapshot, so the push must land before the turn starts.
		wantPushed := []string{plainSkillSource, pluginSource, pluginSkillSource}
		require.Eventually(t, func() bool {
			//nolint:gocritic // Test reads agent-owned rows; ctx carries no per-user actor.
			pushed, lerr := db.ListWorkspaceAgentContextResources(dbauthz.AsSystemRestricted(ctx), agentID)
			if lerr != nil {
				return false
			}
			have := make(map[string]struct{}, len(pushed))
			for _, r := range pushed {
				have[r.Source] = struct{}{}
			}
			for _, want := range wantPushed {
				if _, ok := have[want]; !ok {
					return false
				}
			}
			return true
		}, testutil.WaitSuperLong, testutil.IntervalFast)

		var streamedCallsMu sync.Mutex
		streamedCalls := make([][]chattest.OpenAIMessage, 0, 1)
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("plugin test")
			}
			streamedCallsMu.Lock()
			streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
			streamedCallsMu.Unlock()
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Got it.")...)
		})
		coderdtest.CreateOpenAICompatChatModel(t, expClient, openAIURL)

		workspaceID := workspace.ID
		chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			WorkspaceID:    &workspaceID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Which skills are available?",
			}},
		})
		require.NoError(t, err)

		settled := coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
		require.Equal(t, database.ChatStatusWaiting, settled.Status, "turn should complete without error")

		streamedCallsMu.Lock()
		recordedCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
		streamedCallsMu.Unlock()
		require.NotEmpty(t, recordedCalls, "LLM should have received at least one streaming request")
		var systemPrompt string
		for _, msg := range recordedCalls[0] {
			if msg.Role == "system" {
				systemPrompt += msg.Content + "\n"
			}
		}

		got, err := expClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Context, "chat should be hydrated after the first turn")
		listed := make(map[string]codersdk.ChatContextResource, len(got.Context.Resources))
		for _, r := range got.Context.Resources {
			listed[r.Source] = r
		}

		//nolint:gocritic // Test reads the chat-owned rows as the chatd subject; ctx carries no per-user actor.
		rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
		require.NoError(t, err)
		// Built-in scan roots under HOME may contribute rows; only rows
		// attributed to the test's scan root are counted.
		pinned := make(map[string]database.ChatContextResource, len(rows))
		for _, r := range rows {
			if r.SourcePath == root {
				pinned[r.Source] = r
			}
		}

		return turnResult{
			pluginSource:      pluginSource,
			pluginSkillSource: pluginSkillSource,
			plainSkillSource:  plainSkillSource,
			pinned:            pinned,
			listed:            listed,
			systemPrompt:      systemPrompt,
		}
	}

	res := runPluginTurn(t)

	require.Len(t, res.pinned, 3, "plugin, plugin skill, and plain skill are pinned")
	require.Equal(t, database.WorkspaceAgentContextBodyKindPlugin, res.pinned[res.pluginSource].BodyKind)
	require.Equal(t, database.WorkspaceAgentContextBodyKindSkill, res.pinned[res.pluginSkillSource].BodyKind)
	require.Equal(t, database.WorkspaceAgentContextBodyKindSkill, res.pinned[res.plainSkillSource].BodyKind)

	require.Equal(t, codersdk.ChatContextResourceKindPlugin, res.listed[res.pluginSource].Kind)
	require.Equal(t, pluginName, res.listed[res.pluginSource].PluginName)
	require.Equal(t, codersdk.ChatContextResourceStatusOK, res.listed[res.pluginSource].Status)

	require.Equal(t, codersdk.ChatContextResourceKindSkill, res.listed[res.pluginSkillSource].Kind)
	require.Equal(t, pluginSkillName, res.listed[res.pluginSkillSource].SkillName)
	require.Equal(t, pluginSkillDescription, res.listed[res.pluginSkillSource].SkillDescription)
	require.Equal(t, pluginName, res.listed[res.pluginSkillSource].PluginName)

	require.Equal(t, plainSkillName, res.listed[res.plainSkillSource].SkillName)
	require.Empty(t, res.listed[res.plainSkillSource].PluginName)

	require.Contains(t, res.systemPrompt, "<available-skills>")
	require.Contains(t, res.systemPrompt,
		"- "+pluginSkillName+" (plugin: "+pluginName+"): "+pluginSkillDescription+"\n",
		"plugin skill should be indexed with its plugin label")
	require.Contains(t, res.systemPrompt,
		"- "+plainSkillName+": "+plainSkillDescription+"\n",
		"plain workspace skill should be indexed without a plugin label")
}
