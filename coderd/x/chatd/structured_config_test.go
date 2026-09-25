package chatd_test

import (
	"context"
	"encoding/json"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// An open structured request the finalizer cannot serve fails the turn with
// one configuration_error receipt and no model call; ordinary chats with
// the same configuration are unchanged.
func TestStructuredOutputConfigurationErrors(t *testing.T) {
	t.Parallel()
	strictOptions := json.RawMessage(`{"provider_options":{"openai":{"strict_json_schema":true}}}`)
	valid, unsupported := json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"type":"string","format":"not-a-format"}`)
	for _, tt := range []struct {
		name      string
		provider  string
		options   json.RawMessage
		dynamic   string
		schema    json.RawMessage
		wantError string
	}{
		{name: "StrictToolSchemas", provider: "openai", options: strictOptions, schema: valid, wantError: "The model's strict tool schema setting cannot be used with structured output."},
		{name: "DynamicToolCollision", provider: "openai-compat", dynamic: chatstructured.FinalizerToolName, schema: valid, wantError: "Another tool is named coder_structured_output, which structured output reserves."},
		{name: "InvalidStoredSchema", provider: "openai-compat", schema: unsupported, wantError: "The stored structured output schema no longer compiles."},
		{name: "OrdinaryCollision", provider: "openai-compat", dynamic: chatstructured.FinalizerToolName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			var streamed atomic.Int32
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				streamed.Add(1)
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			provider := dbgen.ChatProvider(t, db, database.ChatProvider{Provider: tt.provider, DisplayName: tt.provider, BaseUrl: openAIURL})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, IsDefault: true, OrganizationID: org.ID, Model: "gpt-5", Options: tt.options,
			})
			factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
			})
			var tools json.RawMessage
			if tt.dynamic != "" {
				var err error
				tools, err = json.Marshal([]mcp.Tool{{Name: tt.dynamic, InputSchema: map[string]any{"type": "object"}}})
				require.NoError(t, err)
			}
			content := []codersdk.ChatMessagePart{codersdk.ChatMessageText("answer")}
			requestID := uuid.New()
			if tt.schema != nil {
				request, err := chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: tt.schema})
				require.NoError(t, err)
				content = append(content, request)
			}
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID, OwnerID: user.ID, Title: "config", ModelConfigID: model.ID, InitialUserContent: content, DynamicTools: tools,
			})
			require.NoError(t, err)
			var done database.Chat
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				done, err = db.GetChatByID(ctx, chat.ID)
				return err == nil && (done.Status == database.ChatStatusWaiting || done.Status == database.ChatStatusError)
			}, testutil.IntervalFast)

			history, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
			require.NoError(t, err)
			var receipts []codersdk.ChatStructuredOutput
			for _, msg := range history {
				if out, err := chatstate.ReceiptRowOutcome(msg); err == nil {
					receipts = append(receipts, out)
				}
			}
			if tt.wantError == "" {
				require.Equal(t, database.ChatStatusWaiting, done.Status, chatLastErrorMessage(done.LastError))
				require.EqualValues(t, 1, streamed.Load())
				require.Empty(t, receipts)
				return
			}
			require.Equal(t, database.ChatStatusError, done.Status)
			require.Equal(t, tt.wantError, chatLastErrorMessage(done.LastError))
			require.Zero(t, streamed.Load())
			require.Equal(t, []codersdk.ChatStructuredOutput{{
				RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusFailed,
				Error: &codersdk.ChatStructuredOutputError{Code: codersdk.ChatStructuredOutputErrorCodeConfigurationError, Message: tt.wantError},
			}}, receipts)
			rows := make([]chatstructured.Row, 0, len(history))
			for _, msg := range history {
				parts, err := chatprompt.ParseContent(msg)
				require.NoError(t, err)
				rows = append(rows, chatstructured.Row{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts})
			}
			state, err := chatstructured.ActiveRequest(rows)
			require.NoError(t, err)
			require.True(t, state.Closed)
		})
	}
}
