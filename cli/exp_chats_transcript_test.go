package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type expChatsTranscriptFixture struct {
	client *codersdk.Client
	db     database.Store
	chat   codersdk.Chat
}

func newExpChatsTranscriptFixture(t *testing.T) expChatsTranscriptFixture {
	t.Helper()

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: expChatsDeploymentValues(t)})
	_ = coderdtest.CreateFirstUser(t, client)
	modelConfig := createTestChatModelConfig(t, client)

	chat, err := client.CreateChat(testutil.Context(t, testutil.WaitLong), codersdk.CreateChatRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "initial prompt",
		}},
		ModelConfigID: &modelConfig.ID,
	})
	require.NoError(t, err)

	return expChatsTranscriptFixture{
		client: client,
		db:     db,
		chat:   chat,
	}
}

func insertTranscriptMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	role codersdk.ChatMessageRole,
	parts ...codersdk.ChatMessagePart,
) {
	t.Helper()

	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)

	_, err = db.InsertChatMessages(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong)), database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{uuid.Nil},
		Role:                []database.ChatMessageRole{database.ChatMessageRole(role)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(content.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
}

func TestExpChatsTranscript_InvalidID(t *testing.T) {
	t.Parallel()

	inv, _ := clitest.New(t, "exp", "chats", "transcript", "not-a-uuid")

	err := inv.WithContext(testutil.Context(t, testutil.WaitShort)).Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid chat ID")
}

func TestExpChatsTranscript_TextFormat(t *testing.T) {
	t.Parallel()

	fixture := newExpChatsTranscriptFixture(t)
	insertTranscriptMessage(t, fixture.db, fixture.chat.ID, codersdk.ChatMessageRoleSystem, codersdk.ChatMessageText("internal system prompt"))
	for i := 1; i <= 55; i++ {
		insertTranscriptMessage(t, fixture.db, fixture.chat.ID, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(fmt.Sprintf("assistant message %02d", i)))
	}

	inv, root := clitest.New(t, "exp", "chats", "transcript", fixture.chat.ID.String())
	clitest.SetupConfig(t, fixture.client, root)

	var stdout bytes.Buffer
	inv.Stdout = &stdout

	err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
	require.NoError(t, err)

	out := stdout.String()
	require.Contains(t, out, "=== User (")
	require.Contains(t, out, "initial prompt")
	require.Contains(t, out, "assistant message 01")
	require.Contains(t, out, "assistant message 55")
	require.NotContains(t, out, "internal system prompt")
	require.Less(t, strings.Index(out, "initial prompt"), strings.Index(out, "assistant message 01"))
	require.Less(t, strings.Index(out, "assistant message 01"), strings.Index(out, "assistant message 55"))
}

func TestExpChatsTranscript_JSONFormat(t *testing.T) {
	t.Parallel()

	fixture := newExpChatsTranscriptFixture(t)
	insertTranscriptMessage(t, fixture.db, fixture.chat.ID, codersdk.ChatMessageRoleSystem, codersdk.ChatMessageText("system message"))
	insertTranscriptMessage(t, fixture.db, fixture.chat.ID, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("assistant message"))

	inv, root := clitest.New(t, "exp", "chats", "transcript", "--output", "json", fixture.chat.ID.String())
	clitest.SetupConfig(t, fixture.client, root)

	var stdout bytes.Buffer
	inv.Stdout = &stdout

	err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
	require.NoError(t, err)

	var messages []codersdk.ChatMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &messages))
	require.Len(t, messages, 3)
	require.Equal(t, codersdk.ChatMessageRoleUser, messages[0].Role)
	require.Equal(t, codersdk.ChatMessageRoleSystem, messages[1].Role)
	require.Equal(t, codersdk.ChatMessageRoleAssistant, messages[2].Role)
	require.Equal(t, "assistant message", messages[2].Content[0].Text)
}

func TestExpChatsTranscript_IncludeTools(t *testing.T) {
	t.Parallel()

	fixture := newExpChatsTranscriptFixture(t)
	insertTranscriptMessage(
		t,
		fixture.db,
		fixture.chat.ID,
		codersdk.ChatMessageRoleAssistant,
		codersdk.ChatMessageText("Need weather. "),
		codersdk.ChatMessageToolCall("call-1", "weather", json.RawMessage(`{"city":"SF"}`)),
		codersdk.ChatMessageToolResult("call-1", "weather", json.RawMessage(`{"temp":"68F"}`), false),
		codersdk.ChatMessageText("Done."),
	)

	t.Run("DefaultOmitsTools", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "exp", "chats", "transcript", fixture.chat.ID.String())
		clitest.SetupConfig(t, fixture.client, root)

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
		require.NoError(t, err)
		require.NotContains(t, stdout.String(), "[Tool Call:")
		require.NotContains(t, stdout.String(), "[Tool Result:")
	})

	t.Run("IncludeToolsShowsTools", func(t *testing.T) {
		t.Parallel()

		inv, root := clitest.New(t, "exp", "chats", "transcript", "--include-tools", fixture.chat.ID.String())
		clitest.SetupConfig(t, fixture.client, root)

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), `[Tool Call: weather({"city":"SF"})]`)
		require.Contains(t, stdout.String(), `[Tool Result: weather → {"temp":"68F"}]`)
	})
}

func TestExpChatsTranscript_OutputFile(t *testing.T) {
	t.Parallel()

	fixture := newExpChatsTranscriptFixture(t)
	insertTranscriptMessage(t, fixture.db, fixture.chat.ID, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("written to a file"))

	path := filepath.Join(t.TempDir(), "transcript.txt")
	inv, root := clitest.New(t, "exp", "chats", "transcript", "--output-file", path, fixture.chat.ID.String())
	clitest.SetupConfig(t, fixture.client, root)

	var stdout bytes.Buffer
	inv.Stdout = &stdout

	err := inv.WithContext(testutil.Context(t, testutil.WaitLong)).Run()
	require.NoError(t, err)
	require.Empty(t, stdout.String())

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), "written to a file")
}
