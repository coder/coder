package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestSubagentFallbackChatTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "EmptyPrompt",
			input: "",
			want:  "New Chat",
		},
		{
			name:  "ShortPrompt",
			input: "Open Firefox",
			want:  "Open Firefox",
		},
		{
			name:  "LongPrompt",
			input: "Please open the Firefox browser and navigate to the settings page",
			want:  "Please open the Firefox browser and...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := subagentFallbackChatTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

type internalTestServerConfig struct {
	logger           slog.Logger
	clock            quartz.Clock
	startWorker      bool
	experiments      codersdk.Experiments
	transportFactory *atomic.Pointer[aibridge.TransportFactory]
}

type internalTestServerOpt func(*internalTestServerConfig)

func withInternalTestServerClock(clk quartz.Clock) internalTestServerOpt {
	return func(cfg *internalTestServerConfig) {
		cfg.clock = clk
	}
}

func withInternalTestServerLogger(logger slog.Logger) internalTestServerOpt {
	return func(cfg *internalTestServerConfig) {
		cfg.logger = logger
	}
}

func withInternalTestServerWorker() internalTestServerOpt {
	return func(cfg *internalTestServerConfig) {
		cfg.startWorker = true
	}
}

func withInternalTestServerExperiments(experiments codersdk.Experiments) internalTestServerOpt {
	return func(cfg *internalTestServerConfig) {
		cfg.experiments = experiments
	}
}

// withInternalTestServerTransportFactory wires an [aibridge.TransportFactory]
// into the server's Config so tests that drive real model generation through
// runSubagentTool or processChat can control the HTTP transport AI Gateway
// routing uses.
func withInternalTestServerTransportFactory(factory aibridge.TransportFactory) internalTestServerOpt {
	return func(cfg *internalTestServerConfig) {
		cfg.transportFactory = aibridgeTestFactoryPointer(factory)
	}
}

func experimentsOrDefault(experiments codersdk.Experiments) codersdk.Experiments {
	if experiments == nil {
		return codersdk.ExperimentsKnown
	}
	return experiments
}

// newInternalTestServer creates a passive Server for internal tests with
// custom provider API keys. Pass withInternalTestServerWorker to start the
// background chat worker for tests that need real execution.
func newInternalTestServer(
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	keys chatprovider.ProviderAPIKeys,
	opts ...internalTestServerOpt,
) *Server {
	t.Helper()

	cfg := internalTestServerConfig{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	server := New(ps, Config{
		Logger:    cfg.logger,
		Database:  db,
		ReplicaID: uuid.New(),
		Clock:     cfg.clock,
		// Use a very long interval so the background loop
		// does not interfere with test assertions.
		PendingChatAcquireInterval: testutil.WaitLong,
		ProviderAPIKeys:            keys,
		Experiments:                experimentsOrDefault(cfg.experiments),
		AIBridgeTransportFactory:   cfg.transportFactory,
	})
	if cfg.startWorker {
		server.Start()
	}
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

type subscribeFailingPubsub struct {
	pubsub.Pubsub
}

func (subscribeFailingPubsub) Subscribe(_ string, _ pubsub.Listener) (func(), error) {
	return nil, xerrors.New("subscribe disabled")
}

func (subscribeFailingPubsub) SubscribeWithErr(_ string, _ pubsub.ListenerWithErr) (func(), error) {
	return nil, xerrors.New("subscribe disabled")
}

type subagentTestLogSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *subagentTestLogSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (*subagentTestLogSink) Sync() {}

func (s *subagentTestLogSink) entriesAtLevelWithMessage(
	level slog.Level,
	message string,
) []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]slog.SinkEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if entry.Level == level && entry.Message == message {
			entries = append(entries, entry)
		}
	}
	return entries
}

// seedInternalChatDeps inserts an OpenAI provider and model config
// into the database and returns the created user, organization,
// and model. This deliberately does NOT create an Anthropic
// provider.
func seedInternalChatDeps(
	t *testing.T,
	db database.Store,
) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_ = testAPIKeyID(t, db, user.ID)
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
	})

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
		IsDefault:    true,
	})

	return user, org, model
}

// insertEnabledAnthropicProvider inserts an enabled Anthropic provider for
// the current test user so computer_use flows keep Anthropic credentials
// after provider-key pruning.
func insertEnabledAnthropicProvider(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) {
	t.Helper()

	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "anthropic",
		DisplayName: "Anthropic",
		APIKey:      "test-anthropic-key",
		CreatedBy:   uuid.NullUUID{UUID: userID, Valid: true},
	})
}

func insertInternalAIProvider(
	t *testing.T,
	db database.Store,
	providerType database.AIProviderType,
	apiKey string,
	enabled bool,
) database.AIProvider {
	t.Helper()
	return dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: providerType,
	}, apiKey, func(params *database.InsertAIProviderParams) {
		params.Enabled = enabled
	})
}

func TestCreateChildSubagentChatPropagatesActiveTurnAPIKeyID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	parent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, apiKey.ID)

	server := &Server{db: db, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
	child, err := server.createChildSubagentChat(ctx, parent, "inspect the workspace", "")
	require.NoError(t, err)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: child.ID})
	require.NoError(t, err)
	var childUserMessage database.ChatMessage
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleUser {
			childUserMessage = message
			break
		}
	}
	require.NotZero(t, childUserMessage.ID)
	require.True(t, childUserMessage.APIKeyID.Valid)
	require.Equal(t, apiKey.ID, childUserMessage.APIKeyID.String)
}

func TestSendSubagentMessagePropagatesActiveTurnAPIKeyID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent-send-subagent-key",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		APIKeyID:           apiKey.ID,
	})
	require.NoError(t, err)
	child, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		ParentChatID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:     uuid.NullUUID{UUID: parent.ID, Valid: true},
		Title:          "child-send-subagent-key",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("do work"),
		},
	})
	require.NoError(t, err)

	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, apiKey.ID)
	_, err = server.sendSubagentMessage(
		ctx,
		parent.ID,
		child.ID,
		"follow up",
		SendMessageBusyBehaviorInterrupt,
	)
	require.NoError(t, err)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: child.ID})
	require.NoError(t, err)
	var latestUserMessage database.ChatMessage
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleUser && message.ID > latestUserMessage.ID {
			latestUserMessage = message
		}
	}
	require.NotZero(t, latestUserMessage.ID)
	require.True(t, latestUserMessage.APIKeyID.Valid)
	require.Equal(t, apiKey.ID, latestUserMessage.APIKeyID.String)
}

func TestCreateChildSubagentChatRequiresActiveTurnAPIKeyIDForAIGateway(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	parent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	server := &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}
	_, err := server.createChildSubagentChat(ctx, parent, "inspect the workspace", "")
	require.ErrorContains(t, err, "active turn API key ID is required for subagent messages")
}

func TestSendSubagentMessageRequiresActiveTurnAPIKeyIDForAIGateway(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-send-subagent-missing-key",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	child, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		ParentChatID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:     uuid.NullUUID{UUID: parent.ID, Valid: true},
		Title:          "child-send-subagent-missing-key",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("do work"),
		},
	})
	require.NoError(t, err)

	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
	_, err = server.sendSubagentMessage(
		ctx,
		parent.ID,
		child.ID,
		"follow up",
		SendMessageBusyBehaviorInterrupt,
	)
	require.ErrorContains(t, err, "active turn API key ID is required for subagent messages")
}

// TestSpawnAgentUsesActiveTurnAPIKeyIDFromContext verifies that, with AI
// Gateway routing enabled, the spawn_agent tool succeeds when the active
// turn's delegated API key ID is present on the context and fails without
// it. The generation worker supplies that key by enriching the tool
// execution context with withActiveTurnAPIKeyID, derived from the prompt
// rows' model build options. This guards the regression where
// executeLocalTools passed an un-enriched context to tool callbacks,
// breaking subagent spawning under AI Gateway routing.
func TestSpawnAgentUsesActiveTurnAPIKeyIDFromContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent-active-turn-key",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		APIKeyID:           apiKey.ID,
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	// The generation worker derives model build options from the prompt
	// rows; this is the source executeLocalTools uses to enrich the tool
	// execution context.
	promptRows, err := server.db.GetChatMessagesForPromptByChatID(ctx, parentChat.ID)
	require.NoError(t, err)
	modelOpts := modelBuildOptionsFromMessages(promptRows)
	require.Equal(t, apiKey.ID, modelOpts.ActiveAPIKeyID)

	// Without the delegated key on the context the spawn fails, matching
	// the original un-enriched executeLocalTools behavior.
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate work",
	})
	require.True(t, resp.IsError, "expected error without active turn key, got: %s", resp.Content)
	require.Contains(t, resp.Content, "active turn API key ID is required for subagent messages")

	// With the key on the context (as withActiveTurnAPIKeyID supplies in
	// executeLocalTools), the spawn succeeds.
	enrichedCtx := withActiveTurnAPIKeyID(ctx, modelOpts)
	resp = runSpawnAgentTool(enrichedCtx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate work",
	})
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeGeneral, result.SubagentType)
}

func TestResolveUserProviderAPIKeys_AIProvider(t *testing.T) {
	t.Parallel()

	t.Run("UserKeyWinsWhenBYOKEnabled", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", true)
		now := time.Now()
		_, err := db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
			ID:           uuid.New(),
			UserID:       user.ID,
			AIProviderID: provider.ID,
			APIKey:       "user-api-key",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		require.NoError(t, err)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, provider.ID)
		require.NoError(t, err)
		require.Equal(t, "user-api-key", keys.APIKey("openai"))
		// The expected URL is dbgen's default AIProvider BaseUrl.
		require.Equal(t, "invalid://test.invalid/", keys.BaseURL("openai"))
	})

	t.Run("ProviderKeyUsedWhenBYOKDisabled", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		server.allowBYOK = false
		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", true)
		now := time.Now()
		_, err := db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
			ID:           uuid.New(),
			UserID:       user.ID,
			AIProviderID: provider.ID,
			APIKey:       "user-api-key",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		require.NoError(t, err)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, provider.ID)
		require.NoError(t, err)
		require.Equal(t, "provider-api-key", keys.APIKey("openai"))
	})

	t.Run("ProviderTypeUsesAIProvider", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		insertInternalAIProvider(t, db, database.AIProviderTypeAzure, "provider-api-key", true)

		keys, err := server.resolveUserProviderAPIKeysForProviderType(ctx, user.ID, "azure")
		require.NoError(t, err)
		require.Equal(t, "provider-api-key", keys.APIKey("azure"))
	})

	t.Run("BedrockUsesAmbientAuth", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeBedrock, "", true)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, provider.ID)
		require.NoError(t, err)
		require.True(t, keys.HasProvider("bedrock"))
		require.Empty(t, keys.APIKey("bedrock"))
	})

	t.Run("RejectsAmbiguousProviderTypeWithoutSelectedProvider", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "first-provider-api-key", true)
		insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "second-provider-api-key", true)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, uuid.Nil)
		require.ErrorContains(t, err, "multiple enabled AI providers use provider type")
		require.Equal(t, chatprovider.ProviderAPIKeys{}, keys)
	})
}

