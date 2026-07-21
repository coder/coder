package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestPostChatsInitialPromptHookErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		response    string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "deny",
			statusCode:  http.StatusOK,
			response:    `{"permission":{"decision":"deny"},"user_message":"blocked by policy"}`,
			wantStatus:  http.StatusForbidden,
			wantMessage: "blocked by policy",
		},
		{
			name:       "dispatch failure",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusBadGateway,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requests := make(chan agenthooks.Request, 2)
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request agenthooks.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				requests <- request
				w.WriteHeader(test.statusCode)
				if test.response != "" {
					_, err := w.Write([]byte(test.response))
					require.NoError(t, err)
				}
			}))
			t.Cleanup(consumer.Close)

			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.ChatWorkerDisabled = true
				require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
				opts.DeploymentValues.AI.Chat.HookSecret = serpent.String("test-hook-secret-32-bytes-minimum!!")
				opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
				opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
			})
			user := coderdtest.CreateFirstUser(t, client.Client)
			model := createAdditionalChatModelConfig(t, client, "openai", "gpt-4.1")
			ctx := testutil.Context(t, testutil.WaitLong)

			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				ModelConfigID:  &model.ID,
				Content: []codersdk.ChatInputPart{{
					Type: codersdk.ChatInputPartTypeText,
					Text: "blocked prompt",
				}},
			})
			sdkErr := coderdtest.SDKError(t, err)
			require.Equal(t, test.wantStatus, sdkErr.StatusCode())
			if test.wantMessage != "" {
				require.Equal(t, test.wantMessage, sdkErr.Message)
			}
			request := testutil.RequireReceive(ctx, t, requests)
			require.Equal(t, agenthooks.EventUserPromptSubmit, request.Type)
			require.NotEqual(t, uuid.Nil, request.Meta.ChatID)
			_, err = db.GetChatByID(dbauthz.AsSystemRestricted(ctx), request.Meta.ChatID)
			require.ErrorIs(t, err, sql.ErrNoRows)
		})
	}
}

