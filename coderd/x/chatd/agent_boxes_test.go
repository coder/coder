package chatd_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
	for _, name := range chattool.BoxToolNames {
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
	require.Eventually(t, func() bool {
		roots, err := os.ReadDir(boxRoot)
		if err != nil {
			return false
		}
		for _, root := range roots {
			entries, err := os.ReadDir(boxRoot + "/" + root.Name())
			if err != nil {
				return false
			}
			if slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.IsDir() }) {
				return false
			}
		}
		return true
	}, testutil.WaitShort, testutil.IntervalFast)
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

	newChat := func(t *testing.T, experimentOn bool, opts func(*chatd.CreateOptions)) []string {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var tools atomic.Pointer[[]string]
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			names := anthropicToolNames(req)
			tools.Store(&names)
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
		if opts != nil {
			opts(&create)
		}
		chat, err := server.CreateChat(ctx, create)
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		got := tools.Load()
		require.NotNil(t, got)
		return *got
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
		names := newChat(t, true, func(o *chatd.CreateOptions) {
			o.PlanMode = database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
		})
		for _, name := range chattool.BoxToolNames {
			require.Contains(t, names, name)
		}
	})

	t.Run("ExploreSubagent", func(t *testing.T) {
		t.Parallel()
		names := newChat(t, true, func(o *chatd.CreateOptions) {
			o.ChatMode = database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true}
		})
		require.False(t, hasBoxTools(names))
	})
}