func TestResolveChatModel_AIProviderDisabled(t *testing.T) {
	t.Parallel()

	ctx := chatdTestContext(t)
	db, ps := dbtestutil.NewDB(t)
	user, org, _ := seedInternalChatDeps(t, db)
	provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", false)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model: "gpt-4o-mini",
		AIProviderID: uuid.NullUUID{
			UUID:  provider.ID,
			Valid: true,
		},
	})
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})

	model, config, _, debugEnabled, resolvedProvider, resolvedModel, err := server.resolveChatModel(ctx, chat, modelBuildOptions{})
	require.ErrorContains(t, err, "is disabled")
	require.Nil(t, model)
	require.Equal(t, database.ChatModelConfig{}, config)
	require.False(t, debugEnabled)
	require.Empty(t, resolvedProvider)
	require.Empty(t, resolvedModel)
}

func TestResolveUserProviderAPIKeys_PreservesAnthropicKeyFromDBProvider(t *testing.T) {
	t.Parallel()

	t.Run("PreservesDBProviderKeyWithoutFallback", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)
		insertEnabledAnthropicProvider(t, db, user.ID)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, uuid.Nil)
		require.NoError(t, err)
		require.Equal(t, "test-anthropic-key", keys.Anthropic)
		require.Equal(t, "test-anthropic-key", keys.APIKey("anthropic"))
		require.Equal(t, "test-anthropic-key", keys.ByProvider["anthropic"])
	})

	t.Run("PrunesFallbackKeyWithoutEnabledProvider", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
			Anthropic: "test-anthropic-key",
		})

		ctx := chatdTestContext(t)
		user, _, _ := seedInternalChatDeps(t, db)

		keys, err := server.resolveUserProviderAPIKeys(ctx, user.ID, uuid.Nil)
		require.NoError(t, err)
		require.Empty(t, keys.Anthropic)
		require.Empty(t, keys.APIKey("anthropic"))
		_, ok := keys.ByProvider["anthropic"]
		require.False(t, ok)
	})
}

func insertInternalChatModelConfig(
	t *testing.T,
	db database.Store,
	model string,
	enabled bool,
) database.ChatModelConfig {
	return insertInternalChatModelConfigForProvider(
		t,
		db,
		"openai",
		model,
		enabled,
	)
}

func insertInternalChatProvider(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	provider string,
	apiKey string,
	centralAPIKeyEnabled bool,
	allowUserAPIKey bool,
	allowCentralAPIKeyFallback bool,
) database.AIProvider {
	t.Helper()

	providerConfig := dbgen.AIProvider(t, db, database.AIProvider{
		Type:        database.AIProviderType(provider),
		Name:        "test-" + uuid.NewString(),
		DisplayName: sql.NullString{String: provider, Valid: true},
	})
	if apiKey != "" {
		dbgen.AIProviderKey(t, db, database.AIProviderKey{
			ProviderID: providerConfig.ID,
			APIKey:     apiKey,
		})
	}

	return providerConfig
}

func insertInternalChatModelConfigForProvider(
	t *testing.T,
	db database.Store,
	provider string,
	model string,
	enabled bool,
) database.ChatModelConfig {
	t.Helper()
	return insertInternalChatModelConfigWithOptions(
		t,
		db,
		provider,
		model,
		enabled,
		json.RawMessage(`{}`),
	)
}

func insertInternalChatModelConfigWithOptions(
	t *testing.T,
	db database.Store,
	provider string,
	model string,
	enabled bool,
	options json.RawMessage,
) database.ChatModelConfig {
	t.Helper()

	// Reuse the newest AI provider of this type (creating a bare credential-less
	// one only when none exists) so the config links the provider already
	// carrying the test's credentials, or lack thereof, rather than a fresh one.
	providers, err := db.GetAIProviders(context.Background(), database.GetAIProvidersParams{IncludeDisabled: true})
	require.NoError(t, err)
	var aiProvider database.AIProvider
	for _, candidate := range providers {
		if candidate.Type != database.AIProviderType(provider) {
			continue
		}
		if aiProvider.ID == uuid.Nil || candidate.CreatedAt.After(aiProvider.CreatedAt) {
			aiProvider = candidate
		}
	}
	if aiProvider.ID == uuid.Nil {
		aiProvider = dbgen.AIProvider(t, db, database.AIProvider{
			Type: database.AIProviderType(provider),
		})
	}
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: aiProvider.ID, Valid: true},
		Model:        model,
		DisplayName:  model,
		Options:      options,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = enabled
	})

	return modelConfig
}

func insertInternalMCPServerConfig(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	slug string,
	allowInPlanMode bool,
) database.MCPServerConfig {
	t.Helper()

	return dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:     slug,
		Slug:            slug,
		Url:             "https://" + slug + ".example.com",
		AllowInPlanMode: allowInPlanMode,
		CreatedBy:       uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:       uuid.NullUUID{UUID: userID, Valid: true},
	})
}

func seedWorkspaceBinding(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) (database.WorkspaceTable, database.WorkspaceBuild, database.WorkspaceAgent) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      userID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       userID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        userID,
		OrganizationID: org.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    userID,
		OrganizationID: org.ID,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	return workspace, build, agent
}

// findToolByName returns the tool with the given name from the
// slice, or nil if no match is found.
func findToolByName(tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	for _, tool := range tools {
		if tool.Info().Name == name {
			return tool
		}
	}
	return nil
}

func chatdTestContext(t *testing.T) context.Context {
	t.Helper()
	return dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
}

func systemRestrictedTestContext(t *testing.T) context.Context {
	t.Helper()
	return dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
}

func enableInternalChatPersonalModelOverrides(
	t *testing.T,
	db database.Store,
) {
	t.Helper()
	require.NoError(
		t,
		db.UpsertChatPersonalModelOverridesEnabled(
			systemRestrictedTestContext(t),
			true,
		),
	)
}

func upsertInternalUserChatPersonalModelOverride(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	overrideContext codersdk.ChatPersonalModelOverrideContext,
	raw string,
) {
	t.Helper()
	require.NoError(
		t,
		db.UpsertUserChatPersonalModelOverride(
			systemRestrictedTestContext(t),
			database.UpsertUserChatPersonalModelOverrideParams{
				UserID: userID,
				Key:    ChatPersonalModelOverrideKey(overrideContext),
				Value:  raw,
			},
		),
	)
}

func TestCreateChildSubagentChatInheritsWorkspaceBinding(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspace.ID,
			Valid: true,
		},
		BuildID: uuid.NullUUID{
			UUID:  build.ID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agent.ID,
			Valid: true,
		},
		Title:              "bound-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(ctx, parentChat, "inspect bindings", "", childSubagentChatOptions{})
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, parentChat.OrganizationID, childChat.OrganizationID)
	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)
}

func createInternalParentChat(
	ctx context.Context,
	t *testing.T,
	server *Server,
	db database.Store,
	orgID uuid.UUID,
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     orgID,
		OwnerID:            userID,
		APIKeyID:           testAPIKeyID(t, db, userID),
		Title:              title,
		ModelConfigID:      modelConfigID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	return parentChat
}

// withSubagentDelegatedKey enriches ctx with a delegated API key ID for
// subagent tool callbacks. AI Gateway routing requires this key on the
// context; tests that do not otherwise set it should call this helper
// before invoking runSpawnAgentTool or runSubagentTool with spawn_agent.
func withSubagentDelegatedKey(ctx context.Context, t *testing.T, db database.Store, ownerID uuid.UUID) context.Context {
	t.Helper()
	return aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, db, ownerID))
}

func runSubagentTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	parentChat database.Chat,
	currentModelConfigID uuid.UUID,
	toolName string,
	args any,
) fantasy.ToolResponse {
	t.Helper()

	tools := server.subagentTools(
		ctx,
		func() database.Chat { return parentChat },
		currentModelConfigID,
	)
	tool := findToolByName(tools, toolName)
	require.NotNil(t, tool, "%s tool must be present", toolName)

	input, err := json.Marshal(args)
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  toolName,
		Input: string(input),
	})
	require.NoError(t, err)

	return resp
}

func runSpawnAgentTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	parentChat database.Chat,
	args spawnAgentArgs,
) fantasy.ToolResponse {
	t.Helper()
	return runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		args,
	)
}

func requireSpawnAgentResponse(t *testing.T, resp fantasy.ToolResponse) struct {
	ChatID       string `json:"chat_id"`
	SubagentType string `json:"type"`
} {
	t.Helper()
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	var result struct {
		ChatID       string `json:"chat_id"`
		SubagentType string `json:"type"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.NotEmpty(t, result.ChatID, "response must contain chat_id")
	require.NotEmpty(t, result.SubagentType, "response must contain type")
	return result
}

func requireSpawnAgentChildChatID(t *testing.T, resp fantasy.ToolResponse) uuid.UUID {
	t.Helper()
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	var result struct {
		ChatID string `json:"chat_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.NotEmpty(t, result.ChatID, "response must contain chat_id")

	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)
	return childID
}

func requireToolResponseMap(
	t *testing.T,
	resp fantasy.ToolResponse,
	wantError bool,
) map[string]any {
	t.Helper()
	require.Equal(t, wantError, resp.IsError, "unexpected tool error state: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

func TestCreateChildSubagentChatCopiesPlanMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "plan-parent",
		ModelConfigID:  model.ID,
		PlanMode:       planMode,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("plan this change"),
		},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	require.Equal(t, planMode, parentChat.PlanMode)

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(ctx, parentChat, "inspect bindings", "", childSubagentChatOptions{})
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, planMode, childChat.PlanMode)
}

