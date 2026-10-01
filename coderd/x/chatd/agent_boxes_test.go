package chatd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func anthropicToolNames(req *chattest.AnthropicRequest) []string {
	names := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// A chat without a workspace runs a script, reads a file it wrote in a
// later step, attaches it, and loses the box once the turn ends.
func TestAgentBoxTurn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	var firstTools atomic.Pointer[[]string]
	var systemPrompt atomic.Pointer[string]
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			names := anthropicToolNames(req)
			firstTools.Store(&names)
			system := string(req.System)
			systemPrompt.Store(&system)
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				chattool.BoxRunToolName,
				`{"language":"javascript","code":"const f = std.open('/box/out.txt', 'w'); f.puts('line one\\nline two\\n'); f.close(); console.log('hi');"}`,
			)...)
		case 2:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				chattool.BoxReadFileToolName,
				`{"path":"/box/out.txt"}`,
			)...)
		case 3:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				chattool.BoxAttachFileToolName,
				`{"path":"/box/out.txt","name":"result.txt"}`,
			)...)
		default:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)

	boxRoot := t.TempDir()
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.AgentBoxRootDir = boxRoot
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "agent-box-turn",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("compute something"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(4), modelCalls.Load())

	tools := firstTools.Load()
	require.NotNil(t, tools)
	for _, name := range chattool.BoxToolNames() {
		require.Contains(t, *tools, name)
	}
	require.Contains(t, *systemPrompt.Load(), "agent-box", "the prompt block is present (JSON escapes the angle brackets)")

	parts := chatToolParts(ctx, t, db, chat.ID)

	run := requireToolResultPart(t, parts, chattool.BoxRunToolName)
	require.False(t, run.IsError, string(run.Result))
	var runResult struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		BoxID    string `json:"box_id"`
		Reset    bool   `json:"box_reset"`
	}
	require.NoError(t, json.Unmarshal(run.Result, &runResult))
	require.Equal(t, 0, runResult.ExitCode)
	require.Equal(t, "hi\n", runResult.Stdout)
	require.NotEmpty(t, runResult.BoxID)
	require.False(t, runResult.Reset)

	read := requireToolResultPart(t, parts, chattool.BoxReadFileToolName)
	require.False(t, read.IsError, string(read.Result))
	var readResult struct {
		Content string `json:"content"`
		BoxID   string `json:"box_id"`
		Reset   bool   `json:"box_reset"`
	}
	require.NoError(t, json.Unmarshal(read.Result, &readResult))
	require.Equal(t, "1\tline one\n2\tline two\n3\t", readResult.Content)
	require.Equal(t, runResult.BoxID, readResult.BoxID, "the same box serves every step of the turn")
	require.False(t, readResult.Reset)

	attach := requireToolResultPart(t, parts, chattool.BoxAttachFileToolName)
	require.False(t, attach.IsError, string(attach.Result))
	var attachResult struct {
		FileID string `json:"file_id"`
		Name   string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(attach.Result, &attachResult))
	require.Equal(t, "result.txt", attachResult.Name)

	var filePart *codersdk.ChatMessagePart
	for _, msg := range chatMessages(ctx, t, db, chat.ID) {
		parsed, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parsed {
			if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid && part.FileID.UUID.String() == attachResult.FileID {
				filePart = &part
			}
		}
	}
	require.NotNil(t, filePart, "the attachment must be promoted to a file part without a workspace")
	require.Equal(t, "result.txt", filePart.Name)
	file, err := db.GetChatFileByID(ctx, filePart.FileID.UUID)
	require.NoError(t, err)
	require.Equal(t, org.ID, file.OrganizationID)
	require.Equal(t, "line one\nline two\n", string(file.Data))

	// The turn's box directory is removed once the chat is waiting.
	testutil.Eventually(ctx, t, func(context.Context) bool {
		roots, err := os.ReadDir(boxRoot)
		if err != nil {
			return false
		}
		for _, root := range roots {
			entries, err := os.ReadDir(filepath.Join(boxRoot, root.Name()))
			if err != nil {
				return false
			}
			if slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.IsDir() }) {
				return false
			}
		}
		return true
	}, testutil.IntervalFast)
}

