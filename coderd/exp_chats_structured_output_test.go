package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

var structuredOn = []codersdk.Experiment{codersdk.ExperimentChatStructuredOutput}

const structuredTestSchema = `{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`

// structuredOutputFixture is a coderd with a fake OpenAI-compatible model
// whose streaming call number step (from zero) answers with respond.
type structuredOutputFixture struct {
	ctx    context.Context
	client *codersdk.ExperimentalClient
	api    *coderd.API
	org    uuid.UUID
	calls  *atomic.Int32
}

func newStructuredOutputFixture(t *testing.T, experiments []codersdk.Experiment, respond func(step int) chattest.OpenAIResponse) structuredOutputFixture {
	t.Helper()
	var calls atomic.Int32
	baseURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title": "Structured"}`)
		}
		return respond(int(calls.Add(1)) - 1)
	})
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentChatAdvisor)}
	for _, experiment := range experiments {
		values.Experiments = append(values.Experiments, string(experiment))
	}
	opts := newChatTestOptions(t, values)
	client, _, api := coderdtest.NewWithAPI(t, opts)
	exp := codersdk.NewExperimentalClient(client)
	firstUser := coderdtest.CreateFirstUser(t, client)
	_ = createChatModelWithBaseURL(t, exp, baseURL)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	return structuredOutputFixture{ctx: testutil.Context(t, testutil.WaitLong), client: exp, api: api, org: firstUser.OrganizationID, calls: &calls}
}

func structuredFormat() *codersdk.ChatResponseFormat {
	return &codersdk.ChatResponseFormat{
		Type:       codersdk.ChatResponseFormatTypeJSONSchema,
		JSONSchema: &codersdk.ChatResponseFormatJSONSchema{Name: "report", Schema: json.RawMessage(structuredTestSchema)},
	}
}

func structuredText(text string) []codersdk.ChatInputPart {
	return []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}}
}

func finalizerResponse(args string) chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(chatstructured.FinalizerToolName, args))
}

func textResponse() chattest.OpenAIResponse {
	return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
}

// createStructuredChat creates a chat whose initial message sets the format
// and returns it with that message's ID.
func (f structuredOutputFixture) createStructuredChat(t *testing.T) (codersdk.Chat, int64) {
	t.Helper()
	chat, err := f.client.CreateChat(f.ctx, codersdk.CreateChatRequest{
		OrganizationID: f.org, Content: structuredText("report please"), ResponseFormat: structuredFormat(),
	})
	require.NoError(t, err)
	return chat, f.firstUserMessageID(t, chat.ID)
}

func (f structuredOutputFixture) firstUserMessageID(t *testing.T, chatID uuid.UUID) int64 {
	t.Helper()
	messages, err := f.client.GetChatMessages(f.ctx, chatID, nil)
	require.NoError(t, err)
	for _, msg := range messages.Messages {
		if msg.Role == codersdk.ChatMessageRoleUser {
			return msg.ID
		}
	}
	require.FailNow(t, "no user message")
	return 0
}

// settled waits for the chat to settle and returns the message's result.
func (f structuredOutputFixture) settled(t *testing.T, chatID uuid.UUID, messageID int64) codersdk.ChatStructuredOutput {
	t.Helper()
	coderdtest.WaitForChatSettled(f.ctx, t, f.api, chatID)
	out, err := f.client.ChatStructuredOutput(f.ctx, chatID, messageID)
	require.NoError(t, err)
	return out
}

func requireFailed(t *testing.T, out codersdk.ChatStructuredOutput, code codersdk.ChatStructuredOutputErrorCode) {
	t.Helper()
	require.Equal(t, codersdk.ChatStructuredOutputStatusFailed, out.Status)
	require.NotNil(t, out.Error)
	require.Equal(t, code, out.Error.Code)
	require.NotEmpty(t, out.Error.Message)
	require.Nil(t, out.Value)
}

func requireSucceeded(t *testing.T, out codersdk.ChatStructuredOutput, value string) {
	t.Helper()
	require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, out.Status, "error: %+v", out.Error)
	require.Nil(t, out.Error)
	require.JSONEq(t, value, string(out.Value))
}

// Acceptance 1: a create with response_format reads pending while the
// turn runs, then succeeded with the value after a valid finalizer call.
func TestStructuredOutputCreateSucceeds(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
		if step == 0 {
			<-release
			return finalizerResponse(`{"output":{"v":"OK"}}`)
		}
		return textResponse()
	})
	chat, messageID := f.createStructuredChat(t)
	testutil.Eventually(f.ctx, t, func(context.Context) bool { return f.calls.Load() > 0 }, testutil.IntervalFast)
	out, err := f.client.ChatStructuredOutput(f.ctx, chat.ID, messageID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStructuredOutputStatusPending, out.Status)
	require.Equal(t, messageID, out.MessageID)
	close(release)
	requireSucceeded(t, f.settled(t, chat.ID, messageID), `{"v":"OK"}`)
}