func TestSpawnAgent_GeneralInheritsParentModelWhenOmitted(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-inherited-model",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate work",
	})
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeGeneral, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, parentChat.LastModelConfigID, childChat.LastModelConfigID)
}

func TestSpawnAgent_GeneralUsesConfiguredModelOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	overrideModel := insertInternalChatModelConfig(
		t, db, "general-override-"+uuid.NewString(), true,
	)
	require.NoError(t, db.UpsertChatGeneralModelOverride(ctx, overrideModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-general-override",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate general work",
	})
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)
	require.False(t, childChat.PlanMode.Valid)
}

func TestSpawnAgent_GeneralHonorsPersonalModelOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		enablePersonalOverride bool
		personalRaw            func(database.ChatModelConfig) string
		personalModel          func(context.Context, *testing.T, database.Store, uuid.UUID) database.ChatModelConfig
		wantModelID            func(
			database.ChatModelConfig,
			database.ChatModelConfig,
			database.ChatModelConfig,
		) uuid.UUID
	}{
		{
			name:                   "UnsetUsesDeploymentOverride",
			enablePersonalOverride: true,
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "DeploymentDefaultUsesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeDeploymentDefault)
			},
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "ChatDefaultBypassesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeChatDefault)
			},
			wantModelID: func(parentModel, _, _ database.ChatModelConfig) uuid.UUID {
				return parentModel.ID
			},
		},
		{
			name:                   "ModelUsesPersonalOverride",
			enablePersonalOverride: true,
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, _, personalModel database.ChatModelConfig) uuid.UUID {
				return personalModel.ID
			},
		},
		{
			name: "AdminFlagOffIgnoresPersonalOverride",
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeChatDefault)
			},
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "DisabledPersonalModelFallsBackToDeploymentOverride",
			enablePersonalOverride: true,
			personalModel: func(
				ctx context.Context,
				t *testing.T,
				db database.Store,
				userID uuid.UUID,
			) database.ChatModelConfig {
				return insertInternalChatModelConfig(
					t,
					db,
					"general-personal-disabled-"+uuid.NewString(),
					false,
				)
			},
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "MissingCredentialsFallsBackToDeploymentOverride",
			enablePersonalOverride: true,
			personalModel: func(
				ctx context.Context,
				t *testing.T,
				db database.Store,
				userID uuid.UUID,
			) database.ChatModelConfig {
				insertInternalChatProvider(
					t,
					db,
					userID,
					"openai-compat",
					"",
					false,
					true,
					false,
				)
				return insertInternalChatModelConfigForProvider(
					t,
					db,
					"openai-compat",
					"gpt-4o-mini",
					true,
				)
			},
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "MalformedValueUsesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return "model:not-a-uuid"
			},
			wantModelID: func(_, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

			ctx := chatdTestContext(t)
			user, org, parentModel := seedInternalChatDeps(t, db)
			deploymentModel := insertInternalChatModelConfig(
				t,
				db,
				"general-deployment-"+uuid.NewString(),
				true,
			)
			require.NoError(t, db.UpsertChatGeneralModelOverride(ctx, deploymentModel.ID.String()))
			personalModel := insertInternalChatModelConfig(
				t,
				db,
				"general-personal-"+uuid.NewString(),
				true,
			)
			if tt.personalModel != nil {
				personalModel = tt.personalModel(ctx, t, db, user.ID)
			}
			if tt.enablePersonalOverride {
				enableInternalChatPersonalModelOverrides(t, db)
			}
			if tt.personalRaw != nil {
				upsertInternalUserChatPersonalModelOverride(
					t,
					db,
					user.ID,
					codersdk.ChatPersonalModelOverrideContextGeneral,
					tt.personalRaw(personalModel),
				)
			}
			parentChat := createInternalParentChat(
				ctx,
				t,
				server,
				db,
				org.ID,
				user.ID,
				parentModel.ID,
				"parent-general-personal-override",
			)

			ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
			resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
				Type:   subagentTypeGeneral,
				Prompt: "delegate general work",
			})
			childID := requireSpawnAgentChildChatID(t, resp)

			childChat, err := db.GetChatByID(ctx, childID)
			require.NoError(t, err)
			require.Equal(
				t,
				tt.wantModelID(parentModel, deploymentModel, personalModel),
				childChat.LastModelConfigID,
			)
			require.False(t, childChat.PlanMode.Valid)
		})
	}
}

func TestSpawnAgent_GeneralOverrideLogsAndFallsBackWhenCredentialsUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	logSink := &subagentTestLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerLogger(logger))

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	insertInternalChatProvider(
		t,
		db,
		user.ID,
		"openai-compat",
		"",
		false,
		true,
		false,
	)

	overrideModel := insertInternalChatModelConfigForProvider(
		t,
		db,
		"openai-compat",
		"gpt-4o-mini",
		true,
	)
	require.NoError(t, db.UpsertChatGeneralModelOverride(ctx, overrideModel.ID.String()))
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "parent-general-credentials-fallback",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("delegate work"),
		},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "inspect provider credentials",
	})
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, model.ID, childChat.LastModelConfigID)
	require.False(t, childChat.PlanMode.Valid)
	require.Len(t, logSink.entriesAtLevelWithMessage(
		slog.LevelInfo,
		"model override credentials are unavailable, ignoring",
	), 1)
}

func TestSpawnAgent_GeneralOverrideLogsAndFallsBackWhenProviderDisabled(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	logSink := &subagentTestLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				"openai-compat": "fallback-key",
			},
		},
		withInternalTestServerLogger(logger),
	)

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai-compat",
		DisplayName: "openai-compat",
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = ""
		p.Enabled = false
		p.CentralApiKeyEnabled = false
		p.AllowUserApiKey = true
		p.AllowCentralApiKeyFallback = false
	})

	overrideModel := insertInternalChatModelConfigForProvider(
		t,
		db,
		"openai-compat",
		"gpt-4o-mini",
		true,
	)
	require.NoError(t, db.UpsertChatGeneralModelOverride(ctx, overrideModel.ID.String()))
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "parent-general-disabled-provider-fallback",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("delegate work"),
		},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "inspect disabled providers",
	})
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, model.ID, childChat.LastModelConfigID)
	require.False(t, childChat.PlanMode.Valid)
	require.Len(t, logSink.entriesAtLevelWithMessage(
		slog.LevelInfo,
		"model override is unavailable, ignoring",
	), 1)
}

func TestResolveConfiguredModelOverride_AcceptsAmbientCredentialsProvider(
	t *testing.T,
) {
	t.Parallel()

	logSink := &subagentTestLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	server := &Server{logger: logger}
	ctx := chatdTestContext(t)
	ownerID := uuid.New()
	modelConfig := database.ChatModelConfig{
		ID:          uuid.New(),
		Model:       "anthropic.claude-haiku-4-5-20251001-v1:0",
		DisplayName: "Ambient Bedrock Override",
		Enabled:     true,
	}

	resolvedModelConfig, reasoningEffort, ok, err := server.resolveConfiguredModelOverride(
		ctx,
		"plan",
		modelConfig.ID.String(),
		ownerID,
		func(
			_ context.Context,
			configuredModelConfigID uuid.UUID,
		) (database.ChatModelConfig, string, error) {
			require.Equal(t, modelConfig.ID, configuredModelConfigID)
			return modelConfig, "bedrock", nil
		},
		func(
			_ context.Context,
			resolvedOwnerID uuid.UUID,
			_ uuid.UUID,
		) (chatprovider.ProviderAPIKeys, error) {
			require.Equal(t, ownerID, resolvedOwnerID)
			return chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"bedrock": ""},
			}, nil
		},
		modelOverrideFailureModeSoft,
	)
	require.NoError(t, err)
	require.Nil(t, reasoningEffort)
	require.True(t, ok)
	require.Equal(t, modelConfig, resolvedModelConfig)
	require.Empty(t, logSink.entriesAtLevelWithMessage(
		slog.LevelInfo,
		"model override credentials are unavailable, ignoring",
	))
}

func TestWithResolvedReasoningEffort(t *testing.T) {
	t.Parallel()

	baseOptions, err := json.Marshal(codersdk.ChatModelCallConfig{
		MaxOutputTokens: ptr.Ref(int64(123)),
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: ptr.Ref(codersdk.ChatModelReasoningEffortLow),
			Max:     ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
		},
	})
	require.NoError(t, err)

	t.Run("NilEffortReturnsOriginalConfig", func(t *testing.T) {
		t.Parallel()
		modelConfig := database.ChatModelConfig{Options: baseOptions}
		require.Equal(t, modelConfig, withResolvedReasoningEffort(modelConfig, nil))
	})

	t.Run("InvalidJSONReturnsOriginalConfig", func(t *testing.T) {
		t.Parallel()
		modelConfig := database.ChatModelConfig{Options: []byte(`{`)}
		require.Equal(t, modelConfig, withResolvedReasoningEffort(modelConfig, ptr.Ref(codersdk.ChatModelReasoningEffortHigh)))
	})

	t.Run("NoReasoningConfigReturnsOriginalConfig", func(t *testing.T) {
		t.Parallel()
		modelConfig := database.ChatModelConfig{Options: []byte(`{"max_output_tokens":123}`)}
		require.Equal(t, modelConfig, withResolvedReasoningEffort(modelConfig, ptr.Ref(codersdk.ChatModelReasoningEffortHigh)))
	})

	t.Run("ClampsRequestedEffortAndPreservesOtherOptions", func(t *testing.T) {
		t.Parallel()
		modelConfig := database.ChatModelConfig{Options: baseOptions}

		got := withResolvedReasoningEffort(modelConfig, ptr.Ref(codersdk.ChatModelReasoningEffortXHigh))

		var callConfig codersdk.ChatModelCallConfig
		require.NoError(t, json.Unmarshal(got.Options, &callConfig))
		require.Equal(t, ptr.Ref(int64(123)), callConfig.MaxOutputTokens)
		require.Equal(t, ptr.Ref(codersdk.ChatModelReasoningEffortHigh), callConfig.ReasoningEffort.Default)
		require.Equal(t, ptr.Ref(codersdk.ChatModelReasoningEffortHigh), callConfig.ReasoningEffort.Max)
	})
}

func TestCreateChildSubagentChat_StoresReasoningEffortOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-effort-override",
	)
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"delegate work",
		"",
		childSubagentChatOptions{reasoningEffortOverride: ptr.Ref("high")},
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true}, childChat.LastReasoningEffort)
}