// The box survives a requires_action wait: the step after the caller
// submits dynamic tool results sees the file written before the pause, in
// the same box.
func TestAgentBoxSurvivesRequiresAction(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				chattool.BoxWriteFileToolName,
				`{"path":"/box/kept.txt","content":"kept"}`,
			)...)
		case 2:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				"client_tool",
				`{"input":"x"}`,
			)...)
		case 3:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
				chattool.BoxRunToolName,
				`{"language":"javascript","code":"console.log(std.loadFile('/box/kept.txt'))"}`,
			)...)
		default:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})

	dynamicTools, err := json.Marshal([]mcp.Tool{{
		Name:        "client_tool",
		Description: "A client-executed tool.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"input": map[string]any{"type": "string"}},
		},
	}})
	require.NoError(t, err)
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "agent-box-requires-action",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("write, ask, then read"),
		},
		DynamicTools: dynamicTools,
	})
	require.NoError(t, err)

	// The runner keeps ownership while it waits for the caller, so the
	// chat stays bound to a worker in requires_action.
	var paused database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		c, err := db.GetChatByID(ctx, chat.ID)
		paused = c
		return err == nil && c.Status == database.ChatStatusRequiresAction
	}, testutil.IntervalFast)
	require.True(t, paused.RunnerID.Valid, "the runner must keep the chat during requires_action")
	call := requireToolCallPart(t, chatToolParts(ctx, t, db, chat.ID), "client_tool")
	require.NoError(t, server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: paused.LastModelConfigID,
		Results: []codersdk.ToolResult{{
			ToolCallID: call.ToolCallID,
			Output:     json.RawMessage(`{"ok":true}`),
		}},
	}))
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Equal(t, int32(4), modelCalls.Load())

	parts := chatToolParts(ctx, t, db, chat.ID)
	var write, run struct {
		BoxID  string `json:"box_id"`
		Reset  bool   `json:"box_reset"`
		Stdout string `json:"stdout"`
	}
	require.NoError(t, json.Unmarshal(requireToolResultPart(t, parts, chattool.BoxWriteFileToolName).Result, &write))
	require.NoError(t, json.Unmarshal(requireToolResultPart(t, parts, chattool.BoxRunToolName).Result, &run))
	require.NotEmpty(t, write.BoxID)
	require.Equal(t, write.BoxID, run.BoxID)
	require.False(t, run.Reset)
	// Box IDs are random per box and a new runner starts with an empty
	// tracker, so an equal ID means the same runner served both steps.
	require.Equal(t, "kept\n", run.Stdout)
}

// The box tools follow the experiment and mode gates.
func TestAgentBoxToolGating(t *testing.T) {
	t.Parallel()

	type turn struct {
		tools  []string
		system string
	}
	// startChat creates a chat and returns the tools and system prompt of
	// its first model call. With withParent, the chat is a child of a
	// root chat created first.
	startChat := func(t *testing.T, experimentOn, withParent bool, opts func(*chatd.CreateOptions)) turn {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var last atomic.Pointer[turn]
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			last.Store(&turn{tools: anthropicToolNames(req), system: string(req.System)})
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("ok")...)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			if !experimentOn {
				cfg.Experiments = slices.DeleteFunc(
					slices.Clone(codersdk.ExperimentsKnown),
					func(e codersdk.Experiment) bool { return e == codersdk.ExperimentAgentBoxes },
				)
			}
		})
		create := chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "agent-box-gating",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("hello"),
			},
		}
		if withParent {
			parent, err := server.CreateChat(ctx, create)
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, parent.ID, database.ChatStatusWaiting)
			last.Store(nil)
			create.ParentChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
			create.RootChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
		}
		if opts != nil {
			opts(&create)
		}
		chat, err := server.CreateChat(ctx, create)
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		got := last.Load()
		require.NotNil(t, got)
		return *got
	}
	newChat := func(t *testing.T, experimentOn bool, opts func(*chatd.CreateOptions)) []string {
		t.Helper()
		return startChat(t, experimentOn, false, opts).tools
	}
	planMode := func(o *chatd.CreateOptions) {
		o.PlanMode = database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
	}

	hasBoxTools := func(names []string) bool {
		return slices.ContainsFunc(names, func(name string) bool { return strings.HasPrefix(name, "box_") })
	}

	t.Run("ExperimentOff", func(t *testing.T) {
		t.Parallel()
		require.False(t, hasBoxTools(newChat(t, false, nil)))
	})

	t.Run("PlanMode", func(t *testing.T) {
		t.Parallel()
		got := startChat(t, true, false, planMode)
		for _, name := range chattool.BoxToolNames() {
			require.Contains(t, got.tools, name)
		}
		require.Contains(t, got.system, chattool.BoxAttachFileToolName)
	})

	t.Run("ChildPlanMode", func(t *testing.T) {
		t.Parallel()
		got := startChat(t, true, true, planMode)
		require.Contains(t, got.tools, chattool.BoxRunToolName)
		require.NotContains(t, got.tools, chattool.BoxAttachFileToolName)
		require.Contains(t, got.system, chattool.BoxRunToolName)
		require.NotContains(t, got.system, chattool.BoxAttachFileToolName, "the prompt matches the offered tools")
	})

	t.Run("ExploreSubagent", func(t *testing.T) {
		t.Parallel()
		names := newChat(t, true, func(o *chatd.CreateOptions) {
			o.ChatMode = database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true}
		})
		require.False(t, hasBoxTools(names))
	})
}

