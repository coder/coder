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
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
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
		wantKind    codersdk.ChatErrorKind
	}{
		{
			name:        "deny",
			statusCode:  http.StatusOK,
			response:    `{"permission":{"decision":"deny"},"user_message":"blocked by policy"}`,
			wantStatus:  http.StatusForbidden,
			wantMessage: "blocked by policy",
			wantKind:    codersdk.ChatErrorKindHookDenied,
		},
		{
			name:       "dispatch failure",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusBadGateway,
			wantKind:   codersdk.ChatErrorKindHookDispatchFailed,
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
			model := createAdditionalChatModel(t, client, "openai", "gpt-4.1")
			ctx := testutil.Context(t, testutil.WaitLong)

			res, err := client.Request(ctx, http.MethodPost, "/api/experimental/chats", codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				ModelConfigID:  &model.ID,
				Content: []codersdk.ChatInputPart{{
					Type: codersdk.ChatInputPartTypeText,
					Text: "blocked prompt",
				}},
			})
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, test.wantStatus, res.StatusCode)
			// Both outcomes share this wire shape, differing only in kind.
			var response struct {
				codersdk.Response
				Kind codersdk.ChatErrorKind `json:"kind"`
			}
			require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
			require.Equal(t, test.wantKind, response.Kind)
			if test.wantMessage != "" {
				require.Equal(t, test.wantMessage, response.Message)
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

	client, _ := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		opts.ChatWorkerDisabled = true
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
	model := createAdditionalChatModel(t, client, "openai", "gpt-4.1")
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "prompt with hooks disabled",
		}},
	})
	require.NoError(t, err)

	require.Zero(t, hookRequests.Load())
}

func TestChatPromptHookContextHiddenFromAPI(t *testing.T) {
	t.Parallel()

	const secret = "test-hook-secret-32-bytes-minimum!!"
	consumer := newHookConsumer(t, secret, agenthooks.Hooks{
		UserPromptSubmit: func(context.Context, agenthooks.Meta, agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			return agenthooks.Response{
				ModelContext: "prompt context",
				UserMessage:  "prompt notice",
			}, nil
		},
	})
	t.Cleanup(consumer.Close)

	client, _ := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
		opts.ChatWorkerDisabled = true
		require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
		opts.DeploymentValues.AI.Chat.HookSecret = serpent.String(secret)
		opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
		opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	model := createAdditionalChatModel(t, client, "openai", "gpt-4.1")
	ctx := testutil.Context(t, testutil.WaitLong)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "initial prompt",
		}},
	})
	require.NoError(t, err)

	messages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	require.Len(t, messages.Messages, 1)
	require.Equal(t, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("initial prompt"),
		{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "prompt notice"},
	}, messages.Messages[0].Content)
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
	consumer := newHookConsumer(t, secret, agenthooks.Hooks{
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
	model := createChatModelWithBaseURL(t, client, modelURL)

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
	require.Contains(t, string(testutil.RequireReceive(ctx, t, secondModelRequest)), "Reason: secret reads are blocked.")

	messages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var allowedCall, deniedCall *codersdk.ChatMessagePart
	for _, message := range messages.Messages {
		for i := range message.Content {
			part := &message.Content[i]
			if part.Type != codersdk.ChatMessagePartTypeToolCall {
				continue
			}
			switch part.ToolCallID {
			case allowedToolCallID:
				allowedCall = part
			case deniedToolCallID:
				deniedCall = part
			}
		}
	}
	require.NotNil(t, allowedCall)
	require.JSONEq(t, `{"query":"public documentation"}`, string(allowedCall.Args))
	require.True(t, allowedCall.HookRewritten)
	require.NotNil(t, deniedCall)
	require.False(t, deniedCall.HookRewritten)

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

	messages, err = client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var foundPostToolNotice bool
	for _, message := range messages.Messages {
		if message.Role != codersdk.ChatMessageRoleSystem {
			continue
		}
		for _, part := range message.Content {
			if part.Type == codersdk.ChatMessagePartTypeHookNotice && part.Text == "Search result approved by policy." {
				foundPostToolNotice = true
			}
		}
	}
	require.True(t, foundPostToolNotice)

	var seenEvents []agenthooks.EventType
	for {
		event := testutil.RequireReceive(ctx, t, hookEvents)
		seenEvents = append(seenEvents, event)
		if event == agenthooks.EventStop {
			break
		}
	}
	require.Contains(t, seenEvents, agenthooks.EventUserPromptSubmit)
	require.Contains(t, seenEvents, agenthooks.EventSessionStart)
	var preToolUseEvents int
	for _, event := range seenEvents {
		if event == agenthooks.EventPreToolUse {
			preToolUseEvents++
		}
	}
	require.GreaterOrEqual(t, preToolUseEvents, 2)
	require.Contains(t, seenEvents, agenthooks.EventPostToolUse)
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
	consumer := newHookConsumer(t, secret, agenthooks.Hooks{
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
	model := createChatModelWithBaseURL(t, client, modelURL)

	uploadFile := func(name string) uuid.UUID {
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 16)...)
		resp, err := client.UploadChatFile(ctx, user.OrganizationID, "image/png", name, bytes.NewReader(pngData))
		require.NoError(t, err)
		return resp.ID
	}

	createFile := uploadFile("create.png")
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "REDACTME create"},
			{Type: codersdk.ChatInputPartTypeFile, FileID: createFile},
		},
	})
	require.NoError(t, err)
	created, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, created.Files, 1, "an overridden create must keep linking its attachments")
	require.Equal(t, createFile, created.Files[0].ID)

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
	require.Len(t, afterSend.Files, 2)
	require.ElementsMatch(t, []uuid.UUID{createFile, keptFile}, []uuid.UUID{afterSend.Files[0].ID, afterSend.Files[1].ID})

	coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)

	overriddenFile := uploadFile("overridden.png")
	sendResp, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeFileReference, FileName: "main.go", StartLine: 1, EndLine: 3, Content: "package main"},
			{Type: codersdk.ChatInputPartTypeText, Text: "REDACTME send"},
			{Type: codersdk.ChatInputPartTypeFile, FileID: overriddenFile},
		},
	})
	require.NoError(t, err)
	require.False(t, sendResp.Queued)
	afterOverride, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, afterOverride.Files, 3, "an overridden send must keep linking its attachments")
	require.ElementsMatch(t, []uuid.UUID{createFile, keptFile, overriddenFile}, []uuid.UUID{
		afterOverride.Files[0].ID,
		afterOverride.Files[1].ID,
		afterOverride.Files[2].ID,
	})

	require.NotNil(t, sendResp.Message)
	require.Equal(t, codersdk.ChatMessageRoleUser, sendResp.Message.Role)
	require.Equal(t, []codersdk.ChatMessagePart{
		codersdk.ChatMessageFileReference("main.go", 1, 3, "package main"),
		codersdk.ChatMessageText("redacted"),
		codersdk.ChatMessageFile(overriddenFile, "image/png", "overridden.png"),
	}, sendResp.Message.Content)
}