// Acceptance 2: a send with response_format to an idle chat gets the same
// result.
func TestStructuredOutputSendSucceeds(t *testing.T) {
	t.Parallel()
	f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
		if step == 1 {
			return finalizerResponse(`{"output":{"v":"SENT"}}`)
		}
		return textResponse()
	})
	chat, err := f.client.CreateChat(f.ctx, codersdk.CreateChatRequest{OrganizationID: f.org, Content: structuredText("hello")})
	require.NoError(t, err)
	coderdtest.WaitForChatSettled(f.ctx, t, f.api, chat.ID)
	sent, err := f.client.CreateChatMessage(f.ctx, chat.ID, codersdk.CreateChatMessageRequest{Content: structuredText("report"), ResponseFormat: structuredFormat()})
	require.NoError(t, err)
	require.False(t, sent.Queued)
	require.NotNil(t, sent.Message)
	requireSucceeded(t, f.settled(t, chat.ID, sent.Message.ID), `{"v":"SENT"}`)
}

// Acceptance 3 and 4: three rejected calls fail as validation_exhausted, and
// a turn without a finalizer call fails as not_produced.
func TestStructuredOutputFailures(t *testing.T) {
	t.Parallel()
	t.Run("ValidationExhausted", func(t *testing.T) {
		t.Parallel()
		f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
			if step < 3 {
				return finalizerResponse(`{"output":{"v":1}}`)
			}
			return textResponse()
		})
		chat, messageID := f.createStructuredChat(t)
		out := f.settled(t, chat.ID, messageID)
		requireFailed(t, out, codersdk.ChatStructuredOutputErrorCodeValidationExhausted)
		require.Equal(t, int32(3), out.RejectedCalls)
	})
	t.Run("NotProduced", func(t *testing.T) {
		t.Parallel()
		f := newStructuredOutputFixture(t, structuredOn, func(int) chattest.OpenAIResponse { return textResponse() })
		chat, messageID := f.createStructuredChat(t)
		requireFailed(t, f.settled(t, chat.ID, messageID), codersdk.ChatStructuredOutputErrorCodeNotProduced)
	})
}

// Acceptance 5: an interrupt, by the interrupt endpoint or by a send with
// busy_behavior=interrupt, fails the request as interrupted, or succeeds
// it when a validated value exists.
func TestStructuredOutputInterrupt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		withValue bool
		send      bool
	}{
		{name: "Endpoint"},
		{name: "EndpointWithValue", withValue: true},
		{name: "SendInterrupt", send: true},
		{name: "SendInterruptWithValue", withValue: true, send: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			blocked := make(chan struct{})
			block := make(chan struct{})
			f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
				if tt.withValue && step == 0 {
					return finalizerResponse(`{"output":{"v":"EARLY"}}`)
				}
				if step == 0 || tt.withValue && step == 1 {
					close(blocked)
					<-block
				}
				return textResponse()
			})
			// Registered after the fixture, so it runs before the fake
			// provider waits for its handlers.
			t.Cleanup(func() { close(block) })
			chat, messageID := f.createStructuredChat(t)
			testutil.TryReceive(f.ctx, t, blocked)
			if tt.send {
				_, err := f.client.CreateChatMessage(f.ctx, chat.ID, codersdk.CreateChatMessageRequest{Content: structuredText("stop"), BusyBehavior: codersdk.ChatBusyBehaviorInterrupt})
				require.NoError(t, err)
			} else {
				_, err := f.client.InterruptChat(f.ctx, chat.ID)
				require.NoError(t, err)
			}
			out := f.settled(t, chat.ID, messageID)
			if tt.withValue {
				requireSucceeded(t, out, `{"v":"EARLY"}`)
				return
			}
			requireFailed(t, out, codersdk.ChatStructuredOutputErrorCodeInterrupted)
		})
	}
}

// Acceptance 6: a generation error after a validated value succeeds (D13),
// and one before any value fails as generation_failed.
func TestStructuredOutputGenerationError(t *testing.T) {
	t.Parallel()
	fail := func() chattest.OpenAIResponse {
		return chattest.OpenAIErrorResponse(http.StatusBadRequest, "invalid_request_error", "boom")
	}
	t.Run("AfterValue", func(t *testing.T) {
		t.Parallel()
		f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
			if step == 0 {
				return finalizerResponse(`{"output":{"v":"KEPT"}}`)
			}
			return fail()
		})
		chat, messageID := f.createStructuredChat(t)
		out := f.settled(t, chat.ID, messageID)
		requireSucceeded(t, out, `{"v":"KEPT"}`)
		got, err := f.client.GetChat(f.ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusError, got.Status)
	})
	t.Run("BeforeValue", func(t *testing.T) {
		t.Parallel()
		f := newStructuredOutputFixture(t, structuredOn, func(int) chattest.OpenAIResponse { return fail() })
		chat, messageID := f.createStructuredChat(t)
		requireFailed(t, f.settled(t, chat.ID, messageID), codersdk.ChatStructuredOutputErrorCodeGenerationFailed)
	})
}

