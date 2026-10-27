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

func TestCreateChat_ComputerUseChildPersistsLineage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	parent, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	prompt := "Use the desktop to open Firefox"
	parentID := uuid.NullUUID{UUID: parent.ID, Valid: true}
	child, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            parent.OwnerID,
		ParentChatID:       parentID,
		RootChatID:         parentID,
		ModelConfigID:      model.ID,
		Title:              "computer-use",
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		SystemPrompt:       "Computer use instructions\n\n" + prompt,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
	})
	require.NoError(t, err)

	got, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	for name, c := range map[string]database.Chat{"returned": child, "fetched": got} {
		require.True(t, c.ParentChatID.Valid, name)
		assert.Equal(t, parent.ID, c.ParentChatID.UUID, name)
		require.True(t, c.RootChatID.Valid, name)
		assert.Equal(t, parent.ID, c.RootChatID.UUID, name)
		require.True(t, c.Mode.Valid, name)
		assert.Equal(t, database.ChatModeComputerUse, c.Mode.ChatMode, name)
	}

	messages, err := db.GetChatMessagesForPromptByChatID(ctx, child.ID)
	require.NoError(t, err)
	// The system message raw content is a JSON-encoded string.
	var foundPrompt bool
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleSystem && msg.Content.Valid &&
			strings.Contains(string(msg.Content.RawMessage), prompt) {
			foundPrompt = true
			break
		}
	}
	assert.True(t, foundPrompt, "a system message should contain the user prompt")
}