func TestChatLifecycleHooksExperimentDisabled(t *testing.T) {
	t.Parallel()

	var hookRequests atomic.Int32
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hookRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(consumer.Close)

	client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		opts.ChatWorkerDisabled = true
		// Valid hook configuration without the agent-lifecycle-hooks
		// experiment: hooks must stay inert.
		opts.DeploymentValues.Experiments = serpent.StringArray{
			string(codersdk.ExperimentChatAdvisor),
			string(codersdk.ExperimentChatVirtualDesktop),
		}
		require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
		opts.DeploymentValues.AI.Chat.HookSecret = serpent.String("test-hook-secret-32-bytes-minimum!!")
		opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
		opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	model := createAdditionalChatModelConfig(t, client, "openai", "gpt-4.1")
	ctx := testutil.Context(t, testutil.WaitLong)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "prompt with hooks disabled",
		}},
	})
	require.NoError(t, err)

	require.Zero(t, hookRequests.Load())
	rows, err := db.ListChatHookDispatchesByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestChatLifecycleHooksWorkedExample(t *testing.T) {
	t.Parallel()

	const (
		secret            = "test-hook-secret-32-bytes-minimum!!"
		deniedToolCallID  = "call_denied"
		allowedToolCallID = "call_allowed"
	)
	ctx := testutil.Context(t, testutil.WaitLong)
	var modelCalls atomic.Int32
	secondModelRequest := make(chan []byte, 1)
	thirdModelRequest := make(chan []byte, 1)
	modelURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Lifecycle hooks")
		}
		switch modelCalls.Add(1) {
		case 1:
			chunk := chattest.OpenAIToolCallChunk("read_secret", `{"path":"/tmp/secret"}`)
			chunk.Choices[0].ToolCalls[0].ID = deniedToolCallID
			return chattest.OpenAIStreamingResponse(chunk)
		case 2:
			secondModelRequest <- bytes.Clone(req.RawBody)
			chunk := chattest.OpenAIToolCallChunk("search_docs", `{"query":"customer secret"}`)
			chunk.Choices[0].ToolCalls[0].ID = allowedToolCallID
			return chattest.OpenAIStreamingResponse(chunk)
		case 3:
			thirdModelRequest <- bytes.Clone(req.RawBody)
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		default:
			return chattest.OpenAIErrorResponse(http.StatusInternalServerError, "unexpected_call", "unexpected model call")
		}
	})

	hookEvents := make(chan agenthooks.EventType, 16)
	recordHook := func(event agenthooks.EventType) {
		hookEvents <- event
	}
	consumer, setHooks := newHookConsumer(t, secret)
	setHooks(agenthooks.Hooks{
		SessionStart: func(context.Context, agenthooks.Meta, agenthooks.SessionStartData) (agenthooks.Response, error) {
			recordHook(agenthooks.EventSessionStart)
			return agenthooks.Response{}, nil
		},
		UserPromptSubmit: func(context.Context, agenthooks.Meta, agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			recordHook(agenthooks.EventUserPromptSubmit)
			return agenthooks.Response{}, nil
		},
		PreToolUse: func(_ context.Context, _ agenthooks.Meta, tool agenthooks.PreToolUseData) (agenthooks.Response, error) {
			recordHook(agenthooks.EventPreToolUse)
			switch tool.ToolUseID {
			case deniedToolCallID:
				return agenthooks.Response{Permission: &agenthooks.Permission{
					Decision: agenthooks.PermissionDeny,
					Reason:   "secret reads are blocked",
				}}, nil
			case allowedToolCallID:
				return agenthooks.Response{Permission: &agenthooks.Permission{
					Decision:      agenthooks.PermissionAllow,
					InputOverride: json.RawMessage(`{"query":"public documentation"}`),
				}}, nil
			default:
				return agenthooks.Response{}, nil
			}
		},
		PostToolUse: func(context.Context, agenthooks.Meta, agenthooks.PostToolUseData) (agenthooks.Response, error) {
			recordHook(agenthooks.EventPostToolUse)
			return agenthooks.Response{
				ModelContext: "The approved search result is safe to use.",
				UserMessage:  "Search result approved by policy.",
			}, nil
		},
		Stop: func(context.Context, agenthooks.Meta, agenthooks.StopData) (agenthooks.Response, error) {
			recordHook(agenthooks.EventStop)
			return agenthooks.Response{}, nil
		},
	})
	t.Cleanup(consumer.Close)

	client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
		opts.DeploymentValues.AI.Chat.HookSecret = serpent.String(secret)
		opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
		opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	model := createChatModelConfigWithBaseURL(t, client, modelURL)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "Find the deployment documentation.",
		}},
		UnsafeDynamicTools: []codersdk.DynamicTool{
			{
				Name:        "read_secret",
				Description: "Read a secret file.",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
			{
				Name:        "search_docs",
				Description: "Search public documentation.",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	})
	require.NoError(t, err)

	var stored database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		stored, err = db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		return err == nil && stored.Status == database.ChatStatusRequiresAction
	}, testutil.IntervalFast)
	require.Equal(t, int32(2), modelCalls.Load())
	require.Contains(t, string(testutil.RequireReceive(ctx, t, secondModelRequest)), "DENIED: secret reads are blocked")

	messages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var allowedCall *codersdk.ChatMessagePart
	for _, message := range messages.Messages {
		for i := range message.Content {
			part := &message.Content[i]
			if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolCallID == allowedToolCallID {
				allowedCall = part
			}
		}
	}
	require.NotNil(t, allowedCall)
	require.JSONEq(t, `{"query":"public documentation"}`, string(allowedCall.Args))

	err = client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
		Results: []codersdk.ToolResult{{
			ToolCallID: allowedToolCallID,
			Output:     json.RawMessage(`{"matches":["agent hooks"]}`),
		}},
	})
	require.NoError(t, err)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		stored, err = db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		return err == nil && stored.Status == database.ChatStatusWaiting
	}, testutil.IntervalFast)
	require.Contains(t, string(testutil.RequireReceive(ctx, t, thirdModelRequest)), "The approved search result is safe to use.")
	require.Equal(t, int32(3), modelCalls.Load())

	rows, err := db.ListChatHookDispatchesByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 6)
	assertDispatch := func(event agenthooks.EventType, toolUseID, result, decision string) database.ChatHookDispatch {
		t.Helper()
		for _, row := range rows {
			if row.Event != string(event) || row.ToolUseID.String != toolUseID {
				continue
			}
			require.Equal(t, result, row.Result)
			require.True(t, row.FinishedAt.Valid)
			require.Equal(t, int32(http.StatusOK), row.HttpStatus.Int32)
			require.Equal(t, decision != "", row.Decision.Valid)
			if decision != "" {
				require.Equal(t, decision, row.Decision.String)
			}
			return row
		}
		require.FailNow(t, "hook dispatch not found", "event=%s tool_use_id=%s", event, toolUseID)
		return database.ChatHookDispatch{}
	}
	assertDispatch(agenthooks.EventUserPromptSubmit, "", "ok", "")
	assertDispatch(agenthooks.EventSessionStart, "", "ok", "")
	denied := assertDispatch(agenthooks.EventPreToolUse, deniedToolCallID, "denied", "deny")
	require.Equal(t, "secret reads are blocked", denied.DecisionReason.String)
	allowed := assertDispatch(agenthooks.EventPreToolUse, allowedToolCallID, "ok", "allow")
	require.JSONEq(t, `{"query":"public documentation"}`, string(allowed.InputOverride.RawMessage))
	post := assertDispatch(agenthooks.EventPostToolUse, allowedToolCallID, "ok", "")
	require.Equal(t, "The approved search result is safe to use.", post.ModelContext.String)
	assertDispatch(agenthooks.EventStop, "", "ok", "")

	seenEvents := make([]agenthooks.EventType, 0, len(rows))
	for range rows {
		seenEvents = append(seenEvents, testutil.RequireReceive(ctx, t, hookEvents))
	}
	require.ElementsMatch(t, []agenthooks.EventType{
		agenthooks.EventUserPromptSubmit,
		agenthooks.EventSessionStart,
		agenthooks.EventPreToolUse,
		agenthooks.EventPreToolUse,
		agenthooks.EventPostToolUse,
		agenthooks.EventStop,
	}, seenEvents)
}

