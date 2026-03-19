package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestComputerUseSubagentSystemPrompt(t *testing.T) {
	t.Parallel()

	// Verify the system prompt constant is non-empty and contains
	// key instructions for the computer use agent.
	assert.NotEmpty(t, computerUseSubagentSystemPrompt)
	assert.Contains(t, computerUseSubagentSystemPrompt, "computer")
	assert.Contains(t, computerUseSubagentSystemPrompt, "screenshot")
}

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

// newInternalTestServer creates a Server for internal tests with
// custom provider API keys. The server is automatically closed
// when the test finishes.
func newInternalTestServer(
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	keys chatprovider.ProviderAPIKeys,
) *Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := New(Config{
		Logger:    logger,
		Database:  db,
		ReplicaID: uuid.New(),
		Pubsub:    ps,
		// Use a very long interval so the background loop
		// does not interfere with test assertions.
		PendingChatAcquireInterval: testutil.WaitLong,
		ProviderAPIKeys:            keys,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// seedInternalChatDeps inserts an OpenAI provider and model config
// into the database and returns the created user and model. This
// deliberately does NOT create an Anthropic provider.
func seedInternalChatDeps(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	return user, model
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

func TestSpawnComputerUseAgent_NoAnthropicProvider(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// No Anthropic key in ProviderAPIKeys.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Create a root parent chat.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-no-anthropic",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Re-fetch so LastModelConfigID is populated from the DB.
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	assert.Nil(t, tool, "spawn_computer_use_agent tool must be omitted when Anthropic is not configured")
}

func TestSpawnComputerUseAgent_NotAvailableForChildChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// Provide an Anthropic key so the provider check passes.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Create a root parent chat.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "root-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Create a child chat under the parent.
	child, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		Title:              "child-subagent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do something")},
	})
	require.NoError(t, err)

	// Re-fetch the child so ParentChatID is populated.
	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childChat.ParentChatID.Valid,
		"child chat must have a parent")

	// Get tools as if the child chat is the current chat.
	tools := server.subagentTools(ctx, func() database.Chat { return childChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	require.NotNil(t, tool, "spawn_computer_use_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  "spawn_computer_use_agent",
		Input: `{"prompt":"open browser"}`,
	})
	require.NoError(t, err)

	assert.True(t, resp.IsError, "expected an error response")
	assert.Contains(t, resp.Content, "delegated chats cannot create child subagents")
}

func TestSpawnComputerUseAgent_DesktopDisabled(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-desktop-disabled",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	assert.Nil(t, tool, "spawn_computer_use_agent tool must be omitted when desktop is disabled")
}

func TestSpawnComputerUseAgent_UsesComputerUseModelNotParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// Provide an Anthropic key so the tool can proceed.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// The parent uses an OpenAI model.
	require.Equal(t, "openai", model.Provider,
		"seed helper must create an OpenAI model")

	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-openai",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	require.NotNil(t, tool)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  "spawn_computer_use_agent",
		Input: `{"prompt":"take a screenshot"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	// Parse the response to get the child chat ID.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	childIDStr, ok := result["chat_id"].(string)
	require.True(t, ok, "response must contain chat_id")

	childID, err := uuid.Parse(childIDStr)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)

	// The child must have Mode=computer_use which causes
	// runChat to override the model to the predefined computer
	// use model instead of using the parent's model config.
	require.True(t, childChat.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, childChat.Mode.ChatMode)

	// The predefined computer use model is Anthropic, which
	// differs from the parent's OpenAI model. This confirms
	// that the child will not inherit the parent's model at
	// runtime.
	assert.NotEqual(t, model.Provider, chattool.ComputerUseModelProvider,
		"computer use model provider must differ from parent model provider")
	assert.Equal(t, "anthropic", chattool.ComputerUseModelProvider)
	assert.NotEmpty(t, chattool.ComputerUseModelName)
}

func TestIsSubagentDescendant(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Build a chain: root -> child -> grandchild.
	root, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root")},
	})
	require.NoError(t, err)

	child, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
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
		OwnerID: user.ID,
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
		OwnerID:            user.ID,
		Title:              "unrelated-root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated")},
	})
	require.NoError(t, err)

	unrelatedChild, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
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