// structuredRequest sends a raw JSON body and returns the status code.
func structuredRequest(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, method, path, body string) int {
	t.Helper()
	res, err := client.Request(ctx, method, path, json.RawMessage(body))
	require.NoError(t, err)
	defer res.Body.Close()
	return res.StatusCode
}

// Acceptance 7: every 400 and 409 rule, the experiment-off rule, and no
// chat or message row on a rejected create.
func TestStructuredOutputRejections(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
		if step == 0 {
			<-block
		}
		return textResponse()
	})
	t.Cleanup(func() { close(block) })
	format := `{"type":"json_schema","json_schema":{"name":"report","schema":` + structuredTestSchema + `}}`
	org := f.org.String()
	countChats := func() int {
		chats, err := f.client.ListChats(f.ctx, nil)
		require.NoError(t, err)
		return len(chats)
	}
	before := countChats()
	for name, body := range map[string]string{
		"DuplicateKey":   `{"organization_id":"` + org + `","content":[{"type":"text","text":"x"}],"response_format":{"type":"json_schema","type":"json_schema","json_schema":{"name":"r","schema":{}}}}`,
		"UnknownKey":     `{"organization_id":"` + org + `","content":[{"type":"text","text":"x"}],"response_format":{"type":"json_schema","strict":true,"json_schema":{"name":"r","schema":{}}}}`,
		"BadName":        `{"organization_id":"` + org + `","content":[{"type":"text","text":"x"}],"response_format":{"type":"json_schema","json_schema":{"name":"a b","schema":{}}}}`,
		"BadSchema":      `{"organization_id":"` + org + `","content":[{"type":"text","text":"x"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","schema":{"$ref":"http://example.com/s.json"}}}}`,
		"PlanMode":       `{"organization_id":"` + org + `","content":[{"type":"text","text":"x"}],"plan_mode":"plan","response_format":` + format + `}`,
		"NoInitialInput": `{"organization_id":"` + org + `","response_format":` + format + `}`,
	} {
		require.Equal(t, http.StatusBadRequest, structuredRequest(f.ctx, t, f.client, http.MethodPost, "/api/v2/chats", body), name)
	}
	require.Equal(t, before, countChats(), "a rejected create writes no chat")

	// A running chat with a pending result.
	chat, messageID := f.createStructuredChat(t)
	path := "/api/v2/chats/" + chat.ID.String()
	sendBody := `{"content":[{"type":"text","text":"more"}],"response_format":` + format + `}`
	require.Equal(t, http.StatusConflict, structuredRequest(f.ctx, t, f.client, http.MethodPost, path+"/messages", sendBody), "busy chat")
	require.Equal(t, http.StatusConflict, structuredRequest(f.ctx, t, f.client, http.MethodPatch, path, `{"plan_mode":"plan"}`), "plan mode while pending")
	require.Equal(t, http.StatusConflict, structuredRequest(f.ctx, t, f.client, http.MethodPost, path+"/messages", `{"content":[{"type":"text","text":"more"}],"plan_mode":"plan"}`), "plan mode send while pending")
	editBody := `{"content":[{"type":"text","text":"edited"}],"response_format":` + format + `}`
	require.Equal(t, http.StatusBadRequest, structuredRequest(f.ctx, t, f.client, http.MethodPatch, path+"/messages/"+strconv.FormatInt(messageID, 10), editBody), "edit")
	messages, err := f.client.GetChatMessages(f.ctx, chat.ID, nil)
	require.NoError(t, err)
	require.Len(t, messages.QueuedMessages, 0, "a rejected send queues nothing")
	out, err := f.client.ChatStructuredOutput(f.ctx, chat.ID, messageID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStructuredOutputStatusPending, out.Status, "rejections leave the result alone")

	// A plan mode chat and a sub-agent chat reject a format.
	plan, err := f.client.CreateChat(f.ctx, codersdk.CreateChatRequest{OrganizationID: f.org, PlanMode: codersdk.ChatPlanModePlan})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, structuredRequest(f.ctx, t, f.client, http.MethodPost, "/api/v2/chats/"+plan.ID.String()+"/messages", sendBody), "plan mode chat")
	child := createChildChat(f.ctx, t, f.api, plan)
	require.Equal(t, http.StatusBadRequest, structuredRequest(f.ctx, t, f.client, http.MethodPost, "/api/v2/chats/"+child.ID.String()+"/messages", sendBody), "sub-agent chat")

	// With the experiment off, new requests fail and results stay readable.
	off := newStructuredOutputFixture(t, nil, func(int) chattest.OpenAIResponse { return textResponse() })
	offBody := `{"organization_id":"` + off.org.String() + `","content":[{"type":"text","text":"x"}],"response_format":` + format + `}`
	require.Equal(t, http.StatusBadRequest, structuredRequest(off.ctx, t, off.client, http.MethodPost, "/api/v2/chats", offBody), "experiment off")
	offChat, err := off.client.CreateChat(off.ctx, codersdk.CreateChatRequest{OrganizationID: off.org, Content: structuredText("plain")})
	require.NoError(t, err)
	coderdtest.WaitForChatSettled(off.ctx, t, off.api, offChat.ID)
	offMessageID := off.firstUserMessageID(t, offChat.ID)
	//nolint:gocritic // The test seeds a result as the system.
	_, err = off.api.Database.InsertChatStructuredOutput(dbauthz.AsSystemRestricted(off.ctx), database.InsertChatStructuredOutputParams{
		MessageID: offMessageID, ChatID: offChat.ID, Request: json.RawMessage(`{"name":"report","schema":{}}`),
	})
	require.NoError(t, err)
	got, err := off.client.ChatStructuredOutput(off.ctx, offChat.ID, offMessageID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatStructuredOutputStatusPending, got.Status)
	sendOff := `{"content":[{"type":"text","text":"x"}],"response_format":` + format + `}`
	require.Equal(t, http.StatusBadRequest, structuredRequest(off.ctx, t, off.client, http.MethodPost, "/api/v2/chats/"+offChat.ID.String()+"/messages", sendOff), "experiment off send")
}