func TestChatHooksFileLinksAfterPromptOverride(t *testing.T) {
	t.Parallel()

	const secret = "test-hook-secret-32-bytes-minimum!!"
	ctx := testutil.Context(t, testutil.WaitLong)
	modelURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	consumer, setHooks := newHookConsumer(t, secret)
	setHooks(agenthooks.Hooks{
		UserPromptSubmit: func(_ context.Context, _ agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			if strings.Contains(data.Prompt, "REDACTME") {
				return agenthooks.Response{Permission: &agenthooks.Permission{
					Decision:      agenthooks.PermissionAllow,
					InputOverride: json.RawMessage(`{"prompt":"redacted"}`),
				}}, nil
			}
			return agenthooks.Response{}, nil
		},
	})
	t.Cleanup(consumer.Close)

	client, api := newChatClientWithAPI(t, func(opts *coderdtest.Options) {
		require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
		opts.DeploymentValues.AI.Chat.HookSecret = serpent.String(secret)
		opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
		opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	model := createChatModelConfigWithBaseURL(t, client, modelURL)

	uploadFile := func(name string) uuid.UUID {
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 16)...)
		resp, err := client.UploadChatFile(ctx, user.OrganizationID, "image/png", name, bytes.NewReader(pngData))
		require.NoError(t, err)
		return resp.ID
	}

	redactedFile := uploadFile("redacted.png")
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "REDACTME create"},
			{Type: codersdk.ChatInputPartTypeFile, FileID: redactedFile},
		},
	})
	require.NoError(t, err)
	created, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, created.Files, "overridden create must not link dropped attachments")

	coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)

	keptFile := uploadFile("kept.png")
	sendResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "keep this"},
			{Type: codersdk.ChatInputPartTypeFile, FileID: keptFile},
		},
	})
	require.NoError(t, err)
	require.False(t, sendResp.Queued)
	afterSend, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, afterSend.Files, 1)
	require.Equal(t, keptFile, afterSend.Files[0].ID)

	coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)

	droppedFile := uploadFile("dropped.png")
	_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "REDACTME send"},
			{Type: codersdk.ChatInputPartTypeFile, FileID: droppedFile},
		},
	})
	require.NoError(t, err)
	afterOverride, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, afterOverride.Files, 1, "overridden send must not link dropped attachments")
	require.Equal(t, keptFile, afterOverride.Files[0].ID)
}

// newHookConsumer starts a hook consumer whose expected JWT audience is its
// own base URL, matching the HookURL the test sets on the deployment. Hooks
// are installed after the server starts because the audience must be known
// at handler construction time.
func newHookConsumer(t *testing.T, secret string) (*httptest.Server, func(agenthooks.Hooks)) {
	t.Helper()

	var handler http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(rw, r)
	}))
	t.Cleanup(server.Close)
	return server, func(hooks agenthooks.Hooks) {
		built, err := agenthooks.NewHTTPHandler([]byte(secret), server.URL, hooks)
		require.NoError(t, err)
		handler = built
	}
}