func TestCreateChildSubagentChat_OverrideWorksWhenParentHasNoModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	overrideModel := insertInternalChatModelConfig(
		t, db, "override-no-parent-model-"+uuid.NewString(), true,
	)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-no-model",
	)

	// The chats table enforces a foreign key for last_model_config_id, so
	// use a synthetic parent value here to exercise the override path.
	parentChat.LastModelConfigID = uuid.Nil
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"delegate work",
		"",
		childSubagentChatOptions{modelConfigIDOverride: &overrideModel.ID},
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreUsesConfiguredModelOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	overrideModel := insertInternalChatModelConfig(
		t, db, "explore-override-"+uuid.NewString(), true,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, overrideModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-explore-override",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "investigate the codebase"},
	)
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeExplore, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)
	require.True(t, childChat.Mode.Valid)
	require.Equal(t, database.ChatModeExplore, childChat.Mode.ChatMode)
	require.False(t, childChat.PlanMode.Valid)
}

func TestSpawnAgent_ExploreFallsBackToCurrentTurnModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(t, db)
	currentTurnModel := insertInternalChatModelConfig(
		t, db, "explore-current-turn-"+uuid.NewString(), true,
	)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-fallback",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "trace the request flow"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
	require.Equal(t, parentModel.ID, parentChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreHonorsPersonalModelOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		enablePersonalOverride bool
		personalRaw            func(database.ChatModelConfig) string
		personalModel          func(context.Context, *testing.T, database.Store, uuid.UUID) database.ChatModelConfig
		wantModelID            func(
			database.ChatModelConfig,
			database.ChatModelConfig,
			database.ChatModelConfig,
			database.ChatModelConfig,
		) uuid.UUID
	}{
		{
			name:                   "UnsetUsesDeploymentOverride",
			enablePersonalOverride: true,
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "DeploymentDefaultUsesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeDeploymentDefault)
			},
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "ChatDefaultBypassesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeChatDefault)
			},
			wantModelID: func(_, currentTurnModel, _, _ database.ChatModelConfig) uuid.UUID {
				return currentTurnModel.ID
			},
		},
		{
			name:                   "ModelUsesPersonalOverride",
			enablePersonalOverride: true,
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, _, _, personalModel database.ChatModelConfig) uuid.UUID {
				return personalModel.ID
			},
		},
		{
			name: "AdminFlagOffIgnoresPersonalOverride",
			personalRaw: func(database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeChatDefault)
			},
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "DisabledPersonalModelFallsBackToDeploymentOverride",
			enablePersonalOverride: true,
			personalModel: func(
				ctx context.Context,
				t *testing.T,
				db database.Store,
				userID uuid.UUID,
			) database.ChatModelConfig {
				return insertInternalChatModelConfig(
					t,
					db,
					"explore-personal-disabled-"+uuid.NewString(),
					false,
				)
			},
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "MissingCredentialsFallsBackToDeploymentOverride",
			enablePersonalOverride: true,
			personalModel: func(
				ctx context.Context,
				t *testing.T,
				db database.Store,
				userID uuid.UUID,
			) database.ChatModelConfig {
				insertInternalChatProvider(
					t,
					db,
					userID,
					"openai-compat",
					"",
					false,
					true,
					false,
				)
				return insertInternalChatModelConfigForProvider(
					t,
					db,
					"openai-compat",
					"gpt-4o-mini",
					true,
				)
			},
			personalRaw: func(personalModel database.ChatModelConfig) string {
				return string(codersdk.ChatPersonalModelOverrideModeModel) + ":" +
					personalModel.ID.String()
			},
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
		{
			name:                   "MalformedValueUsesDeploymentOverride",
			enablePersonalOverride: true,
			personalRaw: func(database.ChatModelConfig) string {
				return "not-a-mode"
			},
			wantModelID: func(_, _, deploymentModel, _ database.ChatModelConfig) uuid.UUID {
				return deploymentModel.ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

			ctx := chatdTestContext(t)
			user, org, parentModel := seedInternalChatDeps(t, db)
			currentTurnModel := insertInternalChatModelConfig(
				t,
				db,
				"explore-current-turn-"+uuid.NewString(),
				true,
			)
			deploymentModel := insertInternalChatModelConfig(
				t,
				db,
				"explore-deployment-"+uuid.NewString(),
				true,
			)
			require.NoError(t, db.UpsertChatExploreModelOverride(ctx, deploymentModel.ID.String()))
			personalModel := insertInternalChatModelConfig(
				t,
				db,
				"explore-personal-"+uuid.NewString(),
				true,
			)
			if tt.personalModel != nil {
				personalModel = tt.personalModel(ctx, t, db, user.ID)
			}
			if tt.enablePersonalOverride {
				enableInternalChatPersonalModelOverrides(t, db)
			}
			if tt.personalRaw != nil {
				upsertInternalUserChatPersonalModelOverride(
					t,
					db,
					user.ID,
					codersdk.ChatPersonalModelOverrideContextExplore,
					tt.personalRaw(personalModel),
				)
			}
			parentChat := createInternalParentChat(
				ctx,
				t,
				server,
				db,
				org.ID,
				user.ID,
				parentModel.ID,
				"parent-explore-personal-override",
			)

			ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
			resp := runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				currentTurnModel.ID,
				spawnAgentToolName,
				spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect the codebase"},
			)
			childID := requireSpawnAgentChildChatID(t, resp)

			childChat, err := db.GetChatByID(ctx, childID)
			require.NoError(t, err)
			require.Equal(
				t,
				tt.wantModelID(parentModel, currentTurnModel, deploymentModel, personalModel),
				childChat.LastModelConfigID,
			)
			require.True(t, childChat.Mode.Valid)
			require.Equal(t, database.ChatModeExplore, childChat.Mode.ChatMode)
			require.False(t, childChat.PlanMode.Valid)
		})
	}
}

func TestCreateChat_ExploreRootStartsWithoutMCPSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	root, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "root-explore",
		ModelConfigID:  model.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("inspect the codebase")},
	})
	require.NoError(t, err)

	rootChat, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.Empty(t, rootChat.MCPServerIDs)
}

func TestResolveExploreToolSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	user, _, _ := seedInternalChatDeps(t, db)
	approvedMCP := insertInternalMCPServerConfig(
		t, db, user.ID, "approved-"+uuid.NewString(), true,
	)
	blockedMCP := insertInternalMCPServerConfig(
		t, db, user.ID, "blocked-"+uuid.NewString(), false,
	)

	// Build parent chats in memory rather than via server.CreateChat.
	// resolveExploreToolSnapshot only reads ID, MCPServerIDs, PlanMode,
	// ParentChatID, and Mode from its parent argument, so persisting
	// the chats is unnecessary. Skipping CreateChat avoids waking the
	// background acquireLoop, which would otherwise try to dial the
	// fake MCP URLs and call OpenAI with the dbgen test API key. Those
	// side effects were the root cause of the flake tracked in
	// CODAGT-367.
	askParent := database.Chat{
		ID:           uuid.New(),
		MCPServerIDs: []uuid.UUID{approvedMCP.ID, blockedMCP.ID},
	}
	planParent := database.Chat{
		ID: uuid.New(),
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
		MCPServerIDs: []uuid.UUID{approvedMCP.ID, blockedMCP.ID},
	}

	subagentPlanParent := planParent
	subagentPlanParent.ID = uuid.New()
	subagentPlanParent.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true}

	exploreParent := askParent
	exploreParent.ID = uuid.New()
	exploreParent.Mode = database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true}
	exploreParent.ParentChatID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	exploreParent.MCPServerIDs = []uuid.UUID{approvedMCP.ID}

	tests := []struct {
		name             string
		parent           database.Chat
		wantMCPServerIDs []uuid.UUID
	}{
		{
			name:             "AskModeRootSnapshotsAllExternalTools",
			parent:           askParent,
			wantMCPServerIDs: []uuid.UUID{approvedMCP.ID, blockedMCP.ID},
		},
		{
			name:             "PlanModeRootKeepsOnlyApprovedExternalTools",
			parent:           planParent,
			wantMCPServerIDs: []uuid.UUID{approvedMCP.ID},
		},
		{
			name:             "PlanModeSubagentKeepsNoExternalTools",
			parent:           subagentPlanParent,
			wantMCPServerIDs: []uuid.UUID{},
		},
		{
			name:             "ExploreParentCannotReEscalateSnapshot",
			parent:           exploreParent,
			wantMCPServerIDs: []uuid.UUID{approvedMCP.ID},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := chatdTestContext(t)
			gotMCPServerIDs, err := server.resolveExploreToolSnapshot(
				ctx,
				tt.parent,
			)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantMCPServerIDs, gotMCPServerIDs)
		})
	}
}

func TestCreateChildSubagentChatWithOptions_ExplorePersistsMCPSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-explore-snapshot",
	)
	mcpCfg := insertInternalMCPServerConfig(
		t, db, user.ID, "snapshot-"+uuid.NewString(), false,
	)

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"inspect the codebase",
		"explore-snapshot",
		childSubagentChatOptions{
			chatMode: database.NullChatMode{
				ChatMode: database.ChatModeExplore,
				Valid:    true,
			},
			inheritedMCPServerIDs: []uuid.UUID{mcpCfg.ID},
		},
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{mcpCfg.ID}, childChat.MCPServerIDs)
}

