package chatd //nolint:testpackage // Locks unexported model-call construction behavior.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// The tests in this file lock the outgoing request shape of each LLM call
// flow: which flows carry model-config provider options and which
// deliberately omit them, plus token defaults. They guard the model-call
// resolver refactor against silent behavior changes.

func modelCallSentinelOptions(t *testing.T, user string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: ptr.Ref(user),
			},
		},
	})
	require.NoError(t, err)
	return raw
}

func openAIResponsesObjectBody(t *testing.T, object string) string {
	t.Helper()
	text := strconv.Quote(object)
	return `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4o-mini","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":` + text + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
}

func TestModelCallShapeStandardTurn(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		Options:      modelCallSentinelOptions(t, "turn-options-sentinel"),
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "standard turn request shape",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "hello"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.prepareGeneration(ctx, generationPrepareInput{
		Chat:     created.Chat,
		Messages: created.InitialMessages,
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Cleanup)

	providerOptions, ok := prepared.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.ProviderOptions[fantasyopenai.Name])
	require.NotNil(t, providerOptions.User)
	require.Equal(t, "turn-options-sentinel", *providerOptions.User)

	require.NotNil(t, prepared.ModelConfig.MaxOutputTokens)
	require.Equal(t, int64(32_000), *prepared.ModelConfig.MaxOutputTokens)

	// The chat-model compaction summary call carries no provider options
	// even when the model config has them.
	require.NotNil(t, prepared.Compaction)
	require.Nil(t, prepared.Compaction.Options.ProviderOptions)
}

func TestModelCallShapeManualTitleCarriesProviderOptions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, messages := titleOverrideTestChatAndMessages(t)
	chat.OrganizationID = uuid.New()
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
	overrideConfig.Options = modelCallSentinelOptions(t, "title-options-sentinel")
	provider := database.AIProvider{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}

	var (
		bodyMu sync.Mutex
		bodies [][]byte
	)
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		bodyBytes, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		bodyMu.Lock()
		bodies = append(bodies, bodyBytes)
		bodyMu.Unlock()
		body := openAIResponsesObjectBody(t, `{"title":"Locked title"}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	db.EXPECT().GetChatMessagesByChatIDAscPaginated(gomock.Any(), database.GetChatMessagesByChatIDAscPaginatedParams{
		ChatID:   chat.ID,
		AfterID:  0,
		LimitVal: manualTitleMessageWindowLimit,
	}).Return(messages, nil)
	db.EXPECT().GetChatMessagesByChatIDDescPaginated(gomock.Any(), database.GetChatMessagesByChatIDDescPaginatedParams{
		ChatID:   chat.ID,
		BeforeID: 0,
		LimitVal: manualTitleMessageWindowLimit,
	}).Return(nil, nil)
	db.EXPECT().GetChatGatewayAPIKey(gomock.Any(), database.GetChatGatewayAPIKeyParams{
		UserID:    chat.OwnerID,
		TokenName: GatewayTokenName(chat.OwnerID),
	}).Return(database.APIKey{
		ID:        uuid.NewString(),
		UserID:    chat.OwnerID,
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}, nil)
	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	server.clock = quartz.NewReal()
	server.aibridgeTransportFactory = aibridgeTestFactoryPointer(factory)
	title, err := server.generateManualTitleCandidate(ctx, db, chat)
	require.NoError(t, err)
	require.Equal(t, "Locked title", title)

	bodyMu.Lock()
	defer bodyMu.Unlock()
	require.Len(t, bodies, 1)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(bodies[0], &raw))
	require.Equal(t, "title-options-sentinel", raw["user"])
}

func TestModelCallShapeChatSummaryOmitsProviderOptions(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		Options:      modelCallSentinelOptions(t, "summary-options-sentinel"),
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	longPrompt := strings.Repeat("please summarize this conversation carefully ", 10)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "summary request shape",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, longPrompt),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	var (
		bodyMu sync.Mutex
		bodies [][]byte
	)
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		bodyBytes, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		bodyMu.Lock()
		bodies = append(bodies, bodyBytes)
		bodyMu.Unlock()
		body := openAIResponsesObjectBody(t, `{"summary":"A locked summary."}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(factory),
	)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server.generateAndStoreChatSummary(ctx, logger, created.Chat)

	fetched, err := db.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.True(t, fetched.Summary.Valid)
	require.Equal(t, "A locked summary.", fetched.Summary.String)

	bodyMu.Lock()
	defer bodyMu.Unlock()
	require.Len(t, bodies, 1)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(bodies[0], &raw))
	// The summary call omits model-config provider options entirely.
	require.NotContains(t, raw, "user")
}

func TestModelCallShapeTurnStatusLabelEnvelope(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	var (
		callMu   sync.Mutex
		captured []fantasy.ObjectCall
	)
	model := &chattest.FakeModel{
		ProviderName: fantasyopenai.Name,
		ModelName:    "gpt-4o-mini",
		GenerateObjectFn: func(_ context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			callMu.Lock()
			captured = append(captured, call)
			callMu.Unlock()
			return &fantasy.ObjectResponse{
				Object: map[string]any{"label": "Finished the tests"},
			}, nil
		},
	}

	server := &Server{logger: logger}
	label := server.generateTurnStatusLabel(
		ctx,
		database.Chat{ID: uuid.New(), OwnerID: uuid.New(), Title: "status shape"},
		database.ChatStatusWaiting,
		"All tests pass now.",
		resolvedModelCall{
			model:            chatprovider.NewModel(model, nil),
			dbConfig:         database.ChatModelConfig{Options: modelCallSentinelOptions(t, "status-options-sentinel")},
			resolvedProvider: fantasyopenai.Name,
			resolvedModel:    "gpt-4o-mini",
		},
		modelBuildOptions{},
		logger,
		nil,
		0,
		0,
	)
	require.Equal(t, "Finished the tests", label)

	callMu.Lock()
	defer callMu.Unlock()
	require.Len(t, captured, 1)
	call := captured[0]
	// The status-label call omits model-config provider options even though
	// the config JSON carries them.
	require.Nil(t, call.ProviderOptions)
	require.NotNil(t, call.MaxOutputTokens)
	require.Equal(t, int64(64), *call.MaxOutputTokens)
	require.NotNil(t, call.Temperature)
	require.Equal(t, quickgenTemperature, *call.Temperature)
	require.Equal(t, "propose_turn_status_label", call.SchemaName)
}