func TestChatHookNoticeMessagesInResponses(t *testing.T) {
	t.Parallel()

	const secret = "test-hook-secret-32-bytes-minimum!!"
	ctx := testutil.Context(t, testutil.WaitLong)
	modelURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})

	consumer := newHookConsumer(t, secret, agenthooks.Hooks{
		SessionStart: func(context.Context, agenthooks.Meta, agenthooks.SessionStartData) (agenthooks.Response, error) {
			return agenthooks.Response{UserMessage: "session notice"}, nil
		},
		UserPromptSubmit: func(_ context.Context, _ agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			response := agenthooks.Response{UserMessage: "prompt notice"}
			if data.Prompt == "edited prompt" {
				response.ModelContext = "prompt context"
			}
			return response, nil
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
	model := createChatModelWithBaseURL(t, client, modelURL)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		ModelConfigID:  &model.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "initial prompt",
		}},
	})
	require.NoError(t, err)

	waitForWaiting := func() {
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			stored, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
			return err == nil && stored.Status == database.ChatStatusWaiting
		}, testutil.IntervalFast)
	}
	waitForWaiting()

	assertPromptContent := func(message codersdk.ChatMessage, prompt string) {
		t.Helper()
		require.Equal(t, codersdk.ChatMessageRoleUser, message.Role)
		require.Equal(t, []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(prompt),
			{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "prompt notice"},
		}, message.Content)
	}

	initialMessages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var initialPrompt *codersdk.ChatMessage
	for i := range initialMessages.Messages {
		message := &initialMessages.Messages[i]
		if message.Role == codersdk.ChatMessageRoleUser && len(message.Content) > 0 && message.Content[0].Text == "initial prompt" {
			initialPrompt = message
			break
		}
	}
	require.NotNil(t, initialPrompt)
	assertPromptContent(*initialPrompt, "initial prompt")

	sent, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "second prompt",
		}},
	})
	require.NoError(t, err)
	require.False(t, sent.Queued, "idle chat must insert directly")
	require.NotNil(t, sent.Message)
	require.NotEmpty(t, sent.Messages, "send response must carry the inserted batch")
	last := sent.Messages[len(sent.Messages)-1]
	require.Equal(t, sent.Message.ID, last.ID, "user message must be last in the batch")
	assertPromptContent(last, "second prompt")
	assertPromptContent(*sent.Message, "second prompt")

	waitForWaiting()

	edited, err := client.EditChatMessage(ctx, chat.ID, sent.Message.ID, codersdk.EditChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "edited prompt",
		}},
	})
	require.NoError(t, err)
	require.NotZero(t, edited.Message.ID, "successful edits must return the replacement message")
	require.NotEmpty(t, edited.Messages, "edit response must carry the inserted batch")
	var editedBatchMessage *codersdk.ChatMessage
	for i := range edited.Messages {
		if edited.Messages[i].ID == edited.Message.ID {
			editedBatchMessage = &edited.Messages[i]
			break
		}
	}
	require.NotNil(t, editedBatchMessage)
	assertPromptContent(*editedBatchMessage, "edited prompt")
	assertPromptContent(edited.Message, "edited prompt")

	allMessages, err := client.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)
	var sessionNoticeFound bool
	for _, message := range allMessages.Messages {
		if message.Role != codersdk.ChatMessageRoleSystem {
			continue
		}
		for _, part := range message.Content {
			if part.Type == codersdk.ChatMessagePartTypeHookNotice && part.Text == "session notice" {
				sessionNoticeFound = true
			}
		}
	}
	require.True(t, sessionNoticeFound)
}

// newHookConsumer serves hooks with its own URL as the configured audience,
// which is the value Coder signs when it dispatches there. The listener is
// allocated first because httptest.NewServer builds its handler before the
// server has a URL.
func newHookConsumer(t *testing.T, secret string, hooks agenthooks.Hooks) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = agenthooks.NewHTTPHandler([]byte(secret), "http://"+server.Listener.Addr().String(), hooks)
	server.Start()
	return server
}