func TestSpawnAgent_ExploreSnapshotsTurnStateParentState(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	turnStartConfig := insertInternalMCPServerConfig(
		t, db, user.ID, "turn-start-"+uuid.NewString(), false,
	)
	mutatedConfig := insertInternalMCPServerConfig(
		t, db, user.ID, "mutated-"+uuid.NewString(), true,
	)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "parent-turn-state-snapshot",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{turnStartConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("inspect the codebase"),
		},
	})
	require.NoError(t, err)

	turnParent, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, db, user.ID))
	tools := server.subagentTools(
		ctx,
		func() database.Chat { return turnParent },
		turnParent.LastModelConfigID,
	)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")

	_, err = server.db.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{
		ID: turnParent.ID,
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
	})
	require.NoError(t, err)
	_, err = server.db.UpdateChatMCPServerIDs(ctx, database.UpdateChatMCPServerIDsParams{
		ID:           turnParent.ID,
		MCPServerIDs: []uuid.UUID{mutatedConfig.ID},
	})
	require.NoError(t, err)

	reloadedParent, err := db.GetChatByID(ctx, turnParent.ID)
	require.NoError(t, err)
	require.True(t, reloadedParent.PlanMode.Valid)
	require.Equal(t, database.ChatPlanModePlan, reloadedParent.PlanMode.ChatPlanMode)
	require.ElementsMatch(t, []uuid.UUID{mutatedConfig.ID}, reloadedParent.MCPServerIDs)

	input, err := json.Marshal(spawnAgentArgs{
		Type:   subagentTypeExplore,
		Prompt: "inspect the codebase",
		Title:  "sub",
	})
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  spawnAgentToolName,
		Input: string(input),
	})
	require.NoError(t, err)

	childID := requireSpawnAgentChildChatID(t, resp)
	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.True(t, childChat.Mode.Valid)
	require.Equal(t, database.ChatModeExplore, childChat.Mode.ChatMode)
	require.ElementsMatch(t, []uuid.UUID{turnStartConfig.ID}, childChat.MCPServerIDs,
		"Explore child should keep the turn-start MCP snapshot after parent mutations")
}

func TestSpawnAgent_ExploreFallsBackOnInvalidUUID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(t, db)
	currentTurnModel := insertInternalChatModelConfig(
		t, db, "explore-invalid-override-"+uuid.NewString(), true,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, "not-a-uuid"))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-invalid-override",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect the handler flow"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreFallsBackWhenOverrideIsUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(t, db)
	currentTurnModel := insertInternalChatModelConfig(
		t, db, "explore-fallback-current-"+uuid.NewString(), true,
	)
	disabledModel := insertInternalChatModelConfig(
		t, db, "explore-disabled-"+uuid.NewString(), false,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, disabledModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-disabled",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect the service boundaries"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreFallsBackWhenOverrideCredentialsAreUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(t, db)
	currentTurnModel := insertInternalChatModelConfig(
		t, db, "explore-missing-user-key-current-"+uuid.NewString(), true,
	)
	overrideProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai-compat",
		DisplayName: "OpenAI Compat",
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = ""
		p.CentralApiKeyEnabled = false
		p.AllowUserApiKey = true
		p.AllowCentralApiKeyFallback = false
	})

	overrideModel := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: overrideProvider.ID, Valid: true},
		Model:        "gpt-4o-mini",
		DisplayName:  "Explore Override Missing User Key",
	})
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, overrideModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-missing-user-key",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect provider credential handling"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestDefaultSystemPromptPlanningGuidance_SteersSubagentSelection(t *testing.T) {
	t.Parallel()

	require.Contains(t, defaultSystemPromptPlanningGuidance, `Prefer type="general" for substantial delegated research, analysis, reasoning, review, planning support, or implementation`)
	require.Contains(t, defaultSystemPromptPlanningGuidance, `Use type="general" even for read-only work when the task is open-ended, multi-step, parallel, requires synthesis, or may later need edits`)
	require.Contains(t, defaultSystemPromptPlanningGuidance, `Use type="explore" only for narrow repository-local read-only code discovery or code tracing`)
	require.Contains(t, defaultSystemPromptPlanningGuidance, `Do not use type="explore" for generic research, broad architecture analysis, planning synthesis, external or web research, parallel research, or tasks that may need edits`)
	require.NotContains(t, defaultSystemPromptPlanningGuidance, "research the codebase")
	require.NotContains(t, defaultSystemPromptPlanningGuidance, "Reserve type=\"general\" for writable delegated work")
}

func TestSpawnAgent_DescriptionListsAllAvailableTypes(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-description-all",
	)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.Contains(t, description, subagentTypeComputerUse)
}

func TestSpawnAgent_DescriptionSteersGeneralForSubstantialResearch(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-description-selection-guidance",
	)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description

	require.Contains(t, description, `Prefer type="general" for substantial delegated research, analysis, reasoning, review, planning support, or implementation`)
	require.Contains(t, description, "even when the child should only report findings")
	require.Contains(t, description, `When using type="general" for read-only work, explicitly instruct the child not to modify files and to return findings`)
	require.Contains(t, description, `Use type="explore" only for narrow repository-local read-only code discovery or code tracing`)
	require.Contains(t, description, `Do not use type="explore" for generic research, broad architecture analysis, planning synthesis, external or web research, parallel research, or tasks that may need edits`)
}

func TestSpawnAgent_DescriptionIncludesComputerUseWithMissingProviderKey(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-description-missing-key",
	)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.Contains(t, description, subagentTypeComputerUse)
}

func TestSpawnAgent_PlanModeDescriptionOmitsComputerUse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "plan-parent-description",
		ModelConfigID:  model.ID,
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("plan this change")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.NotContains(t, description, subagentTypeComputerUse)
	require.Contains(t, description, `type="general" is for non-mutating substantial investigation and planning support`)
	require.Contains(t, description, `type="explore" is for narrow repository-local lookup or tracing`)
	require.Contains(t, description, `only type="general" should be used for cloning repositories or non-local investigation`)
	require.NotContains(t, description, "Both may use shell commands for exploration, such as cloning repositories")
	require.Contains(t, description, "must not implement changes or intentionally modify workspace files")
}

func TestSpawnAgent_PlanModeRejectsComputerUse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "plan-parent-computer-use-reject",
		ModelConfigID:  model.ID,
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("plan this change")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeComputerUse,
		Prompt: "open the browser and click around",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, `type "computer_use" is unavailable in plan mode`)
}

func TestPlanningOverlaySubagentGuidance_UsesPlanModeSafeDescriptions(t *testing.T) {
	t.Parallel()

	guidance := planningOverlaySubagentGuidance()

	require.Contains(t, guidance, subagentTypeGeneral)
	require.Contains(t, guidance, subagentTypeExplore)
	require.Contains(t, guidance, `Use type="general" for substantial investigation, reasoning, and planning support`)
	require.Contains(t, guidance, `Use type="explore" only for narrow repository-local lookup or tracing`)
	require.Contains(t, guidance, "general (non-mutating substantial investigation, analysis, and planning support)")
	require.Contains(t, guidance, "explore (narrow repository-local codebase lookup and code tracing)")
	require.NotContains(t, guidance, subagentTypeComputerUse)
	require.NotContains(t, guidance, "modify")
	require.NotContains(t, guidance, "may inspect or modify workspace files")
}

func TestSpawnAgent_InvalidTypeAndCredentialErrorAreDistinct(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-invalid-type",
	)

	invalidResp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: "invalid", Prompt: "delegate work"},
	)
	require.True(t, invalidResp.IsError)
	require.Contains(t, invalidResp.Content, "type must be one of: general, explore, computer_use")

	credentialResp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "open browser"},
	)
	require.True(t, credentialResp.IsError)
	require.Contains(t, credentialResp.Content, "API key")
	require.Contains(t, credentialResp.Content, "computer-use")
	require.Contains(t, credentialResp.Content, "anthropic")
}

func TestSpawnAgent_ComputerUseAvailabilityUsesConfiguredProvider(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	require.NoError(t, db.UpsertChatComputerUseProvider(
		ctx,
		chattool.ComputerUseProviderOpenAI,
	))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-openai-computer-use",
	)

	ids := availableSubagentTypeIDs(ctx, server, parentChat)
	require.Contains(t, ids, subagentTypeComputerUse)
}

func TestSpawnAgent_ComputerUseRejectsMissingConfiguredProvider(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	require.NoError(t, db.UpsertChatComputerUseProvider(
		ctx,
		chattool.ComputerUseProviderOpenAI,
	))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	user := dbgen.User(t, db, database.User{})
	_ = testAPIKeyID(t, db, user.ID)
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	model := insertInternalChatModelConfigForProvider(
		t,
		db,
		chattool.ComputerUseProviderOpenAI,
		"gpt-4o-mini",
		true,
	)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-openai-missing",
	)

	ids := availableSubagentTypeIDs(ctx, server, parentChat)
	require.Contains(t, ids, subagentTypeComputerUse)
	beforeChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeComputerUse,
		Prompt: "open the browser",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "API key")
	require.Contains(t, resp.Content, "computer-use")
	require.Contains(t, resp.Content, "openai")
	afterChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, afterChats, len(beforeChats))
}

func TestSpawnAgent_ComputerUseRejectsInvalidConfiguredProviderWithStableReason(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	require.NoError(t, db.UpsertChatComputerUseProvider(ctx, "bogus"))
	logSink := &subagentTestLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerLogger(logger))

	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-invalid-computer-use-provider",
	)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeComputerUse,
		Prompt: "open the browser",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, `type "computer_use" is unavailable because its provider configuration could not be loaded`)
	require.NotContains(t, resp.Content, "bogus")
	require.NotContains(t, resp.Content, "agents_computer_use_provider")
	require.NotEmpty(t, logSink.entriesAtLevelWithMessage(
		slog.LevelWarn,
		"computer-use provider config is unavailable",
	))
}

func TestSpawnAgent_ComputerUseRejectsDesktopDisabled(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	experiments := slices.DeleteFunc(
		slices.Clone(codersdk.ExperimentsKnown),
		func(e codersdk.Experiment) bool { return e == codersdk.ExperimentChatVirtualDesktop },
	)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	}, withInternalTestServerExperiments(experiments))

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-desktop-disabled",
	)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeComputerUse,
		Prompt: "open the browser",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, `type "computer_use" is unavailable because the chat-virtual-desktop experiment is not enabled`)
}

func TestSpawnAgent_BlankTypeReturnsValidOptions(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-blank-type",
	)

	tests := []struct {
		name         string
		subagentType string
	}{
		{name: "empty", subagentType: ""},
		{name: "space", subagentType: " "},
		{name: "whitespace", subagentType: "\n\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := chatdTestContext(t)
			resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
				Type:   tt.subagentType,
				Prompt: "delegate work",
			})
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, "type must be one of:")
			require.Contains(t, resp.Content, subagentTypeGeneral)
			require.Contains(t, resp.Content, subagentTypeExplore)
			require.Contains(t, resp.Content, subagentTypeComputerUse)
		})
	}
}

