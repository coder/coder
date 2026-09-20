package chatd_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSpawnComputerUseAgent_CreatesChildWithChatMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	// Create a parent chat.
	parent, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Simulate what spawn_agent does: set ChatMode
	// to computer_use and insert a subagent kind chat.
	prompt := "Use the desktop to open Firefox"
	promptContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)})
	require.NoError(t, err)

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           parent.OwnerID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		Kind:              database.ChatKindSubagent,
		LastModelConfigID: model.ID,
		Title:             "computer-use",
		Mode:              database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		MCPServerIDs:      []uuid.UUID{},
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        promptContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	child := created.Chat

	// Verify parent-child relationship.
	require.True(t, child.ParentChatID.Valid)
	require.Equal(t, parent.ID, child.ParentChatID.UUID)
	require.Equal(t, database.ChatKindSubagent, child.Kind)

	// Verify the chat type is set correctly.
	require.True(t, child.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, child.Mode.ChatMode)

	// Confirm via a fresh DB read as well.
	got, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, got.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, got.Mode.ChatMode)
}

func TestCreateChat_SystemPromptFormat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	prompt := "Navigate to settings page"
	systemPrompt := "Computer use instructions\n\n" + prompt

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		ModelConfigID:      model.ID,
		Title:              "computer-use-format",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       systemPrompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)

	// The system message raw content is a JSON-encoded string.
	// It should contain the system prompt with the user prompt.
	var foundPrompt bool
	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		if msg.Content.Valid && strings.Contains(string(msg.Content.RawMessage), prompt) {
			foundPrompt = true
			break
		}
	}

	assert.True(t, foundPrompt,
		"at least one system message should contain the user prompt")
}
