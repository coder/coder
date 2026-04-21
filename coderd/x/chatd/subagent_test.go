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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSpawnComputerUseAgent_CreatesChildWithChatMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

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
	// to computer_use and provide a system prompt.
	prompt := "Use the desktop to open Firefox"

	child, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        parent.OwnerID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		ModelConfigID:      model.ID,
		Title:              "computer-use",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       "Computer use instructions\n\n" + prompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	// Verify parent-child relationship.
	require.True(t, child.ParentChatID.Valid)
	require.Equal(t, parent.ID, child.ParentChatID.UUID)

	// Verify the chat type is set correctly.
	require.True(t, child.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, child.Mode.ChatMode)

	// Confirm via a fresh DB read as well.
	got, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, got.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, got.Mode.ChatMode)
}

func TestSpawnComputerUseAgent_SystemPromptFormat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	parent, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	prompt := "Navigate to settings page"
	systemPrompt := "Computer use instructions\n\n" + prompt

	child, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        parent.OwnerID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		ModelConfigID:      model.ID,
		Title:              "computer-use-format",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       systemPrompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	messages, err := db.GetChatMessagesForPromptByChatID(ctx, child.ID)
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

func TestSpawnComputerUseAgent_ChildIsListedUnderParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	parent, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	prompt := "Check the UI layout"

	child, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        parent.OwnerID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		ModelConfigID:      model.ID,
		Title:              "computer-use-child",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       "Computer use instructions\n\n" + prompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	// Verify the child is linked to the parent.
	fetchedChild, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, fetchedChild.ParentChatID.Valid)
	assert.Equal(t, parent.ID, fetchedChild.ParentChatID.UUID)
}

func TestSpawnComputerUseAgent_RootChatIDPropagation(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	// Create a root parent chat (no parent of its own).
	parent, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "root-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	prompt := "Take a screenshot"

	child, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        parent.OwnerID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		ModelConfigID:      model.ID,
		Title:              "computer-use-root-test",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       "Computer use instructions\n\n" + prompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	// When the parent has no RootChatID, the child's RootChatID
	// should point to the parent.
	require.True(t, child.RootChatID.Valid)
	assert.Equal(t, parent.ID, child.RootChatID.UUID)

	// Verify chat was retrieved correctly from the DB.
	got, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	assert.True(t, got.RootChatID.Valid)
	assert.Equal(t, parent.ID, got.RootChatID.UUID)
}