func TestSpawnAgent_NotAvailableForChildChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	_, child := createParentChildChats(ctx, t, server, user, org, model)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childChat.ParentChatID.Valid, "child chat must have a parent")

	tools := server.subagentTools(ctx, func() database.Chat { return childChat }, childChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-child",
		Name:  spawnAgentToolName,
		Input: `{"type":"general","prompt":"open browser"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "delegated chats cannot create child subagents")
}

func TestSpawnAgent_NotAvailableForExploreChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	exploreChat, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		Title:          "root-explore",
		ModelConfigID:  model.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("inspect the codebase")},
	})
	require.NoError(t, err)
	currentChat, err := db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return currentChat }, currentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-explore",
		Name:  spawnAgentToolName,
		Input: `{"type":"general","prompt":"delegate work"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "explore chats cannot create child subagents")
}

func TestSubagentLifecycleToolsIncludePersistedSubagentTypeAcrossVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		variant string
	}{
		{name: "General", variant: subagentTypeGeneral},
		{name: "Explore", variant: subagentTypeExplore},
		{name: "ComputerUse", variant: subagentTypeComputerUse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)

			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

			ctx := chatdTestContext(t)
			user, org, model := seedInternalChatDeps(t, db)
			if tt.variant == subagentTypeComputerUse {
				insertEnabledAnthropicProvider(t, db, user.ID)
			}
			parentChat := createInternalParentChat(
				ctx,
				t,
				server,
				db,
				org.ID,
				user.ID,
				model.ID,
				"parent-lifecycle-"+tt.variant,
			)
			ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)

			spawnResp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
				Type:   tt.variant,
				Prompt: "delegate work",
			})
			spawnResult := requireSpawnAgentResponse(t, spawnResp)
			require.Equal(t, tt.variant, spawnResult.SubagentType)
			childID, err := uuid.Parse(spawnResult.ChatID)
			require.NoError(t, err)

			setChatStatus(ctx, t, db, childID, database.ChatStatusWaiting, "")
			insertAssistantMessage(t, db, childID, model.ID, "task complete")
			waitResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"wait_agent",
				waitAgentArgs{ChatID: childID.String()},
			), false)
			require.Equal(t, tt.variant, waitResult["type"])

			messageResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"message_agent",
				messageAgentArgs{ChatID: childID.String(), Message: "follow up"},
			), false)
			require.Equal(t, tt.variant, messageResult["type"])

			setChatStatus(ctx, t, db, childID, database.ChatStatusRunning, "")
			interruptResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"interrupt_agent",
				interruptAgentArgs{ChatID: childID.String()},
			), false)
			require.Equal(t, tt.variant, interruptResult["type"])
		})
	}
}

func TestSubagentLifecycleToolErrorsIncludePersistedSubagentType(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	_, child := createParentChildChats(ctx, t, server, user, org, model)
	unrelated, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "unrelated-lifecycle-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("other")},
	})
	require.NoError(t, err)
	unrelatedChat, err := db.GetChatByID(ctx, unrelated.ID)
	require.NoError(t, err)

	tests := []struct {
		name      string
		toolName  string
		args      any
		wantError string
	}{
		{
			name:      "WaitAgent",
			toolName:  "wait_agent",
			args:      waitAgentArgs{ChatID: child.ID.String()},
			wantError: ErrSubagentNotDescendant.Error(),
		},
		{
			name:      "MessageAgent",
			toolName:  "message_agent",
			args:      messageAgentArgs{ChatID: child.ID.String(), Message: "follow up"},
			wantError: ErrSubagentNotDescendant.Error(),
		},
		{
			name:      "InterruptAgent",
			toolName:  "interrupt_agent",
			args:      interruptAgentArgs{ChatID: child.ID.String()},
			wantError: ErrSubagentNotDescendant.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := chatdTestContext(t)
			result := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				unrelatedChat,
				unrelatedChat.LastModelConfigID,
				tt.toolName,
				tt.args,
			), true)
			require.Equal(t, subagentTypeGeneral, result["type"])
			require.Equal(t, tt.wantError, result["error"])
		})
	}
}

func TestSpawnAgent_ComputerUseUsesComputerUseModelNotParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	insertEnabledAnthropicProvider(t, db, user.ID)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	seedProvider, err := db.GetAIProviderByID(ctx, model.AIProviderID.UUID)
	require.NoError(t, err)
	require.Equal(t, "openai", string(seedProvider.Type), "seed helper must create an OpenAI model")

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		WorkspaceID:        uuid.NullUUID{UUID: workspace.ID, Valid: true},
		BuildID:            uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID:            uuid.NullUUID{UUID: agent.ID, Valid: true},
		Title:              "parent-openai",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "take a screenshot"},
	)
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeComputerUse, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)

	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)
	require.True(t, childChat.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, childChat.Mode.ChatMode)
	computerUseModelProvider, computerUseModelName, ok := chattool.DefaultComputerUseModel(chattool.ComputerUseProviderAnthropic)
	require.True(t, ok)
	assert.NotEqual(t, string(seedProvider.Type), computerUseModelProvider,
		"computer use model provider must differ from parent model provider")
	assert.Equal(t, "anthropic", computerUseModelProvider)
	assert.NotEmpty(t, computerUseModelName)
}

func TestSpawnAgent_ComputerUseInheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	insertEnabledAnthropicProvider(t, db, user.ID)

	mcpCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "MCP Test",
		Slug:        "mcp-test",
		Url:         "https://mcp.example.com",
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	parentMCPIDs := []uuid.UUID{mcpCfg.ID}

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-cu-mcp",
		ModelConfigID:      model.ID,
		MCPServerIDs:       parentMCPIDs,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "check the UI"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	assert.ElementsMatch(t, parentMCPIDs, childChat.MCPServerIDs,
		"computer use child chat must inherit MCP server IDs from parent")
}

func TestCreateChildSubagentChat_InheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	// Insert two MCP server configs so we can verify both are
	// inherited by the child chat.
	mcpA := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "MCP A",
		Slug:        "mcp-a",
		Url:         "https://mcp-a.example.com",
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	mcpB := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "MCP B",
		Slug:        "mcp-b",
		Url:         "https://mcp-b.example.com",
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	parentMCPIDs := []uuid.UUID{mcpA.ID, mcpB.ID}

	// Create a parent chat with MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-with-mcp",
		ModelConfigID:      model.ID,
		MCPServerIDs:       parentMCPIDs,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Refetch the parent to get DB-populated fields.
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, parentMCPIDs, parentChat.MCPServerIDs,
		"parent chat must have the MCP server IDs we set")

	// Spawn a child subagent chat.
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"do some work",
		"child-task",
		childSubagentChatOptions{},
	)
	require.NoError(t, err)

	// Verify the child inherited the parent's MCP server IDs.
	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, parentMCPIDs, childChat.MCPServerIDs,
		"child chat must inherit MCP server IDs from parent")
}

func TestCreateChildSubagentChat_NoMCPServersStaysEmpty(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	// Create a parent chat without any MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "parent-no-mcp",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	// Spawn a child.
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, testAPIKeyID(t, server.db, parentChat.OwnerID))
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"do some work",
		"child-no-mcp",
		childSubagentChatOptions{},
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Empty(t, childChat.MCPServerIDs,
		"child chat must have empty MCP server IDs when parent has none")
}

func TestIsSubagentDescendant(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	// Build a chain: root -> child -> grandchild.
	root, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root")},
	})
	require.NoError(t, err)

	child, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		ParentChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		Title:              "child",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("child")},
	})
	require.NoError(t, err)

	grandchild, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		ParentChatID: uuid.NullUUID{
			UUID:  child.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		Title:              "grandchild",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("grandchild")},
	})
	require.NoError(t, err)

	// Build a separate, unrelated chain.
	unrelated, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "unrelated-root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated")},
	})
	require.NoError(t, err)

	unrelatedChild, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, db, user.ID),
		ParentChatID: uuid.NullUUID{
			UUID:  unrelated.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  unrelated.ID,
			Valid: true,
		},
		Title:              "unrelated-child",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated-child")},
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		ancestor uuid.UUID
		target   uuid.UUID
		want     bool
	}{
		{
			name:     "SameID",
			ancestor: root.ID,
			target:   root.ID,
			want:     false,
		},
		{
			name:     "DirectChild",
			ancestor: root.ID,
			target:   child.ID,
			want:     true,
		},
		{
			name:     "GrandChild",
			ancestor: root.ID,
			target:   grandchild.ID,
			want:     true,
		},
		{
			name:     "Unrelated",
			ancestor: root.ID,
			target:   unrelatedChild.ID,
			want:     false,
		},
		{
			name:     "RootChat",
			ancestor: child.ID,
			target:   root.ID,
			want:     false,
		},
		{
			name:     "BrokenChain",
			ancestor: root.ID,
			target:   uuid.New(),
			want:     false,
		},
		{
			name:     "NotDescendant",
			ancestor: unrelated.ID,
			target:   child.ID,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := chatdTestContext(t)
			got, err := isSubagentDescendant(ctx, db, tt.ancestor, tt.target)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// createParentChildChats creates a parent and child chat pair for
// subagent tests. The child starts in pending status.
func createParentChildChats(
	ctx context.Context,
	t *testing.T,
	server *Server,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, server.db, user.ID),
		Title:              "parent-" + t.Name(),
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	child, err = server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		APIKeyID:       testAPIKeyID(t, server.db, user.ID),
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		Title:              "child-" + t.Name(),
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do work")},
	})
	require.NoError(t, err)

	return parent, child
}

// setChatStatus transitions a chat to the given status.
func setChatStatus(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	status database.ChatStatus,
	lastError string,
) {
	t.Helper()

	params := database.UpdateChatStatusParams{
		ID:     chatID,
		Status: status,
	}
	if lastError != "" {
		encodedLastError, err := json.Marshal(codersdk.ChatError{
			Message: lastError,
			Kind:    codersdk.ChatErrorKindGeneric,
		})
		require.NoError(t, err)
		params.LastError = pqtype.NullRawMessage{RawMessage: encodedLastError, Valid: true}
	}
	_, err := db.UpdateChatStatus(ctx, params)
	require.NoError(t, err)
}

// insertAssistantMessage inserts an assistant message with v1 content
// into a chat.
func insertAssistantMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	modelID uuid.UUID,
	text string,
) {
	t.Helper()

	parts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
	data, err := json.Marshal(parts)
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chatID,
		CreatedBy:      uuid.NullUUID{},
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
		Role:           database.ChatMessageRoleAssistant,
		Content:        pqtype.NullRawMessage{RawMessage: data, Valid: true},
		ContentVersion: chatprompt.ContentVersionV1,
	})
}