// Acceptance 9: GET returns 404 for a message without a request and for
// another user's chat.
func TestStructuredOutputNotFound(t *testing.T) {
	t.Parallel()
	f := newStructuredOutputFixture(t, structuredOn, func(step int) chattest.OpenAIResponse {
		if step == 0 {
			return finalizerResponse(`{"output":{"v":"OK"}}`)
		}
		return textResponse()
	})
	chat, messageID := f.createStructuredChat(t)
	requireSucceeded(t, f.settled(t, chat.ID, messageID), `{"v":"OK"}`)

	plain, err := f.client.CreateChat(f.ctx, codersdk.CreateChatRequest{OrganizationID: f.org, Content: structuredText("plain")})
	require.NoError(t, err)
	coderdtest.WaitForChatSettled(f.ctx, t, f.api, plain.ID)
	_, err = f.client.ChatStructuredOutput(f.ctx, plain.ID, f.firstUserMessageID(t, plain.ID))
	requireSDKStatus(t, err, http.StatusNotFound)
	_, err = f.client.ChatStructuredOutput(f.ctx, plain.ID, messageID)
	requireSDKStatus(t, err, http.StatusNotFound, "a message of another chat")

	other, _ := coderdtest.CreateAnotherUser(t, f.client.Client, f.org)
	_, err = codersdk.NewExperimentalClient(other).ChatStructuredOutput(f.ctx, chat.ID, messageID)
	requireSDKStatus(t, err, http.StatusNotFound, "another user's chat")
}

func requireSDKStatus(t *testing.T, err error, status int, msgAndArgs ...any) {
	t.Helper()
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr, msgAndArgs...)
	require.Equal(t, status, sdkErr.StatusCode(), msgAndArgs...)
}

// createChildChat inserts an idle sub-agent chat under parent.
func createChildChat(ctx context.Context, t *testing.T, api *coderd.API, parent codersdk.Chat) database.Chat {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("child")})
	require.NoError(t, err)
	//nolint:gocritic // Sub-agent chats are created by chatd, not the API.
	created, err := chatstate.CreateChat(dbauthz.AsSystemRestricted(ctx), api.Database, api.Pubsub, chatstate.CreateChatInput{
		OrganizationID: parent.OrganizationID, OwnerID: parent.OwnerID, LastModelConfigID: parent.LastModelConfigID,
		ParentChatID: uuid.NullUUID{UUID: parent.ID, Valid: true}, RootChatID: uuid.NullUUID{UUID: parent.ID, Valid: true},
		Title: "child", ClientType: database.ChatClientTypeApi, InitialStatus: database.ChatStatusWaiting,
		InitialMessages: []chatstate.Message{{
			Role: database.ChatMessageRoleSystem, Content: content, Visibility: database.ChatMessageVisibilityModel,
			ContentVersion: chatprompt.CurrentContentVersion,
		}},
	})
	require.NoError(t, err)
	return created.Chat
}
