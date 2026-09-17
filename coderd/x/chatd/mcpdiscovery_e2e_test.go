package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// controllableMCPServer is an HTTP MCP fixture whose registration can be
// held (hung), fail, or be released on demand, with a per-fixture nonce so
// a tool result proves which process's server answered.
type controllableMCPServer struct {
	url               string
	toolName          string
	nonce             string
	arrived           chan struct{}
	release           func()
	invoked           atomic.Bool
	registrationError bool
}

func newControllableMCPServer(t *testing.T, toolName string) *controllableMCPServer {
	t.Helper()
	c := &controllableMCPServer{toolName: toolName, nonce: uuid.NewString(), arrived: make(chan struct{})}
	released := make(chan struct{})
	c.release = sync.OnceFunc(func() { close(released) })
	srv := mcp.NewServer(&mcp.Implementation{Name: "discovery-fixture", Version: "1"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        toolName,
		Description: "Echo the discovery fixture nonce",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string"}}},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c.invoked.Store(true)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: c.nonce + " " + string(req.Params.Arguments)}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true})
	arrived := sync.OnceFunc(func() { close(c.arrived) })
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived()
		select {
		case <-released:
			if c.registrationError {
				http.Error(w, "registration failed", http.StatusInternalServerError)
				return
			}
			handler.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	}))
	c.url = ts.URL
	t.Cleanup(func() { c.release(); ts.Close() })
	return c
}

// mcpDiscoveryStore releases the held MCP fixture once chatd's discovery
// wait has polled the snapshot twice from one context, proving the wait
// ran before the fixture answered. Transactional publication reads must
// not release the polling barrier.
type mcpDiscoveryStore struct {
	database.Store
	state *mcpDiscoveryBarrier
	inTx  bool
}

type mcpDiscoveryBarrier struct {
	sync.Mutex
	armed       bool
	lastRead    context.Context
	release     func()
	pushGate    chan struct{}
	pushArrived chan struct{}
	pushOnce    sync.Once
}

func (s *mcpDiscoveryStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&mcpDiscoveryStore{Store: tx, state: s.state, inTx: true})
	}, opts)
}

func (s *mcpDiscoveryStore) GetLatestWorkspaceAgentContextSnapshot(ctx context.Context, id uuid.UUID) (database.WorkspaceAgentContextSnapshot, error) {
	snap, err := s.Store.GetLatestWorkspaceAgentContextSnapshot(ctx, id)
	if !s.inTx {
		s.state.Lock()
		if s.state.armed {
			if s.state.lastRead == ctx {
				s.state.release()
				s.state.armed = false
			}
			s.state.lastRead = ctx
		}
		s.state.Unlock()
	}
	return snap, err
}

func (s *mcpDiscoveryStore) UpsertWorkspaceAgentContextSnapshot(ctx context.Context, arg database.UpsertWorkspaceAgentContextSnapshotParams) (database.WorkspaceAgentContextSnapshot, error) {
	s.state.Lock()
	gate := s.state.pushGate
	s.state.Unlock()
	if gate != nil {
		s.state.pushOnce.Do(func() { close(s.state.pushArrived) })
		select {
		case <-gate:
		case <-ctx.Done():
			return database.WorkspaceAgentContextSnapshot{}, ctx.Err()
		}
	}
	return s.Store.UpsertWorkspaceAgentContextSnapshot(ctx, arg)
}