func insertLinkedChatFile(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	name string,
	mediaType string,
	data []byte,
) uuid.UUID {
	t.Helper()

	file, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		Name:           name,
		Mimetype:       mediaType,
		Data:           data,
	})
	require.NoError(t, err)

	rejected, err := db.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{file.ID},
	})
	require.NoError(t, err)
	require.Zero(t, rejected)

	return file.ID
}

func TestWaitAgentDoesNotRelayComputerUseSubagentAttachments(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createComputerUseParentChild(
		t, server, user, org, model, workspace, agent,
		"parent-relay", "child-relay",
	)

	insertedFile := insertLinkedChatFile(
		ctx,
		t,
		db,
		child.ID,
		user.ID,
		workspace.OrganizationID,
		"screenshot.png",
		"image/png",
		[]byte("fake-png"),
	)
	insertAssistantMessage(t, db, child.ID, model.ID, "Shared the screenshot.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, "Shared the screenshot.", result["report"])
	require.Equal(t, string(database.ChatStatusWaiting), result["status"])
	assert.NotContains(t, result, "attachment_count")
	assert.NotContains(t, result, "attachment_warning")

	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		nil,
		[]fantasy.ToolResultContent{{
			ToolCallID:     "call-1",
			ToolName:       "wait_agent",
			ClientMetadata: resp.Metadata,
		}},
		chatloop.PersistedStep{},
		nil,
	)
	assert.Empty(t, parts)

	parentFiles, err := db.GetChatFileMetadataByChatID(ctx, parent.ID)
	require.NoError(t, err)
	assert.Empty(t, parentFiles)

	childFiles, err := db.GetChatFileMetadataByChatID(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, childFiles, 1)
	assert.Equal(t, insertedFile, childFiles[0].ID)
	assert.Equal(t, "screenshot.png", childFiles[0].Name)
	assert.Equal(t, "image/png", childFiles[0].Mimetype)
}

func TestWaitAgentDoesNotRelayRegularSubagentAttachments(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createParentChildChats(ctx, t, server, user, org, model)
	WaitUntilIdleForTest(server)

	insertedFile := insertLinkedChatFile(
		ctx,
		t,
		db,
		child.ID,
		user.ID,
		workspace.OrganizationID,
		"notes.txt",
		"text/plain",
		[]byte("release notes"),
	)
	insertAssistantMessage(t, db, child.ID, model.ID, "Shared the release notes.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, "Shared the release notes.", result["report"])
	assert.NotContains(t, result, "attachment_count")
	assert.NotContains(t, result, "attachment_warning")
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)

	parentFiles, err := db.GetChatFileMetadataByChatID(ctx, parent.ID)
	require.NoError(t, err)
	assert.Empty(t, parentFiles)

	childFiles, err := db.GetChatFileMetadataByChatID(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, childFiles, 1)
	assert.Equal(t, insertedFile, childFiles[0].ID)
	assert.Equal(t, "notes.txt", childFiles[0].Name)
	assert.Equal(t, "text/plain", childFiles[0].Mimetype)
}

func TestAwaitSubagentCompletion(t *testing.T) {
	t.Parallel()

	// Shared fixtures for subtests that use a real clock. Each
	// subtest creates its own parent+child chats (unique IDs)
	// so they don't collide. Mock-clock subtests need their own
	// DB and server so the wait loop's timers stay isolated.
	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	user, org, model := seedInternalChatDeps(t, db)

	t.Run("NotDescendant", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, _ := createParentChildChats(ctx, t, server, user, org, model)

		unrelated, err := server.CreateChat(ctx, CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              "unrelated",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("other")},
		})
		require.NoError(t, err)

		_, _, err = server.awaitSubagentCompletion(
			ctx, parent.ID, unrelated.ID, time.Second,
		)
		require.ErrorIs(t, err, ErrSubagentNotDescendant)
	})

	t.Run("AlreadyWaiting", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(t, db, child.ID, model.ID, "task complete")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Equal(t, database.ChatStatusWaiting, gotChat.Status)
		assert.Equal(t, "task complete", report)
	})

	t.Run("AlreadyError", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "something broke")
		insertAssistantMessage(t, db, child.ID, model.ID, "partial work done")

		_, _, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "partial work done")
	})

	t.Run("AlreadyErrorNoReport", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "crash")

		_, _, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent reached error status")
	})

	t.Run("CompletesViaPoll", func(t *testing.T) {
		t.Parallel()

		// Force subscription failure so awaitSubagentCompletion
		// falls back to the fast 200ms poll interval.
		db, _ := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		ps := subscribeFailingPubsub{Pubsub: pubsub.NewInMemory()}
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerClock(mClock))
		ctx := chatdTestContext(t)
		user, org, model := seedInternalChatDeps(t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		setChatStatus(ctx, t, db, parent.ID, database.ChatStatusRunning, "")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

		// Set the trap BEFORE starting the goroutine so we
		// deterministically catch the ticker creation.
		tickTrap := mClock.Trap().NewTicker("chatd", "subagent_poll")

		type awaitResult struct {
			chat   database.Chat
			report string
			err    error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			chat, report, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, 5*time.Second,
			)
			resultCh <- awaitResult{chat, report, err}
		}()

		// Wait for the poll ticker to be created, confirming
		// the function passed its initial check and entered
		// the loop. Then release the call.
		tickTrap.MustWait(ctx).MustRelease(ctx)
		tickTrap.Close()

		// Now set the state and advance the clock to the next
		// tick so the poll detects the transition.
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(t, db, child.ID, model.ID, "poll result")
		mClock.Advance(subagentAwaitPollInterval).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.NoError(t, result.err)
		assert.Equal(t, child.ID, result.chat.ID)
		assert.Equal(t, "poll result", result.report)
	})

	t.Run("CompletesViaPubsub", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerClock(mClock))
		ctx := chatdTestContext(t)
		user, org, model := seedInternalChatDeps(t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// signalWake from CreateChat may trigger immediate processing.
		// Wait for it to settle, then reset chats to the state we need.
		WaitUntilIdleForTest(server)
		setChatStatus(ctx, t, db, parent.ID, database.ChatStatusRunning, "")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

		// Trap the fallback poll ticker to know when the
		// function has entered the wait setup path. We still
		// need an explicit subscription handshake below because
		// the ticker can be created before SubscribeWithErr has
		// finished registering the listener.
		tickTrap := mClock.Trap().NewTicker("chatd", "subagent_poll")

		type awaitResult struct {
			chat   database.Chat
			report string
			err    error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			chat, report, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, 5*time.Second,
			)
			resultCh <- awaitResult{chat, report, err}
		}()

		// Wait for the ticker to be created so the waiter has
		// entered its setup path, then subscribe our own probe on
		// the same channel. Because MemoryPubsub publishes only to
		// listeners already present at Publish time, waiting for
		// our probe to receive a message proves the waiter's
		// subscription is also registered before we assert on the
		// wake-up behavior.
		tickTrap.MustWait(ctx).MustRelease(ctx)
		tickTrap.Close()

		probeCh := make(chan struct{}, 1)
		cancelProbe, err := ps.SubscribeWithErr(
			coderdpubsub.ChatStateUpdateChannel(child.ID),
			func(_ context.Context, _ []byte, _ error) {
				select {
				case probeCh <- struct{}{}:
				default:
				}
			},
		)
		require.NoError(t, err)
		defer cancelProbe()

		// Insert the message before transitioning to Waiting so any
		// notification observing the terminal status can also read the
		// committed report.
		insertAssistantMessage(t, db, child.ID, model.ID, "pubsub result")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			chat, report, done, err := server.checkSubagentCompletion(ctx, child.ID)
			require.NoError(c, err)
			assert.True(c, done)
			assert.Equal(c, child.ID, chat.ID)
			assert.Equal(c, "pubsub result", report)
		}, testutil.WaitMedium, testutil.IntervalFast)
		require.NoError(t, ps.Publish(
			coderdpubsub.ChatStateUpdateChannel(child.ID),
			[]byte("done"),
		))
		testutil.RequireReceive(ctx, t, probeCh)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.NoError(t, result.err)
		assert.Equal(t, child.ID, result.chat.ID)
		assert.Equal(t, "pubsub result", result.report)
	})

	t.Run("AlreadyWaitingNoReport", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// This case should return immediately, so use the shared
		// real-clock server instead of a mock clock.
		WaitUntilIdleForTest(server)
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, 5*time.Second,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Empty(t, report)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerClock(mClock))
		ctx := chatdTestContext(t)
		user, org, model := seedInternalChatDeps(t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// Trap the timeout timer to know when the function
		// has entered its poll loop.
		timerTrap := mClock.Trap().NewTimer("chatd", "subagent_await")

		type awaitResult struct {
			err error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			_, _, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, time.Second,
			)
			resultCh <- awaitResult{err}
		}()

		// Wait for the timer to be created, release it.
		timerTrap.MustWait(ctx).MustRelease(ctx)
		timerTrap.Close()

		// Advance to the timeout. With pubsub, the fallback
		// poll is at 5s, so the 1s timer fires first.
		mClock.Advance(time.Second).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.Error(t, result.err)
		assert.Contains(t, result.err.Error(), "timed out waiting for delegated subagent completion")
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()

		providerCalled := make(chan struct{}, 1)
		providerReleased := make(chan struct{})
		providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case providerCalled <- struct{}{}:
			default:
			}

			select {
			case <-r.Context().Done():
			case <-providerReleased:
			}
		}))
		t.Cleanup(func() {
			close(providerReleased)
			providerServer.Close()
		})

		db, ps := dbtestutil.NewDB(t)
		providerServerURL, err := url.Parse(providerServer.URL)
		require.NoError(t, err)
		factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL.Scheme = providerServerURL.Scheme
			cloned.URL.Host = providerServerURL.Host
			cloned.Host = providerServerURL.Host
			return http.DefaultTransport.RoundTrip(cloned)
		})}
		server := newInternalTestServer(
			t, db, ps, chatprovider.ProviderAPIKeys{},
			withInternalTestServerWorker(),
			withInternalTestServerTransportFactory(factory),
		)
		ctx := chatdTestContext(t)
		user, org, _ := seedInternalChatDeps(t, db)
		provider := dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:    "openai",
			DisplayName: "OpenAI",
			BaseUrl:     providerServer.URL,
		})
		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "gpt-4o-mini",
			AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
		})

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		testutil.RequireReceive(ctx, t, providerCalled)

		// Use a short-lived context instead of goroutine + sleep.
		shortCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
		defer cancel()

		_, _, err = server.awaitSubagentCompletion(
			shortCtx, parent.ID, child.ID, 5*time.Second,
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("ZeroTimeoutUsesDefault", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// Pre-complete the child so it returns immediately.
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(t, db, child.ID, model.ID, "zero timeout ok")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, 0,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Equal(t, "zero timeout ok", report)
	})
}