// A box script calls an org MCP tool twice through mcp.call and the
// box_run result records both calls. In plan mode a server without
// allow_in_plan_mode is neither advertised nor callable from the box.
func TestAgentBoxMCP(t *testing.T) {
	t.Parallel()

	const code = `const names = mcp.tools().map((t) => t.name).join(",");
let out = [];
for (let i = 0; i < 2; i++) { out.push(mcp.text(mcp.call("paged__echo", {input: "p" + i}))); }
let hidden = "";
try { mcp.call("hidden__echo", {input: "x"}); hidden = "called"; } catch (e) { hidden = e.code; }
console.log(names, out.join("|"), hidden);`

	type outcome struct {
		system string
		run    map[string]any
	}
	runTurn := func(t *testing.T, planMode bool) outcome {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var modelCalls atomic.Int32
		var systemPrompt atomic.Pointer[string]
		codeJSON, err := json.Marshal(code)
		require.NoError(t, err)
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			if modelCalls.Add(1) == 1 {
				system := string(req.System)
				systemPrompt.Store(&system)
				return chattest.AnthropicStreamingResponse(chattest.AnthropicToolCallChunks(
					chattool.BoxRunToolName,
					`{"language":"javascript","code":`+string(codeJSON)+`}`,
				)...)
			}
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		pagedURL := newEchoMCPTestServer(t, "paged")
		hiddenURL := newEchoMCPTestServer(t, "hidden")
		paged := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID:  org.ID,
			DisplayName:     "Paged",
			Slug:            "paged",
			Url:             pagedURL,
			AllowInPlanMode: true,
			CreatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
			UpdatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		hidden := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: org.ID,
			DisplayName:    "Hidden",
			Slug:           "hidden",
			Url:            hiddenURL,
			CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
			UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.AgentBoxRootDir = t.TempDir()
		})
		create := chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "agent-box-mcp",
			ModelConfigID:  model.ID,
			MCPServerIDs:   []uuid.UUID{paged.ID, hidden.ID},
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("page the data"),
			},
		}
		if planMode {
			create.PlanMode = database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
		}
		chat, err := server.CreateChat(ctx, create)
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		run := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), chattool.BoxRunToolName)
		require.False(t, run.IsError, string(run.Result))
		var result map[string]any
		require.NoError(t, json.Unmarshal(run.Result, &result))
		return outcome{system: *systemPrompt.Load(), run: result}
	}

	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		got := runTurn(t, false)
		require.Equal(t, "hidden__echo,paged__echo echo: p0|echo: p1 called\n", got.run["stdout"], got.run["stderr"])
		require.EqualValues(t, 3, got.run["mcp_calls_total"])
		require.EqualValues(t, 0, got.run["mcp_calls_failed"])
		calls, ok := got.run["mcp_calls"].([]any)
		require.True(t, ok)
		require.Len(t, calls, 3)
		require.Contains(t, got.system, "servers: hidden, paged")
		require.Contains(t, got.system, "mcp.call(name, args)")
	})

	t.Run("PlanMode", func(t *testing.T) {
		t.Parallel()
		got := runTurn(t, true)
		require.Equal(t, "paged__echo echo: p0|echo: p1 unknown_tool\n", got.run["stdout"], got.run["stderr"])
		require.EqualValues(t, 3, got.run["mcp_calls_total"])
		require.EqualValues(t, 1, got.run["mcp_calls_failed"])
		require.Contains(t, got.system, "servers: paged)")
		require.NotContains(t, got.system, "hidden")
	})
}