// TestMCPDiscoveryGate_EndToEnd drives a real agent with a real HTTP MCP
// fixture through the chat's first turn: pinned-pending and unpinned
// chats, create/start workspace hooks (including same-batch find_tools),
// autostart, an agent process restart whose previous-run tools must be
// withheld, hung or failed registrations that must fail open within the
// shared budget, and a hung server whose healthy sibling must still be
// usable on that first turn.
func TestMCPDiscoveryGate_EndToEnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                                                            string
		unavailable                                                                     string
		tool                                                                            string
		pinned, missing, stopped, restart, sameBatch, settled, autostart, sibling, plan bool
	}{
		{name: "Attached_PinnedPending", pinned: true},
		{name: "NoMCPServers", unavailable: "empty", settled: true},
		{name: "RegistrationError", unavailable: "error", settled: true},
		{name: "HungRegistrationFailsOpen", unavailable: "hung"},
		{name: "HungSiblingKeepsHealthyServerUsable", unavailable: "hung", sibling: true},
		{name: "HungCreateWorkspace", tool: "create_workspace", unavailable: "hung"},
		{name: "HungStartWorkspace", tool: "start_workspace", stopped: true, unavailable: "hung"},
		{name: "PlanCreateWorkspace", tool: "create_workspace", unavailable: "hung", plan: true},
		{name: "PlanStartWorkspace", tool: "start_workspace", stopped: true, unavailable: "hung", plan: true},
		{name: "Attached_UnpinnedMissing", missing: true},
		{name: "Autostart", stopped: true, autostart: true},
		{name: "CreateWorkspace", tool: "create_workspace"},
		{name: "CreateWorkspace_SameBatch", tool: "create_workspace", sameBatch: true},
		{name: "StartWorkspace", tool: "start_workspace", stopped: true},
		{name: "StartWorkspace_SameBatch", tool: "start_workspace", stopped: true, sameBatch: true},
		{name: "Restart_PinnedStale", pinned: true, restart: true},
		{name: "StartWorkspace_AlreadyRunning", tool: "start_workspace", pinned: true, settled: true, sameBatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitSuperLong)
			rawDB, ps := dbtestutil.NewDB(t)
			barrier := &mcpDiscoveryBarrier{pushArrived: make(chan struct{})}
			db := &mcpDiscoveryStore{Store: rawDB, state: barrier}
			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = append(dv.Experiments, string(codersdk.ExperimentMCPToolSearch))
			tickCh := make(chan time.Time, 1)
			statsCh := make(chan autobuild.Stats, 1)
			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
				AutobuildTicker: tickCh, AutobuildStats: statsCh,
				Database: db, Pubsub: ps, DeploymentValues: dv, IncludeProvisionerDaemon: true,
			})
			aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
			user := coderdtest.CreateFirstUser(t, client)
			exp := codersdk.NewExperimentalClient(client)
			ctrl := newControllableMCPServer(t, "fresh-echo")
			ctrl.registrationError = tc.unavailable == "error"
			// usable is the fixture the first turn must reach: the healthy
			// sibling when the primary fixture is hung, otherwise the primary.
			usable := ctrl
			var sibling *controllableMCPServer
			if tc.sibling {
				sibling = newControllableMCPServer(t, "sibling-echo")
				sibling.release()
				usable = sibling
			}
			// unusable means no workspace MCP tool can be reached on the
			// first turn, so the model proceeds without one.
			unusable := tc.unavailable != "" && !tc.sibling
			token := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse: echo.ParseComplete, ProvisionPlan: echo.PlanComplete,
				ProvisionApply: echo.ApplyComplete, ProvisionGraph: echo.ProvisionGraphWithAgent(token),
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

			var workspace codersdk.Workspace
			var workspaceID *uuid.UUID
			if tc.tool != "create_workspace" {
				workspace = coderdtest.CreateWorkspace(t, client, template.ID)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
				workspaceID = &workspace.ID
				if tc.stopped {
					workspace = coderdtest.MustTransitionWorkspace(t, client, workspace.ID, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop)
				}
			}
			toolArgs := "{}"
			if tc.tool == "create_workspace" {
				toolArgs = fmt.Sprintf(`{"template_id":%q,"name":%q}`, template.ID, "mcp-"+uuid.NewString()[:8])
			}
			usableServerName := "fixture"
			if tc.sibling {
				usableServerName = "sibling"
			}
			qualifiedName := usableServerName + "__" + usable.toolName
			find := chattest.OpenAIToolCallChunk("find_tools", fmt.Sprintf(`{"queries":[%q]}`, usable.toolName))
			invoke := chattest.OpenAIToolCallChunk(qualifiedName, `{"input":"first-turn"}`)
			var calls atomic.Int32
			openAI := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("MCP discovery")
				}
				step := calls.Add(1)
				if unusable && (tc.tool == "" || step > 1) {
					return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Proceeded without MCP tools.")...)
				}
				if step == 1 && tc.tool != "" {
					chunks := []chattest.OpenAIChunk{chattest.OpenAIToolCallChunk(tc.tool, toolArgs)}
					if tc.sameBatch {
						chunks = append(chunks, find)
						for i := range chunks {
							chunks[i].Choices[0].ToolCalls[0].Index = i
						}
					}
					return chattest.OpenAIStreamingResponse(chunks...)
				}
				if tc.tool != "" {
					step--
				}
				if tc.sameBatch {
					step++
				}
				switch step {
				case 1:
					names := make([]string, 0, len(req.Tools))
					for _, tool := range req.Tools {
						names = append(names, openAIToolName(tool))
					}
					assert.Contains(t, names, "find_tools")
					assert.NotContains(t, names, qualifiedName, "MCP schema must be deferred before discovery")
					return chattest.OpenAIStreamingResponse(find)
				case 2:
					for _, tool := range req.Tools {
						if openAIToolName(tool) == qualifiedName {
							return chattest.OpenAIStreamingResponse(invoke)
						}
					}
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Done.")...)
			})
			model := coderdtest.CreateOpenAICompatChatModel(t, exp, openAI)
			var chatID uuid.UUID
			var agentID uuid.UUID
			if tc.pinned {
				ws, err := client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				agentID = ws.LatestBuild.Resources[0].Agents[0].ID
				// Bind early so publication pins the chat before its first user turn.
				chatID = dbgen.Chat(t, rawDB, database.Chat{
					OrganizationID: user.OrganizationID, OwnerID: user.UserID,
					WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
					AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
					LastModelConfigID: model.ID, Status: database.ChatStatusWaiting,
				}).ID
			}
			startAgent := func(c *controllableMCPServer) agent.Agent {
				t.Helper()
				if tc.unavailable == "empty" {
					return agenttest.New(t, client.URL, token)
				}
				servers := map[string]any{"fixture": map[string]any{"url": c.url}}
				if sibling != nil {
					servers["sibling"] = map[string]any{"url": sibling.url}
				}
				config, err := json.Marshal(map[string]any{"mcpServers": servers})
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), ".mcp.json")
				require.NoError(t, os.WriteFile(path, config, 0o600))
				return agenttest.New(t, client.URL, token, func(o *agent.Options) {
					o.ContextConfig = agentcontextconfig.Config{MCPConfigFiles: path}
				})
			}
			var old *controllableMCPServer
			if tc.restart {
				old = newControllableMCPServer(t, "old-echo")
				old.release()
				a := startAgent(old)
				coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
				require.Eventually(t, func() bool {
					snap, err := rawDB.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
					return err == nil && snap.McpDiscoveryPhase == database.WorkspaceAgentMcpDiscoveryPhaseComplete
				}, testutil.WaitLong, testutil.IntervalFast)
				require.NoError(t, a.Close())
			}
			if tc.missing || tc.restart {
				barrier.pushGate = make(chan struct{})
			}
			release := sync.OnceFunc(func() {
				ctrl.release()
				if barrier.pushGate != nil {
					close(barrier.pushGate)
				}
			})
			t.Cleanup(release)
			barrier.release = release
			if tc.settled {
				release()
			}
			_ = startAgent(ctrl)
			if tc.autostart {
				p, err := coderdtest.GetProvisionerForTags(rawDB, time.Now(), user.OrganizationID, map[string]string{})
				require.NoError(t, err)
				tick := coderdtest.NextAutostartTick(t, workspace)
				coderdtest.UpdateProvisionerLastSeenAt(t, rawDB, p.ID, tick)
				testutil.RequireSend(ctx, t, tickCh, tick)
				stats := testutil.RequireReceive(ctx, t, statsCh)
				require.Empty(t, stats.Errors)
				require.Equal(t, database.WorkspaceTransitionStart, stats.Transitions[workspace.ID])
				workspace = coderdtest.MustWorkspace(t, client, workspace.ID)
				require.Equal(t, codersdk.BuildReasonAutostart, workspace.LatestBuild.Reason)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			}
			if (!tc.stopped || tc.autostart) && tc.tool != "create_workspace" {
				coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
				if tc.unavailable != "empty" {
					testutil.TryReceive(ctx, t, ctrl.arrived)
				}
				ws, err := client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				currentID := ws.LatestBuild.Resources[0].Agents[0].ID
				if tc.pinned {
					require.Equal(t, agentID, currentID, "reconnect must retain the agent row")
				}
				agentID = currentID
				if tc.missing || tc.restart {
					testutil.TryReceive(ctx, t, barrier.pushArrived)
					snap, err := rawDB.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
					if tc.missing {
						require.ErrorIs(t, err, sql.ErrNoRows)
					} else {
						require.NoError(t, err)
						current, err := rawDB.GetWorkspaceAgentByID(ctx, agentID)
						require.NoError(t, err)
						require.NotEmpty(t, current.AgentRunID, "the new process must have reported its run id")
						require.NotEmpty(t, snap.AgentRunID)
						require.NotEqual(t, current.AgentRunID, snap.AgentRunID, "previous process snapshot must be stale")
					}
				} else {
					wantPhase := database.WorkspaceAgentMcpDiscoveryPhasePending
					if tc.settled {
						wantPhase = database.WorkspaceAgentMcpDiscoveryPhaseComplete
					}
					require.Eventually(t, func() bool {
						snap, err := rawDB.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
						return err == nil && snap.McpDiscoveryPhase == wantPhase && snap.AgentRunID != ""
					}, testutil.WaitLong, testutil.IntervalFast)
				}
				if tc.pinned {
					require.Eventually(t, func() bool {
						chat, err := rawDB.GetChatByID(ctx, chatID)
						return err == nil && chat.ContextAggregateHash != nil
					}, testutil.WaitLong, testutil.IntervalFast)
				}
			}
			barrier.Lock()
			barrier.armed = !tc.settled && tc.unavailable == ""
			barrier.Unlock()
			input := []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Use the workspace MCP echo tool."}}
			if tc.restart {
				// Before the new process publishes, the GET projection
				// must flag the pinned rows as stale.
				got, err := exp.GetChat(ctx, chatID)
				require.NoError(t, err)
				require.NotNil(t, got.Context)
				require.NotNil(t, got.Context.MCPDiscovery)
				require.True(t, got.Context.MCPDiscovery.Stale, "a previous process snapshot must project as stale")
			}
			started := time.Now()
			if chatID != uuid.Nil {
				_, err := client.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{Content: input})
				require.NoError(t, err)
			} else {
				var planMode codersdk.ChatPlanMode
				if tc.plan {
					planMode = codersdk.ChatPlanModePlan
				}
				chat, err := exp.CreateChat(ctx, codersdk.CreateChatRequest{OrganizationID: user.OrganizationID, WorkspaceID: workspaceID, Content: input, PlanMode: planMode})
				require.NoError(t, err)
				chatID = chat.ID
			}
			chat := waitForChatStatus(ctx, t, rawDB, chatID, database.ChatStatusWaiting)
			require.Equal(t, database.ChatStatusWaiting, chat.Status, "last_error=%v", chat.LastError)
			if tc.sibling {
				require.Less(t, time.Since(started), 30*time.Second, "the healthy sibling must be usable before the hung connect times out")
			}
			if unusable {
				wantCalls := int32(1)
				if tc.tool != "" {
					wantCalls++
				}
				require.Equal(t, wantCalls, calls.Load(), "generation must proceed even without usable MCP tools")
				require.False(t, ctrl.invoked.Load())
				if tc.plan {
					messages, err := exp.GetChatMessages(ctx, chatID, nil)
					require.NoError(t, err)
					var found bool
					for _, msg := range messages.Messages {
						for _, part := range msg.Content {
							if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == tc.tool {
								var result map[string]any
								require.NoError(t, json.Unmarshal(part.Result, &result))
								require.NotContains(t, result, "mcp_discovery", "plan-mode lifecycle tools must not wait for discovery")
								found = true
							}
						}
					}
					require.True(t, found, "lifecycle tool must execute")
					return
				}
				if tc.unavailable == "hung" {
					require.Less(t, time.Since(started), 30*time.Second, "lifecycle tools and preparation must share one initialization budget")
					nextCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
					defer cancel()
					_, err := client.CreateChatMessage(nextCtx, chatID, codersdk.CreateChatMessageRequest{Content: input})
					require.NoError(t, err)
					waitForChatStatus(nextCtx, t, rawDB, chatID, database.ChatStatusWaiting)
					require.Equal(t, wantCalls+1, calls.Load(), "a later user turn must not spend another initialization budget")
				}
				return
			}
			messages, err := exp.GetChatMessages(ctx, chatID, nil)
			require.NoError(t, err)
			var discovered, invoked, lifecycleReported bool
			for _, msg := range messages.Messages {
				for _, part := range msg.Content {
					if part.Type != codersdk.ChatMessagePartTypeToolResult {
						continue
					}
					if part.ToolName == tc.tool {
						var result struct {
							MCPDiscovery *struct {
								Phase string `json:"phase"`
							} `json:"mcp_discovery"`
						}
						require.NoError(t, json.Unmarshal(part.Result, &result), "%s", part.Result)
						if result.MCPDiscovery != nil {
							lifecycleReported = true
							assert.Equal(t, "complete", result.MCPDiscovery.Phase, "%s", part.Result)
						}
					}
					if part.ToolName == "find_tools" {
						var result struct {
							Activated []string `json:"activated"`
						}
						require.NoError(t, json.Unmarshal(part.Result, &result), "%s", part.Result)
						assert.Contains(t, result.Activated, qualifiedName, "first find_tools must persist successful activation: %s", part.Result)
						discovered = true
					}
					if part.ToolName == qualifiedName {
						assert.Contains(t, string(part.Result), usable.nonce)
						assert.Contains(t, string(part.Result), "first-turn")
						invoked = true
					}
					if old != nil {
						assert.NotContains(t, string(part.Result), old.nonce)
					}
				}
			}
			assert.True(t, discovered, "expected persisted find_tools activation result")
			assert.True(t, invoked, "expected persisted MCP invocation result")
			if tc.tool != "" {
				assert.True(t, lifecycleReported, "lifecycle tool result must report the discovery outcome")
			}
			assert.True(t, usable.invoked.Load(), "real MCP handler must be invoked")
			if tc.sibling {
				assert.False(t, ctrl.invoked.Load(), "the hung server must not be invoked")
			}
			if old != nil {
				assert.False(t, old.invoked.Load(), "stale MCP handler must not be invoked")
			}
			if tc.settled {
				assert.Less(t, time.Since(started), 10*time.Second, "idempotent start must accept the existing snapshot")
				ws, err := client.Workspace(ctx, workspace.ID)
				require.NoError(t, err)
				require.Equal(t, workspace.LatestBuild.ID, ws.LatestBuild.ID, "idempotent start must not rebuild")
			}
		})
	}
}