func TestWaitAgentTimeoutReturnsInformationalPayload(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	mClock := quartz.NewMock(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerClock(mClock))
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent, child := createParentChildChats(ctx, t, server, user, org, model)

	WaitUntilIdleForTest(server)
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

	timerTrap := mClock.Trap().NewTimer("chatd", "subagent_await")

	type toolResult struct {
		resp fantasy.ToolResponse
	}
	resultCh := make(chan toolResult, 1)
	oneSecond := 1
	go func() {
		resp := runSubagentTool(
			ctx,
			t,
			server,
			parent,
			parent.LastModelConfigID,
			"wait_agent",
			waitAgentArgs{ChatID: child.ID.String(), TimeoutSeconds: &oneSecond},
		)
		resultCh <- toolResult{resp: resp}
	}()

	// Wait for the timer to be created, then advance past it.
	timerTrap.MustWait(ctx).MustRelease(ctx)
	timerTrap.Close()
	mClock.Advance(time.Second).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, resultCh)
	m := requireToolResponseMap(t, result.resp, false)

	require.Equal(t, true, m["timed_out"])
	require.Equal(t, child.ID.String(), m["chat_id"])
	require.Equal(t, string(database.ChatStatusRunning), m["status"])
	require.Equal(t, subagentTypeGeneral, m["type"])
}

func TestWaitAgentErrorStatusReturnsStructuredPayload(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent, child := createParentChildChats(ctx, t, server, user, org, model)

	// An errored, non-archived agent is often recoverable. wait_agent
	// must surface a structured payload (status, last_error, report)
	// rather than a bare tool error.
	WaitUntilIdleForTest(server)
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "provider overloaded")
	insertAssistantMessage(t, db, child.ID, model.ID, "partial progress")

	result := requireToolResponseMap(t, runSubagentTool(
		ctx,
		t,
		server,
		parent,
		parent.LastModelConfigID,
		"wait_agent",
		waitAgentArgs{ChatID: child.ID.String()},
	), false)

	require.Equal(t, string(database.ChatStatusError), result["status"])
	require.Equal(t, child.ID.String(), result["chat_id"])
	require.Equal(t, "provider overloaded", result["last_error"])
	require.Equal(t, "partial progress", result["report"])
	require.Equal(t, subagentTypeGeneral, result["type"])
	require.NotContains(t, result, "timed_out")
}

func TestWaitAgentTimeoutGapCompletesWithError(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	mClock := quartz.NewMock(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerClock(mClock))
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent, child := createParentChildChats(ctx, t, server, user, org, model)

	WaitUntilIdleForTest(server)
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

	timerTrap := mClock.Trap().NewTimer("chatd", "subagent_await")

	type toolResult struct {
		resp fantasy.ToolResponse
	}
	resultCh := make(chan toolResult, 1)
	oneSecond := 1
	go func() {
		resp := runSubagentTool(
			ctx,
			t,
			server,
			parent,
			parent.LastModelConfigID,
			"wait_agent",
			waitAgentArgs{ChatID: child.ID.String(), TimeoutSeconds: &oneSecond},
		)
		resultCh <- toolResult{resp: resp}
	}()

	// Wait for the timer to be created, then advance past it.
	timerTrap.MustWait(ctx).MustRelease(ctx)
	timerTrap.Close()

	// Flip the child to error before the timer fires so the
	// timeout-gap branch (checkSubagentCompletion after timeout)
	// classifies it through handleSubagentDone.
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "provider overloaded")
	insertAssistantMessage(t, db, child.ID, model.ID, "partial progress")

	mClock.Advance(time.Second).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, resultCh)
	m := requireToolResponseMap(t, result.resp, false)

	require.Equal(t, string(database.ChatStatusError), m["status"])
	require.Equal(t, "provider overloaded", m["last_error"])
	require.Equal(t, "partial progress", m["report"])
	require.Equal(t, child.ID.String(), m["chat_id"])
	require.Equal(t, subagentTypeGeneral, m["type"])
	require.NotContains(t, m, "timed_out")
}

func listAgentsChatIDs(t *testing.T, result map[string]any) []string {
	t.Helper()
	agents, ok := result["agents"].([]any)
	require.True(t, ok, "agents must be an array")
	ids := make([]string, 0, len(agents))
	for _, raw := range agents {
		agent, ok := raw.(map[string]any)
		require.True(t, ok, "each agent must be an object")
		id, ok := agent["chat_id"].(string)
		require.True(t, ok, "each agent must have a chat_id")
		ids = append(ids, id)
	}
	return ids
}

func TestListAgents(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	user, org, model := seedInternalChatDeps(t, db)

	// Helpers take the running subtest's t and ctx so a failed require
	// fires on the correct goroutine.
	newParent := func(t *testing.T, ctx context.Context, title string) database.Chat {
		t.Helper()
		parent, err := server.CreateChat(ctx, CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			APIKeyID:           testAPIKeyID(t, db, user.ID),
			Title:              title,
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		})
		require.NoError(t, err)
		return parent
	}
	newChild := func(t *testing.T, ctx context.Context, parent database.Chat, title string, mode database.NullChatMode) database.Chat {
		t.Helper()
		child, err := server.CreateChat(ctx, CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			APIKeyID:       testAPIKeyID(t, db, user.ID),
			ParentChatID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:     uuid.NullUUID{UUID: parent.ID, Valid: true},
			Title:          title,
			ModelConfigID:  model.ID,
			ChatMode:       mode,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("do work"),
			},
		})
		require.NoError(t, err)
		return child
	}

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-empty")

		result := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{},
		), false)

		require.Equal(t, float64(0), result["total"])
		require.Equal(t, float64(0), result["returned"])
		require.Equal(t, false, result["has_more"])
		require.Empty(t, listAgentsChatIDs(t, result))
	})

	t.Run("ReturnsChildren", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-children")
		generalChild := newChild(t, ctx, parent, "general-child", database.NullChatMode{})
		exploreChild := newChild(t, ctx, parent, "explore-child", database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		})

		result := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{},
		), false)

		require.Equal(t, float64(2), result["total"])
		require.Equal(t, float64(2), result["returned"])
		require.Equal(t, false, result["has_more"])
		ids := listAgentsChatIDs(t, result)
		require.Contains(t, ids, generalChild.ID.String())
		require.Contains(t, ids, exploreChild.ID.String())

		agents, ok := result["agents"].([]any)
		require.True(t, ok)
		typesByID := map[string]string{}
		for _, raw := range agents {
			agent := raw.(map[string]any)
			typesByID[agent["chat_id"].(string)] = agent["type"].(string)
			require.NotEmpty(t, agent["created_at"])
			require.NotEmpty(t, agent["updated_at"])
		}
		require.Equal(t, subagentTypeGeneral, typesByID[generalChild.ID.String()])
		require.Equal(t, subagentTypeExplore, typesByID[exploreChild.ID.String()])
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-pagination")
		newChild(t, ctx, parent, "child-a", database.NullChatMode{})
		newChild(t, ctx, parent, "child-b", database.NullChatMode{})
		newChild(t, ctx, parent, "child-c", database.NullChatMode{})

		limit := 2
		first := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{Limit: &limit},
		), false)
		require.Equal(t, float64(3), first["total"])
		require.Equal(t, float64(2), first["returned"])
		require.Equal(t, true, first["has_more"])
		firstIDs := listAgentsChatIDs(t, first)
		require.Len(t, firstIDs, 2)

		offset := 2
		second := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{Limit: &limit, Offset: &offset},
		), false)
		require.Equal(t, float64(3), second["total"])
		require.Equal(t, float64(1), second["returned"])
		require.Equal(t, false, second["has_more"])
		secondIDs := listAgentsChatIDs(t, second)
		require.Len(t, secondIDs, 1)
		require.NotContains(t, firstIDs, secondIDs[0])
	})

	t.Run("OrderByUpdatedAtDesc", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-order")
		older := newChild(t, ctx, parent, "older-child", database.NullChatMode{})
		newChild(t, ctx, parent, "newer-child", database.NullChatMode{})

		// Touch the older child so its updated_at advances past the
		// newer one; it must then sort first.
		setChatStatus(ctx, t, db, older.ID, database.ChatStatusWaiting, "")

		result := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{},
		), false)
		ids := listAgentsChatIDs(t, result)
		require.Len(t, ids, 2)
		require.Equal(t, older.ID.String(), ids[0])
	})

	t.Run("ExcludesArchived", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-archived")
		archivedChild := newChild(t, ctx, parent, "archived-child", database.NullChatMode{})

		WaitUntilIdleForTest(server)
		// SetArchived is only allowed from a waiting/error state, so
		// settle the family into waiting first. Archiving then marks
		// the children archived; they must be excluded from
		// list_agents by default.
		setChatStatus(ctx, t, db, parent.ID, database.ChatStatusWaiting, "")
		setChatStatus(ctx, t, db, archivedChild.ID, database.ChatStatusWaiting, "")
		require.NoError(t, server.ArchiveChat(ctx, parent))

		result := requireToolResponseMap(t, runSubagentTool(
			ctx, t, server, parent, parent.LastModelConfigID,
			"list_agents", listAgentsArgs{},
		), false)
		require.Equal(t, float64(0), result["total"])
		require.Empty(t, listAgentsChatIDs(t, result))
	})

	t.Run("DelegatedChatRejected", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)
		parent := newParent(t, ctx, "list-agents-delegated")
		child := newChild(t, ctx, parent, "delegated-caller", database.NullChatMode{})

		resp := runSubagentTool(
			ctx, t, server, child, child.LastModelConfigID,
			"list_agents", listAgentsArgs{},
		)
		require.True(t, resp.IsError, "list_agents on a delegated chat must return an error")
		msg := resp.Content
		require.Contains(t, msg, "only available on root chats")
	})
}
