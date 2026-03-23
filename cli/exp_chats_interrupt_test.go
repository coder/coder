package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
)

type expChatsDiffOutput struct {
	GitChanges []codersdk.ChatGitChange  `json:"git_changes"`
	Diff       codersdk.ChatDiffContents `json:"diff"`
}

func TestExpChatsInterrupt_InvalidID(t *testing.T) {
	t.Parallel()

	_, _, err := runExpChatsCommand(t, nil, "exp", "chats", "interrupt", "not-a-uuid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid chat ID \"not-a-uuid\"")
}

func TestExpChatsInterrupt_NotFound(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	_, _, err := runExpChatsCommand(t, client, "exp", "chats", "interrupt", uuid.NewString())
	require.Error(t, err)
	require.Contains(t, err.Error(), "interrupt chat")
	require.Contains(t, err.Error(), "not found")
}

func TestExpChatsInterrupt_Success(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)
	_ = createExpChatModelConfig(t, client)

	chat := createExpChat(t, client, "interrupt this chat")

	stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "interrupt", chat.ID.String())
	require.NoError(t, err)
	require.Empty(t, stderr)
	lowerOutput := strings.ToLower(stdout)
	require.Contains(t, lowerOutput, "status")
	require.Contains(t, stdout, chat.ID.String())
	require.Contains(t, stdout, chat.Title)
	require.Contains(t, stdout, string(codersdk.ChatStatusPending))

	jsonStdout, jsonStderr, err := runExpChatsCommand(t, client, "exp", "chats", "interrupt", chat.ID.String(), "--output", "json")
	require.NoError(t, err)
	require.Empty(t, jsonStderr)

	var interrupted codersdk.Chat
	require.NoError(t, json.Unmarshal([]byte(jsonStdout), &interrupted))
	require.Equal(t, chat.ID, interrupted.ID)
	require.Equal(t, codersdk.ChatStatusPending, interrupted.Status)
}

func TestExpChatsDiff_InvalidID(t *testing.T) {
	t.Parallel()

	_, _, err := runExpChatsCommand(t, nil, "exp", "chats", "diff", "not-a-uuid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid chat ID \"not-a-uuid\"")
}

func TestExpChatsDiff_NotFound(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)

	_, _, err := runExpChatsCommand(t, client, "exp", "chats", "diff", uuid.NewString())
	require.Error(t, err)
	require.Contains(t, err.Error(), "get chat git changes")
	require.Contains(t, err.Error(), "not found")
}

func TestExpChatsDiff_Flags(t *testing.T) {
	t.Parallel()

	client := newExpChatsClient(t)
	_ = coderdtest.CreateFirstUser(t, client)
	_ = createExpChatModelConfig(t, client)

	chat := createExpChat(t, client, "diff flags chat")

	t.Run("Summary", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "diff", chat.ID.String())
		require.NoError(t, err)
		require.Empty(t, stderr)
		require.Contains(t, stdout, "No changes detected.")
	})

	t.Run("Stat", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "diff", chat.ID.String(), "--stat")
		require.NoError(t, err)
		require.Empty(t, stderr)
		require.Contains(t, stdout, "No changes detected.")
	})

	t.Run("Raw", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "diff", chat.ID.String(), "--raw")
		require.NoError(t, err)
		require.Empty(t, stderr)
		require.Empty(t, stdout)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, err := runExpChatsCommand(t, client, "exp", "chats", "diff", chat.ID.String(), "--output", "json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		var output expChatsDiffOutput
		require.NoError(t, json.Unmarshal([]byte(stdout), &output))
		require.Empty(t, output.GitChanges)
		require.Equal(t, chat.ID, output.Diff.ChatID)
		require.Empty(t, output.Diff.Diff)
	})
}
